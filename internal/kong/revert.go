package kong

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/carloseduardo21/apisix-dashboard/internal/apisix"
	"github.com/carloseduardo21/apisix-dashboard/internal/audit"
	"github.com/carloseduardo21/apisix-dashboard/internal/i18n"
	"github.com/carloseduardo21/apisix-dashboard/internal/store"
)

// Revert actions, decided per migrated object.
const (
	RevertDelete  = "delete"  // created by the migration: delete it
	RevertRestore = "restore" // replaced an existing object: put the original back
	RevertManual  = "manual"  // cannot be reverted automatically (see Reason)
	RevertMissing = "missing" // no longer in APISIX: only the record is dropped
)

// MaxRevertItems bounds one revert request.
const MaxRevertItems = 5000

// Audit actions that write migrated objects (kong.migrate) or put them back
// to their pre-migration state (kong.rollback, kong.revert).
var migrationActions = []string{"kong.migrate", "kong.rollback", "kong.revert"}

// Revert order: dependents first, the reverse of the apply order.
var revertOrder = []string{KindRoute, KindGlobal, KindCredential, KindConsumer, KindService, KindUpstream}

// RevertItem is what reverting one migrated object would do.
type RevertItem struct {
	RecordID int64  `json:"record_id"`
	Kind     string `json:"kind"`
	KongName string `json:"kong_name"`
	Path     string `json:"path"`
	Action   string `json:"action"`
	// Changed: the object was modified in APISIX after its last migration
	// write. Such objects are only reverted when explicitly included.
	Changed bool          `json:"changed"`
	Reason  *i18n.Message `json:"reason,omitempty"`

	record   *store.KongMigration
	original map[string]any // pre-migration object (restore)
	current  map[string]any
}

// RevertPlan lists the selected objects in the order they would be reverted.
type RevertPlan struct {
	Items []*RevertItem `json:"items"`
	Hash  string        `json:"hash"`
}

// Affected counts the objects a revert would delete or restore.
func (p *RevertPlan) Affected(includeChanged bool) int {
	n := 0
	for _, it := range p.Items {
		if it.acts(includeChanged) {
			n++
		}
	}
	return n
}

func (it *RevertItem) acts(includeChanged bool) bool {
	return (it.Action == RevertDelete || it.Action == RevertRestore) && (includeChanged || !it.Changed)
}

// RevertPlan works out, from the audit trail, how each selected migration
// record can be undone.
func (m *Migrator) RevertPlan(ctx context.Context, recordIDs []int64) (*RevertPlan, error) {
	if len(recordIDs) == 0 || len(recordIDs) > MaxRevertItems {
		return nil, &ValidationError{i18n.E("kong.revert.selection", "select between 1 and {max} migrated objects", "max", MaxRevertItems)}
	}
	records, err := m.Store.ListKongMigrations(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]*store.KongMigration, len(records))
	for _, r := range records {
		byID[r.ID] = r
	}
	plan := &RevertPlan{Items: []*RevertItem{}}
	seen := map[int64]bool{}
	for _, id := range recordIDs {
		r := byID[id]
		if r == nil {
			return nil, &ValidationError{i18n.E("kong.revert.unknown_record", "migration record {id} does not exist (reload the history)", "id", id)}
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		it, err := m.revertItem(ctx, r)
		if err != nil {
			return nil, err
		}
		plan.Items = append(plan.Items, it)
	}
	// Deterministic order (the hash must not depend on how the ids were sent).
	slices.SortFunc(plan.Items, func(a, b *RevertItem) int {
		if d := slices.Index(revertOrder, a.Kind) - slices.Index(revertOrder, b.Kind); d != 0 {
			return d
		}
		return strings.Compare(a.Path, b.Path)
	})
	h := sha256.New()
	for _, it := range plan.Items {
		_ = json.NewEncoder(h).Encode([]any{it.RecordID, it.Path, it.Action, it.Changed, it.current}) // hash writes never fail
	}
	plan.Hash = hex.EncodeToString(h.Sum(nil))
	return plan, nil
}

func (m *Migrator) revertItem(ctx context.Context, r *store.KongMigration) (*RevertItem, error) {
	it := &RevertItem{RecordID: r.ID, Kind: r.Kind, KongName: r.KongName, Path: r.Path(), record: r}

	resp, err := m.APISIX.Do(ctx, http.MethodGet, escapePath(r.Path()), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("apisix: %w", err)
	}
	switch {
	case resp.Status == http.StatusNotFound:
		it.Action = RevertMissing
		return it, nil
	case !resp.OK():
		return nil, fmt.Errorf("apisix: GET %s: HTTP %d", r.Path(), resp.Status)
	}
	var env struct {
		Value map[string]any `json:"value"`
	}
	if err := json.Unmarshal(resp.Body, &env); err != nil || env.Value == nil {
		return nil, fmt.Errorf("apisix: GET %s: unexpected body", r.Path())
	}
	it.current = env.Value

	// The first migration write since the object was last put back tells
	// what existed before; the latest write tells what the migration left.
	events, err := m.Store.ObjectHistory(ctx, r.Resource, r.APISIXID, migrationActions...)
	if err != nil {
		return nil, err
	}
	var first, last *store.AuditEvent
	for _, e := range events {
		if e.Action != "kong.migrate" {
			first, last = nil, nil
			continue
		}
		if first == nil {
			first = e
		}
		last = e
	}
	if first == nil {
		it.Action = RevertManual
		it.Reason = msgPtr(i18n.M("kong.revert.no_audit", "no migration write in the audit trail; revert it manually"))
		return it, nil
	}
	it.Changed = changedSince(last, it.current)
	if len(first.Before) == 0 {
		it.Action = RevertDelete
		return it, nil
	}
	var original map[string]any
	if err := json.Unmarshal(first.Before, &original); err != nil || original == nil {
		it.Action = RevertManual
		it.Reason = msgPtr(i18n.M("kong.revert.unreadable_original", "the original object in the audit trail is unreadable; revert it manually"))
		return it, nil
	}
	if containsMasked(original) {
		it.Action = RevertManual
		it.Reason = msgPtr(i18n.M("kong.revert.masked_original", "the original object had secrets, which the audit trail keeps masked; restore it manually from the audit trail"))
		return it, nil
	}
	it.Action, it.original = RevertRestore, original
	return it, nil
}

// changedSince reports whether current differs from what the last migration
// write left, using APISIX's update_time (unknown counts as changed).
func changedSince(last *store.AuditEvent, current map[string]any) bool {
	var after map[string]any
	if last == nil || json.Unmarshal(last.After, &after) != nil || after == nil {
		return true
	}
	a, b := fmt.Sprint(after["update_time"]), fmt.Sprint(current["update_time"])
	return a == "<nil>" || a != b
}

func containsMasked(v any) bool {
	switch t := v.(type) {
	case string:
		return t == apisix.Masked
	case map[string]any:
		for _, x := range t {
			if containsMasked(x) {
				return true
			}
		}
	case []any:
		for _, x := range t {
			if containsMasked(x) {
				return true
			}
		}
	}
	return false
}

func msgPtr(m i18n.Message) *i18n.Message { return &m }

// RevertResult is the outcome of reverting one object.
type RevertResult struct {
	Path    string        `json:"path"`
	Kind    string        `json:"kind"`
	Action  string        `json:"action"`
	Outcome string        `json:"outcome"` // success, failure, skipped
	Error   *i18n.Message `json:"error,omitempty"`
}

// Revert undoes the selected migration records if the plan still matches
// hash and confirm equals the number of objects that will be deleted or
// restored (a second, server-side confirmation).
func (m *Migrator) Revert(ctx context.Context, recordIDs []int64, hash string, includeChanged bool, confirm int, a Actor) ([]RevertResult, error) {
	m.mu.Lock()
	if m.job != nil && m.job.Status == JobRunning || m.reverting {
		m.mu.Unlock()
		return nil, ErrBusy
	}
	m.reverting = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.reverting = false
		m.mu.Unlock()
	}()

	plan, err := m.RevertPlan(ctx, recordIDs)
	if err != nil {
		return nil, err
	}
	if plan.Hash != hash {
		return nil, ErrStale
	}
	if n := plan.Affected(includeChanged); confirm != n {
		return nil, &ValidationError{i18n.E("kong.revert.confirm", "confirmation mismatch: type {count} to revert {count} objects", "count", n)}
	}

	// The revert must finish even if the browser goes away.
	ctx = context.WithoutCancel(ctx)
	results := make([]RevertResult, 0, len(plan.Items))
	ok, failed := 0, 0
	for _, it := range plan.Items {
		res := RevertResult{Path: it.Path, Kind: it.Kind, Action: it.Action, Outcome: "skipped"}
		switch {
		case it.Action == RevertMissing:
			if err := m.Store.DeleteKongMigration(ctx, it.record.Kind, it.record.KongID); err != nil {
				return results, err
			}
			res.Outcome = "success"
		case it.acts(includeChanged):
			if err := m.revertOne(ctx, it, a); err != nil {
				res.Outcome = "failure"
				msg := i18n.M("kong.revert.failed", "{error}", "error", err.Error())
				res.Error = &msg
				failed++
			} else {
				res.Outcome = "success"
				ok++
			}
		}
		results = append(results, res)
	}
	outcome := audit.Success
	if failed > 0 {
		outcome = audit.Failure
	}
	m.Audit.Record(ctx, store.AuditEvent{Actor: a.Name, IP: a.IP, Action: "kong.revert_batch", Resource: "kong_migration",
		Outcome: outcome, Detail: fmt.Sprintf("%d objects reverted, %d failed, %d selected", ok, failed, len(plan.Items))})
	return results, nil
}

func (m *Migrator) revertOne(ctx context.Context, it *RevertItem, a Actor) error {
	r := it.record
	var resp *apisix.Response
	var err error
	if it.Action == RevertDelete {
		resp, err = m.APISIX.Do(ctx, http.MethodDelete, escapePath(r.Path()), nil, nil)
	} else {
		prev := map[string]any{}
		for k, v := range it.original {
			if k != "create_time" && k != "update_time" {
				prev[k] = v
			}
		}
		payload, _ := json.Marshal(prev)
		path := escapePath(r.Path())
		if r.Kind == KindConsumer {
			path = "consumers"
		}
		resp, err = m.APISIX.Do(ctx, http.MethodPut, path, nil, payload)
	}
	switch {
	case err != nil:
		err = fmt.Errorf("admin api unreachable: %w", err)
	case it.Action == RevertDelete && resp.Status == http.StatusNotFound:
		// Already gone (e.g. credentials removed with their consumer).
	case !resp.OK():
		msg := http.StatusText(resp.Status)
		var e struct {
			ErrorMsg string `json:"error_msg"`
		}
		if json.Unmarshal(resp.Body, &e) == nil && e.ErrorMsg != "" {
			msg = e.ErrorMsg
		}
		err = fmt.Errorf("HTTP %d: %s", resp.Status, msg)
	}
	e := store.AuditEvent{Actor: a.Name, IP: a.IP, Action: "kong.revert", Resource: r.Resource, ResourceID: r.APISIXID,
		Outcome: audit.Success, Detail: "reverting migration of kong " + r.Kind + " " + r.KongName + " (" + it.Action + ")"}
	e.Before, _ = json.Marshal(m.Mask(r.Resource, it.current))
	if it.original != nil {
		e.After, _ = json.Marshal(m.Mask(r.Resource, it.original))
	}
	if err != nil {
		e.Outcome, e.Detail = audit.Failure, e.Detail+": "+err.Error()
	}
	e.Time = time.Now()
	m.Audit.Record(ctx, e)
	if err != nil {
		return err
	}
	return m.Store.DeleteKongMigration(ctx, r.Kind, r.KongID)
}
