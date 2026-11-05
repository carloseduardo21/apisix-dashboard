// Command apisix-dashboard serves the dashboard (default) and manages local
// users from the command line.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/carloseduardo21/apisix-dashboard/internal/acme"
	"github.com/carloseduardo21/apisix-dashboard/internal/apisix"
	"github.com/carloseduardo21/apisix-dashboard/internal/audit"
	"github.com/carloseduardo21/apisix-dashboard/internal/auth"
	"github.com/carloseduardo21/apisix-dashboard/internal/config"
	"github.com/carloseduardo21/apisix-dashboard/internal/kong"
	"github.com/carloseduardo21/apisix-dashboard/internal/secure"
	"github.com/carloseduardo21/apisix-dashboard/internal/server"
	"github.com/carloseduardo21/apisix-dashboard/internal/store"
	"github.com/carloseduardo21/apisix-dashboard/internal/web"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = `apisix-dashboard %s

Uso:
  apisix-dashboard [serve]                  inicia o servidor
  apisix-dashboard user add -u NOME -r PERFIL
  apisix-dashboard user passwd -u NOME      define uma nova senha (temporária)
  apisix-dashboard user reset-mfa -u NOME
  apisix-dashboard user unlock -u NOME
  apisix-dashboard user list
  apisix-dashboard healthcheck              verifica o servidor local (HEALTHCHECK do Docker)
  apisix-dashboard version

Perfis: viewer, operator, admin. Configuração via variáveis DASH_* (veja o README).
`

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve()
	case "user":
		err = userCmd(os.Args[2:])
	case "healthcheck":
		err = healthcheck()
	case "version", "--version", "-v":
		fmt.Println(version)
	case "help", "--help", "-h":
		fmt.Printf(usage, version)
	default:
		fmt.Fprintf(os.Stderr, usage, version)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

// state is what both the server and the CLI need.
type state struct {
	cfg   *config.Config
	store *store.Store
	audit *audit.Logger
	auth  *auth.Service
	aead  *secure.AEAD
}

func open(auditOut io.Writer) (*state, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, err
	}
	explicitKey := os.Getenv("DASH_ENCRYPTION_KEY_FILE") != ""
	key, created, err := secure.LoadKey(cfg.EncryptionKeyFile, !explicitKey)
	if err != nil {
		return nil, fmt.Errorf("encryption key: %w", err)
	}
	if created {
		slog.Warn("generated a new encryption key; back it up or provide DASH_ENCRYPTION_KEY_FILE", "path", cfg.EncryptionKeyFile)
	}
	aead, err := secure.NewAEAD(key)
	if err != nil {
		return nil, err
	}
	st, err := store.Open(filepath.Join(cfg.DataDir, "dashboard.db"))
	if err != nil {
		return nil, err
	}
	al := audit.New(st, auditOut)
	svc := auth.NewService(st, aead, al, auth.Config{
		Issuer:             "APISIX Dashboard",
		SessionIdleTimeout: cfg.SessionIdleTimeout,
		SessionMaxLifetime: cfg.SessionMaxLifetime,
		PendingTimeout:     5 * time.Minute,
		MaxLoginAttempts:   cfg.MaxLoginAttempts,
		LockoutDuration:    cfg.LockoutDuration,
	})
	return &state{cfg: cfg, store: st, audit: al, auth: svc, aead: aead}, nil
}

func auditWriter() (io.Writer, error) {
	switch dest := os.Getenv("DASH_AUDIT_LOG"); dest {
	case "", "stdout":
		return os.Stdout, nil
	case "none":
		return nil, nil
	default:
		return os.OpenFile(dest, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	}
}

func serve() error {
	out, err := auditWriter()
	if err != nil {
		return fmt.Errorf("audit log: %w", err)
	}
	st, err := open(out)
	if err != nil {
		return err
	}
	defer st.store.Close()
	cfg := st.cfg
	if err := cfg.ValidateServe(); err != nil {
		return err
	}
	for _, w := range cfg.Warnings() {
		slog.Warn(w)
	}

	client, err := apisix.NewClient(cfg.AdminAPIURL, cfg.AdminAPIKey, cfg.AdminAPICAFile, cfg.AdminAPITimeout)
	if err != nil {
		return err
	}
	catalog := apisix.NewCatalog(client)
	var issuer *acme.Manager
	if cfg.ACME.Enabled() {
		if issuer, err = newIssuer(st, client); err != nil {
			return err
		}
	}
	var migrator *kong.Migrator
	if cfg.Kong.Enabled() {
		kc, err := kong.NewClient(cfg.Kong.AdminURL, cfg.Kong.Workspace, cfg.Kong.TokenHeader, cfg.Kong.Token, cfg.Kong.CAFile, cfg.Kong.Timeout)
		if err != nil {
			return err
		}
		migrator = kong.NewMigrator(kong.Options{
			Store: st.store, APISIX: client, Audit: st.audit, Kong: kc,
			Mask: func(resource string, v any) any { return apisix.Mask(resource, v, catalog.EncryptFields) },
		})
	}
	srv := server.New(server.Options{
		Store:        st.store,
		Auth:         st.auth,
		APISIX:       client,
		Catalog:      catalog,
		ACME:         issuer,
		Kong:         migrator,
		Audit:        st.audit,
		Static:       web.FS(),
		CookieSecure: cfg.CookieSecure,
		TrustProxy:   cfg.TrustProxyHeaders,
		TLS:          cfg.TLSCertFile != "",
		Version:      version,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv.StartSessionJanitor(ctx)
	catalog.Run(ctx)
	if issuer != nil {
		issuer.Run(ctx)
		slog.Info("acme enabled", "provider", "cloudflare", "renew_before", cfg.ACME.RenewBefore)
	}
	if migrator != nil {
		migrator.Run(ctx)
		slog.Info("kong migration enabled", "kong_admin_api", cfg.Kong.AdminURL)
	}

	if n, err := st.store.CountUsers(ctx); err == nil && n == 0 {
		slog.Warn("no users yet; create an admin with: apisix-dashboard user add -u admin -r admin")
	}

	hs := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
	}
	errc := make(chan error, 1)
	go func() {
		slog.Info("dashboard listening", "addr", cfg.ListenAddr, "tls", cfg.TLSCertFile != "", "admin_api", cfg.AdminAPIURL, "version", version)
		if cfg.TLSCertFile != "" {
			errc <- hs.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
		} else {
			errc <- hs.ListenAndServe()
		}
	}()

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		slog.Info("shutting down")
		shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return hs.Shutdown(shutCtx)
	}
	return nil
}

func newIssuer(st *state, client *apisix.Client) (*acme.Manager, error) {
	a := st.cfg.ACME
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if a.CAFile != "" {
		pem, err := os.ReadFile(a.CAFile)
		if err != nil {
			return nil, fmt.Errorf("DASH_ACME_CA_FILE: %w", err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("DASH_ACME_CA_FILE: no certificates found")
		}
		tr.TLSClientConfig.RootCAs = pool
	}
	hc := &http.Client{Timeout: 30 * time.Second, Transport: tr}
	return acme.NewManager(acme.Options{
		Store:              st.store,
		AEAD:               st.aead,
		APISIX:             client,
		Audit:              st.audit,
		DNS:                acme.NewCloudflare(a.CloudflareAPIURL, a.CloudflareToken, &http.Client{Timeout: 30 * time.Second}),
		HTTPClient:         hc,
		Email:              a.Email,
		Directories:        map[string]string{acme.EnvProduction: a.DirectoryURL, acme.EnvStaging: a.StagingURL},
		RenewBefore:        a.RenewBefore,
		Resolver:           a.DNSResolver,
		PropagationTimeout: a.PropagationTimeout,
	}), nil
}

// healthcheck probes the local server; used by the container HEALTHCHECK
// since the distroless image has no shell or curl.
func healthcheck() error {
	addr := os.Getenv("DASH_LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	scheme := "http"
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if os.Getenv("DASH_TLS_CERT_FILE") != "" {
		scheme = "https"
		// Loopback only: the certificate is issued for the public name.
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	c := &http.Client{Timeout: 3 * time.Second, Transport: tr}
	resp, err := c.Get(scheme + "://127.0.0.1:" + port + "/api/health")
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unhealthy: HTTP %d", resp.StatusCode)
	}
	return nil
}
