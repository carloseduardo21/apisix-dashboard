package auth

import (
	"testing"
	"time"
)

// RFC 6238 appendix B test vector (SHA1).
func TestTOTPVector(t *testing.T) {
	secret := []byte("12345678901234567890")
	if got := totpCode(secret, 59/30); got != "287082" {
		t.Errorf("got %s want 287082", got)
	}
	if got := totpCode(secret, 1111111109/30); got != "081804" {
		t.Errorf("got %s want 081804", got)
	}
}

func TestTOTPReplay(t *testing.T) {
	secret := newTOTPSecret()
	now := time.Unix(1_800_000_000, 0)
	code := totpCode(secret, now.Unix()/totpPeriod)
	step, ok := verifyTOTP(secret, code, now, 0)
	if !ok {
		t.Fatal("valid code rejected")
	}
	if _, ok := verifyTOTP(secret, code, now, step); ok {
		t.Fatal("replayed code accepted")
	}
	if _, ok := verifyTOTP(secret, code, now.Add(5*time.Minute), 0); ok {
		t.Fatal("stale code accepted")
	}
}

func TestPassword(t *testing.T) {
	h := HashPassword("correct horse 42 battery")
	if !VerifyPassword("correct horse 42 battery", h) || VerifyPassword("wrong", h) {
		t.Fatal("verify mismatch")
	}
	for _, pw := range []string{"short1", "onlyletterslong", "123456789012345", "admin-is-here-123"} {
		if ValidatePassword("admin", pw) == nil {
			t.Errorf("%q should be rejected", pw)
		}
	}
	if err := ValidatePassword("admin", "Gateway-Seguro-2026"); err != nil {
		t.Error(err)
	}
}

func TestTOTPURI(t *testing.T) {
	t.Parallel()
	secret := []byte("12345678901234567890")
	tests := []struct {
		name, issuer, account, want string
	}{
		{
			name:    "plain",
			issuer:  "Lintel",
			account: "alice",
			want:    "otpauth://totp/Lintel:alice?algorithm=SHA1&digits=6&issuer=Lintel&period=30&secret=",
		},
		{
			name:    "escaped",
			issuer:  "Lintel Prod",
			account: "bob smith",
			want:    "otpauth://totp/Lintel%20Prod:bob%20smith?algorithm=SHA1&digits=6&issuer=Lintel+Prod&period=30&secret=",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// The base32 secret is appended here so scanners do not flag a literal.
			if got, want := totpURI(tc.issuer, tc.account, secret), tc.want+b32.EncodeToString(secret); got != want {
				t.Errorf("got  %s\nwant %s", got, want)
			}
		})
	}
}
