package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/carloseduardo21/apisix-dashboard/internal/apisix"
	"github.com/carloseduardo21/apisix-dashboard/internal/audit"
	"github.com/carloseduardo21/apisix-dashboard/internal/auth"
	"github.com/carloseduardo21/apisix-dashboard/internal/store"
)

// Query parameters forwarded to the Admin API on list requests.
var listParams = []string{"page", "page_size", "name", "label", "uri"}

func badGateway(w http.ResponseWriter, err error) {
	slog.Error("admin api request failed", "err", err)
	writeError(w, http.StatusBadGateway, "falha ao contactar a Admin API do APISIX")
}

func (s *Server) mask(resource string, v any) any {
	return apisix.Mask(resource, v, s.Catalog.EncryptFields)
}

// handleSchemas serves the plugin and resource schemas the forms are built from.
func (s *Server) handleSchemas(w http.ResponseWriter, r *http.Request, p *principal) {
	data, err := s.Catalog.JSON(r.Context())
	if err != nil {
		badGateway(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(data) // client may have gone away; nothing to do
}

// writeAdminResponse relays an Admin API response with secrets masked.
func (s *Server) writeAdminResponse(w http.ResponseWriter, resource string, resp *apisix.Response) {
	v, err := apisix.Decode(resp.Body)
	if err != nil {
		// Not JSON (unexpected): do not relay raw content.
		writeError(w, resp.Status, http.StatusText(resp.Status))
		return
	}
	if resp.OK() {
		v = apisix.MaskEnvelope(resource, v, s.Catalog.EncryptFields)
		if resource == "ssls" {
			apisix.AnnotateCerts(v)
		}
	}
	writeJSON(w, resp.Status, v)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request, p *principal) {
	start := time.Now()
	resp, err := s.APISIX.Do(r.Context(), http.MethodGet, "routes", url.Values{"page": {"1"}, "page_size": {"10"}}, nil)
	out := map[string]any{"reachable": false, "latency_ms": time.Since(start).Milliseconds()}
	if err != nil {
		slog.Warn("admin api unreachable", "err", err)
		out["error"] = "Admin API inacessível"
	} else {
		out["reachable"] = resp.OK()
		out["server"] = resp.Server
		if !resp.OK() {
			out["error"] = "Admin API respondeu HTTP " + http.StatusText(resp.Status)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handlePlugins exposes the read-only plugin catalog: plugins/list and
// plugins/<name> (schema).
func (s *Server) handlePlugins(w http.ResponseWriter, r *http.Request, p *principal) {
	name := r.PathValue("name")
	if !apisix.ValidPluginName(name) {
		writeError(w, http.StatusNotFound, "não encontrado")
		return
	}
	q := url.Values{}
	if sub := r.URL.Query().Get("subsystem"); sub == "http" || sub == "stream" {
		q.Set("subsystem", sub)
	}
	resp, err := s.APISIX.Do(r.Context(), http.MethodGet, "plugins/"+name, q, nil)
	if err != nil {
		badGateway(w, err)
		return
	}
	// Re-encode instead of relaying raw bytes: only valid JSON reaches the browser.
	v, err := apisix.Decode(resp.Body)
	if err != nil {
		writeError(w, resp.Status, http.StatusText(resp.Status))
		return
	}
	writeJSON(w, resp.Status, v)
}

func (s *Server) handleAPISIX(w http.ResponseWriter, r *http.Request, p *principal) {
	path, ok := apisix.ParsePath(r.PathValue("path"))
	if !ok {
		writeError(w, http.StatusNotFound, "recurso não suportado")
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.apisixRead(w, r, path)
	case http.MethodPut, http.MethodPost, http.MethodPatch, http.MethodDelete:
		if !auth.CanWrite(p.user.Role) {
			writeError(w, http.StatusForbidden, "seu perfil não permite alterações")
			return
		}
		s.apisixWrite(w, r, p, path)
	default:
		writeError(w, http.StatusMethodNotAllowed, "método não permitido")
	}
}

func (s *Server) apisixRead(w http.ResponseWriter, r *http.Request, path apisix.Path) {
	q := url.Values{}
	if !path.IsItem() {
		for _, k := range listParams {
			if v := r.URL.Query().Get(k); v != "" {
				q.Set(k, v)
			}
		}
	}
	resp, err := s.APISIX.Do(r.Context(), http.MethodGet, path.String(), q, nil)
	if err != nil {
		badGateway(w, err)
		return
	}
	s.writeAdminResponse(w, path.Resource, resp)
}

var writeActions = map[string]string{
	http.MethodPut:    "apisix.put",
	http.MethodPost:   "apisix.create",
	http.MethodPatch:  "apisix.patch",
	http.MethodDelete: "apisix.delete",
}

// apisixWrite forwards a change, restoring masked secrets and recording the
// masked before/after state in the audit trail.
func (s *Server) apisixWrite(w http.ResponseWriter, r *http.Request, p *principal, path apisix.Path) {
	ctx := r.Context()

	var body any
	if r.Method != http.MethodDelete {
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodySize))
		if err != nil {
			writeError(w, http.StatusRequestEntityTooLarge, "corpo da requisição muito grande")
			return
		}
		body, err = apisix.Decode(raw)
		if _, isObj := body.(map[string]any); err != nil || !isObj {
			writeError(w, http.StatusBadRequest, "o corpo deve ser um objeto JSON")
			return
		}
	}

	// Consumers are created/updated with PUT /consumers and the username in
	// the body; resolve the item so the current state can be loaded.
	target := path
	if path.Resource == "consumers" && len(path.Segments) == 0 && r.Method == http.MethodPut {
		if name, ok := body.(map[string]any)["username"].(string); ok {
			if t, ok := path.Child(name); ok {
				target = t
			}
		}
	}

	// Certificates issued by the dashboard are only changed by the issuer, so
	// a manual edit cannot be silently overwritten by the next renewal.
	if target.Resource == "ssls" && target.IsItem() {
		if c, err := s.Store.GetCertificateBySSLID(ctx, target.ID()); err == nil {
			writeError(w, http.StatusConflict, "este SSL é emitido automaticamente ("+strings.Join(c.Domains, ", ")+"); gerencie-o em Certificados automáticos")
			return
		}
	}

	var before any
	if target.IsItem() {
		resp, err := s.APISIX.Do(ctx, http.MethodGet, target.String(), nil, nil)
		if err != nil {
			badGateway(w, err)
			return
		}
		if resp.OK() {
			if env, err := apisix.Decode(resp.Body); err == nil {
				if m, ok := env.(map[string]any); ok {
					before = m["value"]
				}
			}
		}
	}

	var payload []byte
	if body != nil {
		restored, err := apisix.Unmask(body, before)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		payload, _ = json.Marshal(restored)
	}

	q := url.Values{}
	if r.Method == http.MethodDelete && r.URL.Query().Get("force") == "true" {
		q.Set("force", "true")
	}
	resp, err := s.APISIX.Do(ctx, r.Method, path.String(), q, payload)
	if err != nil {
		s.recordChange(r, p, path, target, before, nil, audit.Failure, "admin api unreachable")
		badGateway(w, err)
		return
	}

	var after any
	detail := ""
	outcome := audit.Success
	if resp.OK() {
		if env, err := apisix.Decode(resp.Body); err == nil {
			if m, ok := env.(map[string]any); ok && r.Method != http.MethodDelete {
				after = m["value"]
			}
		}
	} else {
		outcome = audit.Failure
		detail = "HTTP " + http.StatusText(resp.Status)
		if env, err := apisix.Decode(resp.Body); err == nil {
			if m, ok := env.(map[string]any); ok {
				if msg, ok := m["error_msg"].(string); ok {
					detail += ": " + truncate(msg, 500)
				}
			}
		}
	}
	s.recordChange(r, p, path, target, before, after, outcome, detail)
	s.writeAdminResponse(w, path.Resource, resp)
}

func (s *Server) recordChange(r *http.Request, p *principal, path, target apisix.Path, before, after any, outcome, detail string) {
	id := target.ID()
	if id == "" {
		// POST without id: APISIX generated one.
		if m, ok := after.(map[string]any); ok {
			if v, ok := m["id"].(string); ok {
				id = v
			}
		}
	}
	e := store.AuditEvent{
		Actor: p.user.Username, IP: p.meta.IP, Action: writeActions[r.Method],
		Resource: path.Resource, ResourceID: id, Outcome: outcome, Detail: detail,
	}
	if before != nil {
		e.Before, _ = json.Marshal(s.mask(path.Resource, before))
	}
	if after != nil {
		e.After, _ = json.Marshal(s.mask(path.Resource, after))
	}
	s.Audit.Record(r.Context(), e)
}
