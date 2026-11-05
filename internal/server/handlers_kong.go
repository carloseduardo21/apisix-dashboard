package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"apisix-dashboard/internal/kong"
)

func (p *principal) kongActor() kong.Actor { return kong.Actor{Name: p.user.Username, IP: p.meta.IP} }

// kongOff answers when the migration is not configured.
func (s *Server) kongOff(w http.ResponseWriter) bool {
	if s.Kong == nil {
		writeError(w, http.StatusConflict, "migração do Kong desativada: defina DASH_KONG_ADMIN_URL")
		return true
	}
	return false
}

func kongError(w http.ResponseWriter, err error) {
	var ve *kong.ValidationError
	switch {
	case errors.As(err, &ve):
		writeError(w, http.StatusBadRequest, ve.Error())
	case errors.Is(err, kong.ErrBusy), errors.Is(err, kong.ErrStale):
		writeError(w, http.StatusConflict, err.Error())
	case strings.HasPrefix(err.Error(), "kong: "), strings.HasPrefix(err.Error(), "apisix: "):
		// Errors from the Admin APIs carry only paths and status codes.
		slog.Warn("kong migration", "err", err)
		writeError(w, http.StatusBadGateway, err.Error())
	default:
		internalError(w, err)
	}
}

func (s *Server) handleKong(w http.ResponseWriter, r *http.Request, p *principal) {
	if s.Kong == nil {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	set, err := s.Kong.Settings(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	out := map[string]any{"enabled": true, "url": s.Kong.Kong.URL(), "settings": set, "builtin_plugins": kong.BuiltinPlugins()}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if info, err := s.Kong.Kong.Info(ctx); err != nil {
		slog.Warn("kong admin api unreachable", "err", err)
		out["error"] = "Admin API do Kong inacessível: " + err.Error()
	} else {
		out["info"] = info
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleKongSettings(w http.ResponseWriter, r *http.Request, p *principal) {
	if s.kongOff(w) {
		return
	}
	var set kong.Settings
	if !readJSON(w, r, &set) {
		return
	}
	if err := s.Kong.SaveSettings(r.Context(), set, p.kongActor()); err != nil {
		kongError(w, err)
		return
	}
	saved, _ := s.Kong.Settings(r.Context())
	writeJSON(w, http.StatusOK, saved)
}

// maskPlan hides secrets (credentials, plugin passwords) before the plan
// reaches the browser. keepCurrent drops the current APISIX state otherwise.
func (s *Server) maskPlan(p *kong.Plan, keepCurrent bool) *kong.Plan {
	out := &kong.Plan{Hash: p.Hash, Items: make([]*kong.PlanItem, 0, len(p.Items))}
	for _, pi := range p.Items {
		it := *pi.Item
		it.Body, _ = s.mask(it.Resource, it.Body).(map[string]any)
		cp := *pi
		cp.Item = &it
		cp.Current = nil
		if keepCurrent && pi.Current != nil {
			cp.Current, _ = s.mask(it.Resource, pi.Current).(map[string]any)
		}
		out.Items = append(out.Items, &cp)
	}
	return out
}

func (s *Server) handleKongInventory(w http.ResponseWriter, r *http.Request, p *principal) {
	if s.kongOff(w) {
		return
	}
	plan, err := s.Kong.Plan(r.Context(), nil)
	if err != nil {
		kongError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.maskPlan(plan, false))
}

func (s *Server) handleKongPlan(w http.ResponseWriter, r *http.Request, p *principal) {
	if s.kongOff(w) {
		return
	}
	var sel kong.Selection
	if !readJSON(w, r, &sel) {
		return
	}
	if sel.Empty() {
		writeError(w, http.StatusBadRequest, "selecione ao menos um item")
		return
	}
	plan, err := s.Kong.Plan(r.Context(), &sel)
	if err != nil {
		kongError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.maskPlan(plan, true))
}

func (s *Server) handleKongApply(w http.ResponseWriter, r *http.Request, p *principal) {
	if s.kongOff(w) {
		return
	}
	var req struct {
		kong.Selection
		Hash string `json:"hash"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Selection.Empty() || req.Hash == "" {
		writeError(w, http.StatusBadRequest, "selecione os itens e gere a pré-visualização antes de aplicar")
		return
	}
	job, err := s.Kong.Apply(r.Context(), req.Selection, req.Hash, p.kongActor())
	if err != nil {
		kongError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleKongJob(w http.ResponseWriter, r *http.Request, p *principal) {
	if s.kongOff(w) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job": s.Kong.Job()})
}

type kongRecordView struct {
	Kind       string    `json:"kind"`
	KongID     string    `json:"kong_id"`
	KongName   string    `json:"kong_name"`
	Resource   string    `json:"resource"`
	ID         string    `json:"id"`
	MigratedBy string    `json:"migrated_by"`
	MigratedAt time.Time `json:"migrated_at"`
}

func (s *Server) handleKongHistory(w http.ResponseWriter, r *http.Request, p *principal) {
	if s.kongOff(w) {
		return
	}
	list, err := s.Kong.History(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]kongRecordView, 0, len(list))
	for _, m := range list {
		out = append(out, kongRecordView{Kind: m.Kind, KongID: m.KongID, KongName: m.KongName, Resource: m.Resource, ID: m.APISIXID,
			MigratedBy: m.MigratedBy, MigratedAt: m.MigratedAt})
	}
	writeJSON(w, http.StatusOK, out)
}
