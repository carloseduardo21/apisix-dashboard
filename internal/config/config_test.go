package config

import (
	"strings"
	"testing"
)

func TestTrustedProxies(t *testing.T) {
	tests := []struct {
		in      string
		want    []string
		wantErr bool
	}{
		{in: "", want: nil},
		{in: "10.0.0.0/8", want: []string{"10.0.0.0/8"}},
		{in: " 10.1.2.3/8 , 192.0.2.7 ,2001:db8::/32", want: []string{"10.0.0.0/8", "192.0.2.7/32", "2001:db8::/32"}},
		{in: "::ffff:192.0.2.7", want: []string{"192.0.2.7/32"}},
		{in: "10.0.0.0/8,example.com", wantErr: true},
		{in: "10.0.0.0/33", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			t.Setenv("DASH_TRUSTED_PROXIES", tc.in)
			got, err := prefixes("DASH_TRUSTED_PROXIES")
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i].String() != tc.want[i] {
					t.Errorf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestLoginLimitsValidation(t *testing.T) {
	tests := []struct {
		name, key, value, wantErr string
	}{
		{"concurrency zero", "DASH_LOGIN_CONCURRENCY", "0", "DASH_LOGIN_CONCURRENCY must be between 1 and 64"},
		{"concurrency too high", "DASH_LOGIN_CONCURRENCY", "65", "DASH_LOGIN_CONCURRENCY must be between 1 and 64"},
		{"rate zero", "DASH_LOGIN_RATE_PER_MINUTE", "0", "DASH_LOGIN_RATE_PER_MINUTE must be between 1 and 1000"},
		{"rate not a number", "DASH_LOGIN_RATE_PER_MINUTE", "fast", "invalid integer"},
		{"valid", "DASH_LOGIN_CONCURRENCY", "4", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			c, err := Load()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if c.LoginConcurrency != 4 || c.LoginRatePerMinute != 20 {
					t.Errorf("got concurrency %d rate %d", c.LoginConcurrency, c.LoginRatePerMinute)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("err = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestLegacyTrustProxyWarning(t *testing.T) {
	t.Setenv("DASH_TRUST_PROXY_HEADERS", "true")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(c.Warnings(), "\n"), "DASH_TRUST_PROXY_HEADERS is deprecated") {
		t.Error("expected a deprecation warning")
	}
	t.Setenv("DASH_TRUSTED_PROXIES", "10.0.0.0/8")
	if c, err = Load(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(c.Warnings(), "\n"), "deprecated") {
		t.Error("no deprecation warning once DASH_TRUSTED_PROXIES is set")
	}
}
