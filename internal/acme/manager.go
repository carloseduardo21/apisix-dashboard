// Package acme issues and renews certificates with an ACME CA (Let's Encrypt)
// using the DNS-01 challenge, and publishes them to APISIX as SSL objects.
//
// Private keys are generated for every issuance, sent to APISIX and never
// stored by the dashboard. Only the CA account key is kept, encrypted.
package acme

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/acme"

	"github.com/carloseduardo21/apisix-dashboard/internal/apisix"
	"github.com/carloseduardo21/apisix-dashboard/internal/audit"
	"github.com/carloseduardo21/apisix-dashboard/internal/secure"
	"github.com/carloseduardo21/apisix-dashboard/internal/store"
)

const (
	StatusPending = "pending"
	StatusIssuing = "issuing"
	StatusValid   = "valid"
	StatusError   = "error"

	EnvProduction = "production"
	EnvStaging    = "staging"

	KeyEC256   = "ec256"
	KeyRSA2048 = "rsa2048"

	// Labels set on the SSL objects published by the dashboard.
	LabelManagedBy = "managed-by"
	LabelCertID    = "acme-certificate"
	ManagedByValue = "apisix-dashboard"

	issueTimeout  = 10 * time.Minute
	checkInterval = time.Hour
)

var ErrBusy = errors.New("emissão já em andamento para este certificado")

// Actor identifies who triggered an operation, for the audit trail.
type Actor struct {
	Name string
	IP   string
}

// AutoRenew is the actor of scheduled renewals.
var AutoRenew = Actor{Name: "acme:renovação-automática"}

type Options struct {
	Store  *store.Store
	AEAD   *secure.AEAD
	APISIX *apisix.Client
	Audit  *audit.Logger
	DNS    *Cloudflare
	// HTTPClient talks to the CA.
	HTTPClient         *http.Client
	Email              string
	Directories        map[string]string // environment → directory URL
	RenewBefore        time.Duration
	Resolver           string // host:port; empty uses the zone's name servers
	PropagationTimeout time.Duration
}

type Manager struct {
	Options

	ctx     context.Context
	wake    chan struct{}
	issueMu sync.Mutex // one issuance at a time

	mu   sync.Mutex
	busy map[int64]bool

	clientsMu sync.Mutex
	clients   map[string]*acme.Client
}

func NewManager(o Options) *Manager {
	return &Manager{Options: o, ctx: context.Background(), wake: make(chan struct{}, 1), busy: map[int64]bool{}, clients: map[string]*acme.Client{}}
}

// Run recovers interrupted issuances and starts the renewal scheduler.
func (m *Manager) Run(ctx context.Context) {
	m.ctx = ctx
	certs, err := m.Store.ListCertificates(ctx)
	if err != nil {
		slog.Error("acme: list certificates", "err", err)
	}
	for _, c := range certs {
		if c.Status == StatusIssuing {
			c.Status = StatusError
			c.LastError = "emissão interrompida pelo reinício da dashboard"
			c.NextAttemptAt = time.Now()
			c.UpdatedAt = time.Now()
			if err := m.Store.UpdateCertificate(ctx, c); err != nil {
				slog.Error("acme: mark interrupted issuance", "id", c.ID, "err", err)
			}
		}
	}
	go func() {
		t := time.NewTimer(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			case <-m.wake:
			}
			m.renewDue(ctx)
			t.Reset(checkInterval)
		}
	}()
}

// Due reports whether c must be (re)issued now.
func (m *Manager) Due(c *store.Certificate, now time.Time) bool {
	if c.Status == StatusIssuing || c.NextAttemptAt.After(now) {
		return false
	}
	return c.NotAfter.IsZero() || !now.Before(m.RenewAt(c))
}

// RenewAt is when c will be renewed, ignoring failures: RenewBefore ahead of
// expiry, but never earlier than 2/3 of the lifetime, so short-lived
// certificates are not renewed on every check.
func (m *Manager) RenewAt(c *store.Certificate) time.Time {
	if c.NotAfter.IsZero() {
		return time.Time{}
	}
	window := m.RenewBefore
	if life := c.NotAfter.Sub(c.NotBefore); !c.NotBefore.IsZero() && window > life/3 {
		window = life / 3
	}
	return c.NotAfter.Add(-window)
}

func (m *Manager) renewDue(ctx context.Context) {
	certs, err := m.Store.ListCertificates(ctx)
	if err != nil {
		slog.Error("acme: list certificates", "err", err)
		return
	}
	now := time.Now()
	for _, c := range certs {
		if m.Due(c, now) {
			if err := m.start(ctx, c, AutoRenew, false); err == nil {
				m.issue(ctx, c.ID, AutoRenew)
			}
		}
	}
}

// Trigger issues c now, in the background.
func (m *Manager) Trigger(ctx context.Context, id int64, who Actor) error {
	c, err := m.Store.GetCertificate(ctx, id)
	if err != nil {
		return err
	}
	if err := m.start(ctx, c, who, true); err != nil {
		return err
	}
	go m.issue(m.ctx, id, who)
	return nil
}

// Busy reports whether an issuance of id is running.
func (m *Manager) Busy(id int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.busy[id]
}

// start marks c as issuing so the UI shows progress right away.
func (m *Manager) start(ctx context.Context, c *store.Certificate, who Actor, manual bool) error {
	m.mu.Lock()
	if m.busy[c.ID] {
		m.mu.Unlock()
		return ErrBusy
	}
	m.busy[c.ID] = true
	m.mu.Unlock()

	c.Status = StatusIssuing
	c.LastError = ""
	reason := "renovação automática"
	if manual {
		reason = "solicitada por " + who.Name
	} else if c.NotAfter.IsZero() {
		reason = "primeira emissão"
	}
	c.Log, _ = json.Marshal([]logEntry{{Time: time.Now(), Level: "info", Message: "Emissão iniciada (" + reason + ")"}})
	c.UpdatedAt = time.Now()
	if err := m.Store.UpdateCertificate(ctx, c); err != nil {
		m.release(c.ID)
		return err
	}
	return nil
}

func (m *Manager) release(id int64) {
	m.mu.Lock()
	delete(m.busy, id)
	m.mu.Unlock()
}

type logEntry struct {
	Time    time.Time `json:"ts"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

// runLog collects the steps of an issuance and saves them as they happen.
type runLog struct {
	m       *Manager
	cert    *store.Certificate
	entries []logEntry
}

func (l *runLog) add(level, format string, args ...any) {
	l.entries = append(l.entries, logEntry{Time: time.Now(), Level: level, Message: fmt.Sprintf(format, args...)})
	l.cert.Log, _ = json.Marshal(l.entries)
	l.cert.UpdatedAt = time.Now()
	if err := l.m.Store.UpdateCertificate(context.WithoutCancel(l.m.ctx), l.cert); err != nil {
		slog.Error("acme: save log", "err", err)
	}
}

func (l *runLog) info(format string, args ...any) { l.add("info", format, args...) }
func (l *runLog) warn(format string, args ...any) { l.add("warn", format, args...) }

func (m *Manager) issue(ctx context.Context, id int64, who Actor) {
	m.issueMu.Lock()
	defer m.issueMu.Unlock()
	defer m.release(id)

	c, err := m.Store.GetCertificate(ctx, id)
	if err != nil {
		slog.Error("acme: load certificate", "id", id, "err", err)
		return
	}
	run := &runLog{m: m, cert: c}
	if err := json.Unmarshal(c.Log, &run.entries); err != nil && len(c.Log) > 0 {
		slog.Warn("acme: previous issuance log unreadable, starting a new one", "id", id, "err", err)
	}

	ctx, cancel := context.WithTimeout(ctx, issueTimeout)
	defer cancel()
	c.LastAttemptAt = time.Now()
	res, err := m.obtain(ctx, c, run)
	if err == nil {
		err = m.publish(ctx, c, res, who, run)
	}

	now := time.Now()
	if err != nil {
		c.Failures++
		c.Status = StatusError
		c.LastError = err.Error()
		c.NextAttemptAt = now.Add(backoff(c.Failures))
		run.add("error", "Falhou: %s. Nova tentativa automática em %s.", err, c.NextAttemptAt.Format("02/01 15:04"))
		m.Audit.Record(ctx, store.AuditEvent{
			Actor: who.Name, IP: who.IP, Action: "acme.issue", Resource: "ssls", ResourceID: c.SSLID,
			Outcome: audit.Failure, Detail: strings.Join(c.Domains, ", ") + ": " + truncate(err.Error(), 500),
		})
		slog.Warn("acme: issuance failed", "id", c.ID, "domains", c.Domains, "err", err)
		return
	}
	c.Status = StatusValid
	c.Failures = 0
	c.LastError = ""
	c.NextAttemptAt = time.Time{}
	c.NotBefore, c.NotAfter = res.leaf.NotBefore, res.leaf.NotAfter
	c.Fingerprint = res.fingerprint
	c.Issuer = res.leaf.Issuer.CommonName
	c.IssuedAt = now
	run.info("Concluído: válido até %s, renovação prevista para %s.", c.NotAfter.Format("02/01/2006"), m.RenewAt(c).Format("02/01/2006"))
	slog.Info("acme: certificate issued", "id", c.ID, "domains", c.Domains, "not_after", c.NotAfter)
}

// backoff respects the CA rate limits (5 failed validations per hour).
func backoff(failures int) time.Duration {
	d := time.Hour << min(failures-1, 5)
	return min(d, 24*time.Hour)
}

type issued struct {
	certPEM     string
	keyPEM      string
	leaf        *x509.Certificate
	fingerprint string
}

func (m *Manager) obtain(ctx context.Context, c *store.Certificate, run *runLog) (*issued, error) {
	client, err := m.client(ctx, c.Environment)
	if err != nil {
		return nil, fmt.Errorf("conta ACME: %w", err)
	}
	ids := make([]acme.AuthzID, 0, len(c.Domains))
	for _, d := range c.Domains {
		ids = append(ids, acme.AuthzID{Type: "dns", Value: d})
	}
	order, err := client.AuthorizeOrder(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("criar pedido: %w", err)
	}
	run.info("Pedido criado na CA (%s) para %s", c.Environment, strings.Join(c.Domains, ", "))

	type pending struct {
		authzURL string
		chal     *acme.Challenge
		fqdn     string
		value    string
	}
	var todo []pending
	var records []string
	defer func() {
		// Challenge records are removed even when the issuance fails.
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		for _, ref := range records {
			if err := m.DNS.DeleteTXT(cctx, ref); err != nil {
				run.warn("Não foi possível remover um registro TXT temporário: %s", err)
			}
		}
		if len(records) > 0 {
			run.info("Registros TXT temporários removidos")
		}
	}()

	for _, u := range order.AuthzURLs {
		z, err := client.GetAuthorization(ctx, u)
		if err != nil {
			return nil, fmt.Errorf("obter autorização: %w", err)
		}
		name := strings.TrimPrefix(z.Identifier.Value, "*.")
		if z.Status == acme.StatusValid {
			run.info("%s já está validado na CA", z.Identifier.Value)
			continue
		}
		var chal *acme.Challenge
		for _, ch := range z.Challenges {
			if ch.Type == "dns-01" {
				chal = ch
			}
		}
		if chal == nil {
			return nil, fmt.Errorf("a CA não ofereceu o desafio dns-01 para %s", z.Identifier.Value)
		}
		value, err := client.DNS01ChallengeRecord(chal.Token)
		if err != nil {
			return nil, err
		}
		fqdn := "_acme-challenge." + name
		ref, err := m.DNS.CreateTXT(ctx, fqdn, value)
		if err != nil {
			return nil, fmt.Errorf("criar TXT %s: %w", fqdn, err)
		}
		records = append(records, ref)
		run.info("TXT %s criado na Cloudflare", fqdn)
		todo = append(todo, pending{authzURL: u, chal: chal, fqdn: fqdn, value: value})
	}

	for _, p := range todo {
		m.waitPropagation(ctx, p.fqdn, p.value, run)
	}
	for _, p := range todo {
		if _, err := client.Accept(ctx, p.chal); err != nil {
			return nil, fmt.Errorf("aceitar desafio: %w", err)
		}
	}
	for _, p := range todo {
		if _, err := client.WaitAuthorization(ctx, p.authzURL); err != nil {
			return nil, fmt.Errorf("validação de %s recusada pela CA: %w", p.fqdn, err)
		}
	}
	if len(todo) > 0 {
		run.info("Desafio DNS validado pela CA")
	}

	order, err = client.WaitOrder(ctx, order.URI)
	if err != nil {
		return nil, fmt.Errorf("pedido não ficou pronto: %w", err)
	}
	key, keyPEM, err := newKey(c.KeyType)
	if err != nil {
		return nil, err
	}
	tmpl := &x509.CertificateRequest{DNSNames: c.Domains}
	if len(c.Domains[0]) <= 64 {
		tmpl.Subject = pkix.Name{CommonName: c.Domains[0]}
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, tmpl, key)
	if err != nil {
		return nil, err
	}
	der, _, err := client.CreateCertFromOrder(ctx, order, csr, true)
	if err != nil {
		return nil, fmt.Errorf("finalizar pedido: %w", err)
	}
	res, err := verifyIssued(der, key, c.Domains)
	if err != nil {
		return nil, err
	}
	res.keyPEM = keyPEM
	run.info("Certificado emitido por %s (chave %s, SHA-256 %s)", res.leaf.Issuer.CommonName, c.KeyType, shortFP(res.fingerprint))
	return res, nil
}

// verifyIssued checks the certificate matches the key and covers every
// domain before it is published.
func verifyIssued(der [][]byte, key crypto.Signer, domains []string) (*issued, error) {
	if len(der) == 0 {
		return nil, errors.New("a CA não retornou certificado")
	}
	leaf, err := x509.ParseCertificate(der[0])
	if err != nil {
		return nil, fmt.Errorf("certificado inválido: %w", err)
	}
	pub, ok := leaf.PublicKey.(interface{ Equal(crypto.PublicKey) bool })
	if !ok || !pub.Equal(key.Public()) {
		return nil, errors.New("o certificado emitido não corresponde à chave gerada")
	}
	for _, d := range domains {
		if !slices.Contains(leaf.DNSNames, d) {
			return nil, fmt.Errorf("o certificado emitido não cobre %s", d)
		}
	}
	if time.Now().After(leaf.NotAfter) {
		return nil, errors.New("o certificado emitido já está vencido")
	}
	var chain strings.Builder
	for _, b := range der {
		_ = pem.Encode(&chain, &pem.Block{Type: "CERTIFICATE", Bytes: b}) // strings.Builder writes never fail
	}
	sum := sha256.Sum256(der[0])
	return &issued{certPEM: chain.String(), leaf: leaf, fingerprint: hex.EncodeToString(sum[:])}, nil
}

func newKey(kind string) (crypto.Signer, string, error) {
	switch kind {
	case KeyRSA2048:
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, "", err
		}
		return k, string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)})), nil
	default:
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, "", err
		}
		b, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			return nil, "", err
		}
		return k, string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: b})), nil
	}
}

// publish stores the certificate in APISIX, which applies it without reload.
func (m *Manager) publish(ctx context.Context, c *store.Certificate, res *issued, who Actor, run *runLog) error {
	path := "ssls/" + c.SSLID
	var before any
	if resp, err := m.APISIX.Do(ctx, http.MethodGet, path, nil, nil); err == nil && resp.OK() {
		before = envelopeValue(resp.Body)
	}
	body, _ := json.Marshal(map[string]any{
		"cert":   res.certPEM,
		"key":    res.keyPEM,
		"snis":   c.Domains,
		"type":   "server",
		"labels": map[string]string{LabelManagedBy: ManagedByValue, LabelCertID: strconv.FormatInt(c.ID, 10)},
	})
	resp, err := m.APISIX.Do(ctx, http.MethodPut, path, nil, body)
	if err != nil {
		return fmt.Errorf("publicar no APISIX: %w", err)
	}
	if !resp.OK() {
		return fmt.Errorf("publicar no APISIX: HTTP %d %s", resp.Status, adminError(resp.Body))
	}
	after := envelopeValue(resp.Body)
	run.info("Publicado no APISIX como SSL %s (aplicado sem reload)", c.SSLID)

	e := store.AuditEvent{
		Actor: who.Name, IP: who.IP, Action: "acme.issue", Resource: "ssls", ResourceID: c.SSLID, Outcome: audit.Success,
		Detail: fmt.Sprintf("%s · válido até %s · SHA-256 %s", strings.Join(c.Domains, ", "), res.leaf.NotAfter.Format("02/01/2006"), shortFP(res.fingerprint)),
	}
	if before != nil {
		e.Before, _ = json.Marshal(apisix.Mask("ssls", before, nil))
	}
	if after != nil {
		e.After, _ = json.Marshal(apisix.Mask("ssls", after, nil))
	}
	m.Audit.Record(ctx, e)
	return nil
}

// Release stops managing c. With deleteSSL the SSL object is removed from
// APISIX; otherwise it is kept (and keeps working) as a regular certificate.
func (m *Manager) Release(ctx context.Context, c *store.Certificate, deleteSSL bool, who Actor) error {
	if m.Busy(c.ID) {
		return ErrBusy
	}
	path := "ssls/" + c.SSLID
	detail := strings.Join(c.Domains, ", ")
	if deleteSSL {
		resp, err := m.APISIX.Do(ctx, http.MethodDelete, path, nil, nil)
		if err != nil {
			return fmt.Errorf("excluir SSL no APISIX: %w", err)
		}
		if !resp.OK() && resp.Status != http.StatusNotFound {
			return fmt.Errorf("excluir SSL no APISIX: HTTP %d %s", resp.Status, adminError(resp.Body))
		}
		detail += " · SSL excluído do APISIX"
	} else {
		// Drop the "managed" labels; the certificate keeps being served.
		resp, err := m.APISIX.Do(ctx, http.MethodPatch, path+"/labels", nil, []byte(`{}`))
		if err == nil && !resp.OK() && resp.Status != http.StatusNotFound {
			slog.Warn("acme: could not clear labels", "ssl", c.SSLID, "status", resp.Status, "err", adminError(resp.Body))
		}
		detail += " · SSL mantido no APISIX sem renovação"
	}
	if err := m.Store.DeleteCertificate(ctx, c.ID); err != nil {
		return err
	}
	m.Audit.Record(ctx, store.AuditEvent{
		Actor: who.Name, IP: who.IP, Action: "acme.delete", Resource: "ssls", ResourceID: c.SSLID, Outcome: audit.Success, Detail: detail,
	})
	return nil
}

// client returns the ACME client for an environment, registering the
// account on first use.
func (m *Manager) client(ctx context.Context, env string) (*acme.Client, error) {
	dir, ok := m.Directories[env]
	if !ok {
		return nil, fmt.Errorf("ambiente %q desconhecido", env)
	}
	m.clientsMu.Lock()
	defer m.clientsMu.Unlock()
	if c := m.clients[dir]; c != nil {
		return c, nil
	}
	ad := []byte("acme-account:" + dir)
	c := &acme.Client{DirectoryURL: dir, HTTPClient: m.HTTPClient, UserAgent: "apisix-dashboard"}

	acct, err := m.Store.GetACMEAccount(ctx, dir)
	switch {
	case err == nil:
		der, err := m.AEAD.Open(acct.Key, ad)
		if err != nil {
			return nil, fmt.Errorf("decifrar chave da conta: %w", err)
		}
		k, err := x509.ParsePKCS8PrivateKey(der)
		if err != nil {
			return nil, err
		}
		c.Key = k.(crypto.Signer)
		c.KID = acme.KeyID(acct.URI)
	case errors.Is(err, store.ErrNotFound):
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}
		c.Key = k
		a := &acme.Account{}
		if m.Email != "" {
			a.Contact = []string{"mailto:" + m.Email}
		}
		reg, err := c.Register(ctx, a, acme.AcceptTOS)
		if err != nil && !errors.Is(err, acme.ErrAccountAlreadyExists) {
			return nil, err
		}
		der, err := x509.MarshalPKCS8PrivateKey(k)
		if err != nil {
			return nil, err
		}
		acct = &store.ACMEAccount{Directory: dir, Key: m.AEAD.Seal(der, ad), CreatedAt: time.Now()}
		if reg != nil {
			acct.URI = reg.URI
			c.KID = acme.KeyID(reg.URI)
		}
		if err := m.Store.SaveACMEAccount(ctx, acct); err != nil {
			return nil, err
		}
		slog.Info("acme: account registered", "directory", dir)
	default:
		return nil, err
	}
	m.clients[dir] = c
	return c, nil
}

// TermsOfService returns the CA terms URL of an environment, if any.
func (m *Manager) TermsOfService(ctx context.Context, env string) string {
	dir, ok := m.Directories[env]
	if !ok {
		return ""
	}
	d, err := (&acme.Client{DirectoryURL: dir, HTTPClient: m.HTTPClient}).Discover(ctx)
	if err != nil {
		return ""
	}
	return d.Terms
}

// waitPropagation polls the authoritative name servers until the TXT record
// is visible. A timeout is only a warning: the CA may still see it.
func (m *Manager) waitPropagation(ctx context.Context, fqdn, value string, run *runLog) {
	var servers []string
	if m.Resolver != "" {
		servers = []string{m.Resolver}
	} else if zone, err := m.DNS.ZoneFor(ctx, fqdn); err == nil {
		for _, ns := range zone.NameServers {
			servers = append(servers, net.JoinHostPort(ns, "53"))
		}
	}
	if len(servers) == 0 {
		run.warn("Sem servidor DNS para conferir a propagação de %s; aguardando 15s", fqdn)
		sleep(ctx, 15*time.Second)
		return
	}
	deadline := time.Now().Add(m.PropagationTimeout)
	for {
		missing := ""
		for _, srv := range servers {
			if !hasTXT(ctx, srv, fqdn, value) {
				missing = srv
				break
			}
		}
		if missing == "" {
			run.info("TXT %s visível nos servidores DNS autoritativos", fqdn)
			return
		}
		if time.Now().After(deadline) {
			run.warn("Propagação de %s não confirmada em %s (%s); seguindo com a validação", fqdn, m.PropagationTimeout, missing)
			return
		}
		if !sleep(ctx, 5*time.Second) {
			return
		}
	}
}

func hasTXT(ctx context.Context, server, fqdn, value string) bool {
	r := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		d := net.Dialer{Timeout: 5 * time.Second}
		return d.DialContext(ctx, network, server)
	}}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	txts, err := r.LookupTXT(ctx, fqdn)
	return err == nil && slices.Contains(txts, value)
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func envelopeValue(body []byte) any {
	v, err := apisix.Decode(body)
	if err != nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return m["value"]
	}
	return nil
}

func adminError(body []byte) string {
	var e struct {
		ErrorMsg string `json:"error_msg"`
	}
	_ = json.Unmarshal(body, &e) // non-JSON body: no message to show
	return truncate(e.ErrorMsg, 300)
}

func shortFP(fp string) string {
	if len(fp) < 16 {
		return fp
	}
	return fp[:16]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

var (
	domainRe = regexp.MustCompile(`^(\*\.)?([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z][a-z0-9-]{0,61}[a-z0-9]$`)
	sslIDRe  = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,64}$`)
)

// NormalizeDomains lowercases, deduplicates and validates domain names.
func NormalizeDomains(in []string) ([]string, error) {
	var out []string
	for _, d := range in {
		d = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(d)), ".")
		if d == "" || slices.Contains(out, d) {
			continue
		}
		if len(d) > 253 || !domainRe.MatchString(d) {
			return nil, fmt.Errorf("domínio inválido: %q", d)
		}
		out = append(out, d)
	}
	if len(out) == 0 {
		return nil, errors.New("informe ao menos um domínio")
	}
	if len(out) > 100 {
		return nil, errors.New("no máximo 100 domínios por certificado")
	}
	return out, nil
}

func ValidSSLID(id string) bool { return sslIDRe.MatchString(id) }

// DefaultSSLID derives the APISIX SSL id from the first domain.
func DefaultSSLID(domain string) string {
	id := "acme-" + strings.ReplaceAll(strings.Replace(domain, "*", "wildcard", 1), ".", "-")
	return truncate(id, 64)
}
