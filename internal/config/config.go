// Package config loads the dashboard configuration from environment variables.
//
// Every secret value (e.g. DASH_ADMIN_API_KEY) can also be provided through a
// file by appending _FILE to the variable name, which plays well with Docker
// Swarm secrets mounted under /run/secrets.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddr  string
	TLSCertFile string
	TLSKeyFile  string

	AdminAPIURL     string
	AdminAPIKey     string
	AdminAPICAFile  string
	AdminAPITimeout time.Duration

	DataDir           string
	EncryptionKeyFile string

	SessionIdleTimeout time.Duration
	SessionMaxLifetime time.Duration
	CookieSecure       bool
	TrustProxyHeaders  bool

	MaxLoginAttempts int
	LockoutDuration  time.Duration

	// AuditLog is where audit events are mirrored as JSON lines, in addition
	// to the local database: "stdout", "none" or a file path.
	AuditLog string

	ACME ACME
	Kong Kong
}

// ACME configures automatic certificate issuance (DNS-01 via Cloudflare).
// It is enabled when a Cloudflare API token is provided.
type ACME struct {
	Email              string
	CloudflareToken    string
	CloudflareAPIURL   string
	DirectoryURL       string
	StagingURL         string
	CAFile             string
	RenewBefore        time.Duration
	DNSResolver        string
	PropagationTimeout time.Duration
}

func (a ACME) Enabled() bool { return a.CloudflareToken != "" }

// Kong configures the migration from Kong Gateway. It is enabled when the
// Kong Admin API URL is provided; the dashboard only reads from it.
type Kong struct {
	AdminURL    string
	Workspace   string
	TokenHeader string
	Token       string
	CAFile      string
	Timeout     time.Duration
}

func (k Kong) Enabled() bool { return k.AdminURL != "" }

func Load() (*Config, error) {
	c := &Config{
		ListenAddr:        env("DASH_LISTEN_ADDR", ":8080"),
		TLSCertFile:       env("DASH_TLS_CERT_FILE", ""),
		TLSKeyFile:        env("DASH_TLS_KEY_FILE", ""),
		AdminAPIURL:       strings.TrimRight(env("DASH_ADMIN_API_URL", ""), "/"),
		AdminAPICAFile:    env("DASH_ADMIN_API_CA_FILE", ""),
		DataDir:           env("DASH_DATA_DIR", "./data"),
		EncryptionKeyFile: env("DASH_ENCRYPTION_KEY_FILE", ""),
		AuditLog:          env("DASH_AUDIT_LOG", "stdout"),
		ACME: ACME{
			Email:            env("DASH_ACME_EMAIL", ""),
			CloudflareAPIURL: strings.TrimRight(env("DASH_ACME_CLOUDFLARE_API_URL", "https://api.cloudflare.com/client/v4"), "/"),
			DirectoryURL:     env("DASH_ACME_DIRECTORY", "https://acme-v02.api.letsencrypt.org/directory"),
			StagingURL:       env("DASH_ACME_STAGING_DIRECTORY", "https://acme-staging-v02.api.letsencrypt.org/directory"),
			CAFile:           env("DASH_ACME_CA_FILE", ""),
			DNSResolver:      env("DASH_ACME_DNS_RESOLVER", ""),
		},
		Kong: Kong{
			AdminURL:    strings.TrimRight(env("DASH_KONG_ADMIN_URL", ""), "/"),
			Workspace:   env("DASH_KONG_WORKSPACE", ""),
			TokenHeader: env("DASH_KONG_ADMIN_TOKEN_HEADER", "Kong-Admin-Token"),
			CAFile:      env("DASH_KONG_CA_FILE", ""),
		},
	}

	var errs []error
	var err error
	if c.AdminAPIKey, err = secret("DASH_ADMIN_API_KEY"); err != nil {
		errs = append(errs, err)
	}
	c.AdminAPITimeout = duration("DASH_ADMIN_API_TIMEOUT", 10*time.Second, &errs)
	c.SessionIdleTimeout = duration("DASH_SESSION_IDLE_TIMEOUT", 15*time.Minute, &errs)
	c.SessionMaxLifetime = duration("DASH_SESSION_MAX_LIFETIME", 12*time.Hour, &errs)
	c.LockoutDuration = duration("DASH_LOCKOUT_DURATION", 30*time.Minute, &errs)
	c.CookieSecure = boolean("DASH_COOKIE_SECURE", true, &errs)
	c.TrustProxyHeaders = boolean("DASH_TRUST_PROXY_HEADERS", false, &errs)
	c.MaxLoginAttempts = integer("DASH_MAX_LOGIN_ATTEMPTS", 6, &errs)
	if c.ACME.CloudflareToken, err = secret("DASH_ACME_CLOUDFLARE_API_TOKEN"); err != nil {
		errs = append(errs, err)
	}
	c.ACME.RenewBefore = duration("DASH_ACME_RENEW_BEFORE", 30*24*time.Hour, &errs)
	c.ACME.PropagationTimeout = duration("DASH_ACME_PROPAGATION_TIMEOUT", 2*time.Minute, &errs)
	if c.Kong.Token, err = secret("DASH_KONG_ADMIN_TOKEN"); err != nil {
		errs = append(errs, err)
	}
	c.Kong.Timeout = duration("DASH_KONG_TIMEOUT", 30*time.Second, &errs)
	if c.Kong.AdminURL != "" {
		if u, err := url.Parse(c.Kong.AdminURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errs = append(errs, fmt.Errorf("DASH_KONG_ADMIN_URL is not a valid http(s) URL: %q", c.Kong.AdminURL))
		}
	}

	if (c.TLSCertFile == "") != (c.TLSKeyFile == "") {
		errs = append(errs, errors.New("DASH_TLS_CERT_FILE and DASH_TLS_KEY_FILE must be set together"))
	}
	if c.MaxLoginAttempts < 1 || c.MaxLoginAttempts > 10 {
		errs = append(errs, errors.New("DASH_MAX_LOGIN_ATTEMPTS must be between 1 and 10"))
	}
	if c.EncryptionKeyFile == "" {
		c.EncryptionKeyFile = filepath.Join(c.DataDir, "encryption.key")
	}
	return c, errors.Join(errs...)
}

// ValidateServe checks the settings that are only required to run the server.
func (c *Config) ValidateServe() error {
	if c.AdminAPIURL == "" {
		return errors.New("DASH_ADMIN_API_URL is required (e.g. http://apisix:9180)")
	}
	u, err := url.Parse(c.AdminAPIURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("DASH_ADMIN_API_URL is not a valid http(s) URL: %q", c.AdminAPIURL)
	}
	return nil
}

// Warnings returns settings that are allowed but deviate from PCI DSS guidance.
func (c *Config) Warnings() []string {
	var w []string
	if c.AdminAPIKey == "" {
		w = append(w, "DASH_ADMIN_API_KEY is not set: Admin API requests are sent without X-API-KEY (only acceptable in test environments)")
	}
	if c.SessionIdleTimeout > 15*time.Minute {
		w = append(w, "DASH_SESSION_IDLE_TIMEOUT above 15m does not meet PCI DSS 8.2.8")
	}
	if c.LockoutDuration < 30*time.Minute {
		w = append(w, "DASH_LOCKOUT_DURATION below 30m does not meet PCI DSS 8.3.4")
	}
	if !c.CookieSecure {
		w = append(w, "DASH_COOKIE_SECURE=false: session cookie will be sent over plain HTTP")
	}
	if c.TLSCertFile == "" && !c.TrustProxyHeaders {
		w = append(w, "TLS is not enabled; terminate TLS in front of the dashboard")
	}
	if strings.HasPrefix(c.Kong.AdminURL, "http://") {
		w = append(w, "Kong Admin API is reached over plain HTTP; keep it on an isolated internal network")
	}
	if strings.HasPrefix(c.AdminAPIURL, "http://") {
		w = append(w, "Admin API is reached over plain HTTP; keep it on an isolated internal network")
	}
	return w
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return strings.TrimSpace(v)
	}
	return def
}

func secret(key string) (string, error) {
	if path := env(key+"_FILE", ""); path != "" {
		b, err := os.ReadFile(path) // #nosec G304 -- *_FILE secret path set by the operator (e.g. /run/secrets)
		if err != nil {
			return "", fmt.Errorf("%s_FILE: %w", key, err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	return env(key, ""), nil
}

func duration(key string, def time.Duration, errs *[]error) time.Duration {
	v := env(key, "")
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		*errs = append(*errs, fmt.Errorf("%s: invalid duration %q", key, v))
		return def
	}
	return d
}

func boolean(key string, def bool, errs *[]error) bool {
	v := env(key, "")
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s: invalid boolean %q", key, v))
		return def
	}
	return b
}

func integer(key string, def int, errs *[]error) int {
	v := env(key, "")
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s: invalid integer %q", key, v))
		return def
	}
	return n
}
