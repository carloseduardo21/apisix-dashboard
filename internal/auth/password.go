package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters (OWASP recommendation: m=64MiB, t=1, p=4 or stronger).
const (
	argonTime    = 2
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16

	MinPasswordLength = 12
	maxPasswordLength = 256
	PasswordHistory   = 4
)

var b64 = base64.RawStdEncoding

func HashPassword(pw string) string {
	salt := make([]byte, argonSaltLen)
	rand.Read(salt)
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads, b64.EncodeToString(salt), b64.EncodeToString(key))
}

func VerifyPassword(pw, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := b64.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false
	}
	got := argon2.IDKey([]byte(pw), salt, t, m, p, uint32(len(want))) // #nosec G115 -- len(want) is a decoded hash length, far below MaxUint32
	return subtle.ConstantTimeCompare(got, want) == 1
}

// ValidatePassword enforces PCI DSS 8.3.6: at least 12 characters with both
// letters and numbers.
func ValidatePassword(username, pw string) error {
	if len([]rune(pw)) < MinPasswordLength {
		return validationErr("auth.password_too_short", "the password must have at least {min} characters", "min", MinPasswordLength)
	}
	if len(pw) > maxPasswordLength {
		return validationErr("auth.password_too_long", "the password must have at most {max} characters", "max", maxPasswordLength)
	}
	var letter, digit bool
	for _, r := range pw {
		switch {
		case unicode.IsLetter(r):
			letter = true
		case unicode.IsDigit(r):
			digit = true
		}
	}
	if !letter || !digit {
		return validationErr("auth.password_letters_digits", "the password must contain letters and digits")
	}
	if username != "" && strings.Contains(strings.ToLower(pw), strings.ToLower(username)) {
		return validationErr("auth.password_contains_username", "the password cannot contain the username")
	}
	return nil
}
