package server

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestClientIP(t *testing.T) {
	t.Parallel()
	proxies := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("2001:db8:ffff::/48")}
	tests := []struct {
		name    string
		trusted []netip.Prefix
		legacy  bool
		remote  string
		xff     []string
		realIP  string
		want    string
	}{
		{name: "no proxy config ignores headers", remote: "198.51.100.7:1234", xff: []string{"203.0.113.1"}, realIP: "203.0.113.2", want: "198.51.100.7"},
		{name: "untrusted peer cannot forward", trusted: proxies, remote: "198.51.100.7:1234", xff: []string{"203.0.113.1"}, want: "198.51.100.7"},
		{name: "trusted proxy", trusted: proxies, remote: "10.0.0.2:1234", xff: []string{"203.0.113.5"}, want: "203.0.113.5"},
		{name: "spoofed leftmost entry is ignored", trusted: proxies, remote: "10.0.0.2:1234", xff: []string{"1.1.1.1, 203.0.113.5"}, want: "203.0.113.5"},
		{name: "chain of trusted proxies", trusted: proxies, remote: "10.0.0.2:1234", xff: []string{"6.6.6.6, 203.0.113.5, 10.1.1.1"}, want: "203.0.113.5"},
		{name: "several header lines", trusted: proxies, remote: "10.0.0.2:1234", xff: []string{"6.6.6.6", "203.0.113.5, 10.1.1.1"}, want: "203.0.113.5"},
		{name: "all hops trusted", trusted: proxies, remote: "10.0.0.2:1234", xff: []string{"10.9.9.9"}, want: "10.9.9.9"},
		{name: "malformed entry stops the walk", trusted: proxies, remote: "10.0.0.2:1234", xff: []string{"203.0.113.5, garbage, 10.1.1.1"}, want: "10.1.1.1"},
		{name: "real ip from trusted proxy", trusted: proxies, remote: "10.0.0.2:1234", realIP: "203.0.113.9", want: "203.0.113.9"},
		{name: "invalid real ip", trusted: proxies, remote: "10.0.0.2:1234", realIP: "nope", want: "10.0.0.2"},
		{name: "ipv6 proxy", trusted: proxies, remote: "[2001:db8:ffff::1]:443", xff: []string{"2001:db8:1::5"}, want: "2001:db8:1::5"},
		{name: "ipv4-mapped peer", trusted: proxies, remote: "[::ffff:10.0.0.2]:443", xff: []string{"203.0.113.5"}, want: "203.0.113.5"},
		{name: "legacy trusts only the direct peer", legacy: true, remote: "198.51.100.7:1234", xff: []string{"1.1.1.1, 203.0.113.5"}, want: "203.0.113.5"},
		{name: "legacy without header", legacy: true, remote: "198.51.100.7:1234", want: "198.51.100.7"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := New(Options{TrustedProxies: tc.trusted, TrustProxy: tc.legacy})
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tc.remote
			for _, v := range tc.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			if tc.realIP != "" {
				r.Header.Set("X-Real-IP", tc.realIP)
			}
			if got := s.clientIP(r); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}
