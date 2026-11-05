package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"apisix-dashboard/internal/acme"
	"apisix-dashboard/internal/audit"
	"apisix-dashboard/internal/store"
)

type certView struct {
	ID            int64           `json:"id"`
	Domains       []string        `json:"domains"`
	SSLID         string          `json:"ssl_id"`
	Environment   string          `json:"environment"`
	KeyType       string          `json:"key_type"`
	Status        string          `json:"status"`
	LastError     string          `json:"last_error,omitempty"`
	Log           json.RawMessage `json:"log"`
	NotBefore     *time.Time      `json:"not_before"`
	NotAfter      *time.Time      `json:"not_after"`
	RenewAt       *time.Time      `json:"renew_at"`
	Fingerprint   string          `json:"fingerprint,omitempty"`
	Issuer        string          `json:"issuer,omitempty"`
	Failures      int             `json:"failures"`
	NextAttemptAt *time.Time      `json:"next_attempt_at"`
	LastAttemptAt *time.Time      `json:"last_attempt_at"`
	IssuedAt      *time.Time      `json:"issued_at"`
	CreatedBy     string          `json:"created_by"`
	CreatedAt     time.Time       `json:"created_at"`
}

func optTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func (s *Server) viewCert(c *store.Certificate) certView {
	return certView{
		ID: c.ID, Domains: c.Domains, SSLID: c.SSLID, Environment: c.Environment, KeyType: c.KeyType,
		Status: c.Status, LastError: c.LastError, Log: c.Log,
		NotBefore: optTime(c.NotBefore), NotAfter: optTime(c.NotAfter), RenewAt: optTime(s.ACME.RenewAt(c)),
		Fingerprint: c.Fingerprint, Issuer: c.Issuer, Failures: c.Failures,
		NextAttemptAt: optTime(c.NextAttemptAt), LastAttemptAt: optTime(c.LastAttemptAt), IssuedAt: optTime(c.IssuedAt),
		CreatedBy: c.CreatedBy, CreatedAt: c.CreatedAt,
	}
}

func (p *principal) actor() acme.Actor { return acme.Actor{Name: p.user.Username, IP: p.meta.IP} }

// acmeOff answers when automatic issuance is not configured.
func (s *Server) acmeOff(w http.ResponseWriter) bool {
	if s.ACME == nil {
		writeError(w, http.StatusConflict, "emissão automática desativada: defina DASH_ACME_CLOUDFLARE_API_TOKEN")
		return true
	}
	return false
}

func (s *Server) handleACME(w http.ResponseWriter, r *http.Request, p *principal) {
	if s.ACME == nil {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false, "certificates": []any{}})
		return
	}
	certs, err := s.Store.ListCertificates(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	views := make([]certView, 0, len(certs))
	for _, c := range certs {
		views = append(views, s.viewCert(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":           true,
		"provider":          "cloudflare",
		"email":             s.ACME.Email,
		"renew_before_days": int(s.ACME.RenewBefore.Hours() / 24),
		"directories":       s.ACME.Directories,
		"certificates":      views,
	})
}

// handleACMEProvider checks the Cloudflare token and lists the zones it can
// edit, so the admin can tell right away whether issuance will work.
func (s *Server) handleACMEProvider(w http.ResponseWriter, r *http.Request, p *principal) {
	if s.acmeOff(w) {
		return
	}
	ctx := r.Context()
	out := map[string]any{"token_ok": false, "zones": []any{}}
	if err := s.ACME.DNS.Verify(ctx); err != nil {
		out["error"] = err.Error()
		writeJSON(w, http.StatusOK, out)
		return
	}
	out["token_ok"] = true
	zones, err := s.ACME.DNS.Zones(ctx)
	if err != nil {
		out["error"] = err.Error()
	} else {
		list := make([]map[string]string, 0, len(zones))
		for _, z := range zones {
			list = append(list, map[string]string{"name": z.Name, "status": z.Status})
		}
		out["zones"] = list
	}
	out["terms_of_service"] = s.ACME.TermsOfService(ctx, acme.EnvProduction)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateCertificate(w http.ResponseWriter, r *http.Request, p *principal) {
	if s.acmeOff(w) {
		return
	}
	var req struct {
		Domains     []string `json:"domains"`
		SSLID       string   `json:"ssl_id"`
		Environment string   `json:"environment"`
		KeyType     string   `json:"key_type"`
		Replace     bool     `json:"replace"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	domains, err := acme.NormalizeDomains(req.Domains)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Environment == "" {
		req.Environment = acme.EnvProduction
	}
	if req.Environment != acme.EnvProduction && req.Environment != acme.EnvStaging {
		writeError(w, http.StatusBadRequest, "ambiente deve ser production ou staging")
		return
	}
	if req.KeyType == "" {
		req.KeyType = acme.KeyEC256 // gitleaks:allow (constant name, not a secret)
	}
	if req.KeyType != acme.KeyEC256 && req.KeyType != acme.KeyRSA2048 {
		writeError(w, http.StatusBadRequest, "tipo de chave deve ser ec256 ou rsa2048")
		return
	}
	req.SSLID = strings.TrimSpace(req.SSLID)
	if req.SSLID == "" {
		req.SSLID = acme.DefaultSSLID(domains[0])
	}
	if !acme.ValidSSLID(req.SSLID) {
		writeError(w, http.StatusBadRequest, "ID do SSL inválido: use letras, números, '.', '_' ou '-' (até 64)")
		return
	}
	// Fail early when the token cannot edit a zone of these domains.
	for _, d := range domains {
		if _, err := s.ACME.DNS.ZoneFor(ctx, "_acme-challenge."+strings.TrimPrefix(d, "*.")); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	// An existing SSL object is only overwritten when explicitly asked: this
	// is how a manually managed certificate is migrated.
	resp, err := s.APISIX.Do(ctx, http.MethodGet, "ssls/"+req.SSLID, nil, nil)
	if err != nil {
		badGateway(w, err)
		return
	}
	if resp.OK() && !req.Replace {
		writeError(w, http.StatusConflict, "já existe um SSL com o ID "+req.SSLID+" no APISIX; escolha outro ID ou marque a opção de substituí-lo")
		return
	}

	now := time.Now()
	c := &store.Certificate{
		Domains: domains, SSLID: req.SSLID, Environment: req.Environment, KeyType: req.KeyType,
		Status: acme.StatusPending, CreatedBy: p.user.Username, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.Store.CreateCertificate(ctx, c); err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusConflict, "o SSL "+req.SSLID+" já é gerenciado por outro certificado automático")
			return
		}
		internalError(w, err)
		return
	}
	detail := strings.Join(domains, ", ") + " · " + req.Environment + " · " + req.KeyType
	if resp.OK() {
		detail += " · substitui o SSL existente na próxima emissão"
	}
	s.Audit.Record(ctx, store.AuditEvent{
		Actor: p.user.Username, IP: p.meta.IP, Action: "acme.create", Resource: "ssls", ResourceID: c.SSLID,
		Outcome: audit.Success, Detail: detail,
	})
	if err := s.ACME.Trigger(ctx, c.ID, p.actor()); err != nil {
		internalError(w, err)
		return
	}
	c, _ = s.Store.GetCertificate(ctx, c.ID)
	writeJSON(w, http.StatusCreated, s.viewCert(c))
}

func (s *Server) certFromPath(w http.ResponseWriter, r *http.Request) (*store.Certificate, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "não encontrado")
		return nil, false
	}
	c, err := s.Store.GetCertificate(r.Context(), id)
	if err != nil {
		handleErr(w, err)
		return nil, false
	}
	return c, true
}

func (s *Server) handleIssueCertificate(w http.ResponseWriter, r *http.Request, p *principal) {
	if s.acmeOff(w) {
		return
	}
	c, ok := s.certFromPath(w, r)
	if !ok {
		return
	}
	if err := s.ACME.Trigger(r.Context(), c.ID, p.actor()); err != nil {
		if errors.Is(err, acme.ErrBusy) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": acme.StatusIssuing})
}

func (s *Server) handleDeleteCertificate(w http.ResponseWriter, r *http.Request, p *principal) {
	if s.acmeOff(w) {
		return
	}
	c, ok := s.certFromPath(w, r)
	if !ok {
		return
	}
	err := s.ACME.Release(r.Context(), c, r.URL.Query().Get("delete_ssl") == "true", p.actor())
	switch {
	case errors.Is(err, acme.ErrBusy):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		writeError(w, http.StatusBadGateway, err.Error())
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
