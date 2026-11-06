package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/carloseduardo21/apisix-dashboard/internal/i18n"
	"github.com/carloseduardo21/apisix-dashboard/internal/kong"
)

func (p *principal) kongActor() kong.Actor { return kong.Actor{Name: p.user.Username, IP: p.meta.IP} }

// kongOff answers when the migration is not configured.
func (s *Server) kongOff(w http.ResponseWriter) bool {
	if s.Kong == nil {
		writeError(w, http.StatusConflict, "kong.disabled", "Kong migration is disabled: set DASH_KONG_ADMIN_URL")
		return true
	}
	return false
}

func kongError(w http.ResponseWriter, err error) {
	var ve *kong.ValidationError
	switch {
	case errors.As(err, &ve):
		writeUserError(w, http.StatusBadRequest, err)
	case errors.Is(err, kong.ErrBusy), errors.Is(err, kong.ErrStale):
		writeUserError(w, http.StatusConflict, err)
	case strings.HasPrefix(err.Error(), "kong: "), strings.HasPrefix(err.Error(), "apisix: "):
		// Errors from the Admin APIs carry only paths and status codes.
		slog.Warn("kong migration", "err", err)
		writeError(w, http.StatusBadGateway, "kong.admin_api_error", "Admin API error: {detail}", "detail", err.Error())
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
		setMessage(out, i18n.M("kong.unreachable", "could not reach the Kong Admin API: {detail}", "detail", err.Error()))
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
	current, err := s.Kong.Settings(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	set.Normalize() // compare rules in the form they are stored in
	if err := s.restrictedKongRules(p.user.Role, set.PluginRules, current.PluginRules); err != nil {
		writeUserError(w, http.StatusForbidden, err)
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
		writeError(w, http.StatusBadRequest, "kong.selection_empty", "select at least one item")
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
		writeError(w, http.StatusBadRequest, "kong.preview_required", "select the items and generate the preview before applying")
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
	RecordID   int64     `json:"record_id"`
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
		out = append(out, kongRecordView{RecordID: m.ID, Kind: m.Kind, KongID: m.KongID, KongName: m.KongName, Resource: m.Resource, ID: m.APISIXID,
			MigratedBy: m.MigratedBy, MigratedAt: m.MigratedAt})
	}
	writeJSON(w, http.StatusOK, out)
}

type kongRevertRequest struct {
	RecordIDs      []int64 `json:"record_ids"`
	Hash           string  `json:"hash"`
	IncludeChanged bool    `json:"include_changed"`
	Confirm        int     `json:"confirm"`
}

// handleKongRevertPlan previews how the selected migration records would be
// undone (delete created objects, restore replaced ones).
func (s *Server) handleKongRevertPlan(w http.ResponseWriter, r *http.Request, p *principal) {
	if s.kongOff(w) {
		return
	}
	var req kongRevertRequest
	if !readJSON(w, r, &req) {
		return
	}
	plan, err := s.Kong.RevertPlan(r.Context(), req.RecordIDs)
	if err != nil {
		kongError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

// handleKongRevert undoes the selected migration records. The client sends
// the preview hash and, as a confirmation, the number of objects that will
// be deleted or restored.
func (s *Server) handleKongRevert(w http.ResponseWriter, r *http.Request, p *principal) {
	if s.kongOff(w) {
		return
	}
	var req kongRevertRequest
	if !readJSON(w, r, &req) {
		return
	}
	results, err := s.Kong.Revert(r.Context(), req.RecordIDs, req.Hash, req.IncludeChanged, req.Confirm, p.kongActor())
	if err != nil {
		kongError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}
