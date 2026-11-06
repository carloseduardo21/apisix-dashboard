package kong

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/carloseduardo21/apisix-dashboard/internal/apisix"
	"github.com/carloseduardo21/apisix-dashboard/internal/audit"
	"github.com/carloseduardo21/apisix-dashboard/internal/i18n"
	"github.com/carloseduardo21/apisix-dashboard/internal/secure"
	"github.com/carloseduardo21/apisix-dashboard/internal/store"
)

const (
	// What applying an item does.
	ActionCreate    = "create"
	ActionUpdate    = "update"    // object created by this migration
	ActionOverwrite = "overwrite" // existing object not created by the migration
	ActionUnchanged = "unchanged"
	ActionConflict  = "conflict" // exists, not ours, skipped by the settings
	ActionBlocked   = "blocked"

	JobRunning = "running"
	JobSuccess = "success"
	JobFailed  = "failed"

	settingsKey = "kong_migration"
	fetchers    = 8
)

var (
	ErrBusy  = i18n.E("kong.busy", "a migration is already running")
	ErrStale = i18n.E("kong.stale", "Kong or APISIX changed since the preview; generate the preview again")
)

// Actor identifies who triggered an operation, for the audit trail.
type Actor struct {
	Name string
	IP   string
}

type Options struct {
	Store  *store.Store
	APISIX *apisix.Client
	Audit  *audit.Logger
	Kong   *Client
	// Mask hides secrets of an APISIX object (audit trail and UI).
	Mask func(resource string, v any) any
}

type Migrator struct {
	Options

	ctx context.Context
	mu  sync.Mutex
	job *Job
}

func NewMigrator(o Options) *Migrator {
	return &Migrator{Options: o, ctx: context.Background()}
}

// Run sets the context background jobs run in (cancelled on shutdown).
func (m *Migrator) Run(ctx context.Context) { m.ctx = ctx }

// ---- settings ----

func (m *Migrator) Settings(ctx context.Context) (Settings, error) {
	b, err := m.Store.GetSetting(ctx, settingsKey)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return Settings{}, err
	}
	return ParseSettings(b)
}

// ValidationError carries messages safe to show in the UI (i18n errors,
// possibly joined).
type ValidationError struct{ Err error }

func (e *ValidationError) Error() string { return e.Err.Error() }
func (e *ValidationError) Unwrap() error { return e.Err }

func (m *Migrator) SaveSettings(ctx context.Context, s Settings, a Actor) error {
	s.Normalize()
	if err := s.Validate(); err != nil {
		return &ValidationError{err}
	}
	before, err := m.Settings(ctx)
	if err != nil {
		return err
	}
	b, _ := json.Marshal(s)
	if err := m.Store.PutSetting(ctx, settingsKey, b, a.Name, time.Now()); err != nil {
		return err
	}
	prev, _ := json.Marshal(before)
	m.Audit.Record(ctx, store.AuditEvent{Actor: a.Name, IP: a.IP, Action: "kong.settings", Resource: "kong_migration",
		Outcome: audit.Success, Before: prev, After: b})
	return nil
}

// ---- plan ----

// PlanItem is an Item with what applying it would do.
type PlanItem struct {
	*Item
	Action     string         `json:"action"`
	Current    map[string]any `json:"current,omitempty"`
	MigratedAt *time.Time     `json:"migrated_at,omitempty"`
	MigratedBy string         `json:"migrated_by,omitempty"`
	// Changed: migrated before, but Kong (or the settings) changed since.
	Changed bool `json:"changed,omitempty"`
}

func (p *PlanItem) writes() bool {
	return p.Action == ActionCreate || p.Action == ActionUpdate || p.Action == ActionOverwrite
}

type Plan struct {
	Items []*PlanItem `json:"items"`
	// Hash binds an apply to the preview it was approved from.
	Hash string `json:"hash"`
}

// Plan translates the selection (all of Kong when sel is nil) and compares
// it with the current APISIX configuration.
func (m *Migrator) Plan(ctx context.Context, sel *Selection) (*Plan, error) {
	set, err := m.Settings(ctx)
	if err != nil {
		return nil, err
	}
	snap, err := m.Kong.Snapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("kong: %w", err)
	}
	if sel == nil {
		all := All(snap)
		sel = &all
	}
	items := Translate(snap, set, *sel)

	records, err := m.Store.ListKongMigrations(ctx)
	if err != nil {
		return nil, err
	}
	byPath := map[string]*store.KongMigration{}
	for _, r := range records {
		byPath[r.Path()] = r
	}
	current, err := m.current(ctx, items)
	if err != nil {
		return nil, err
	}

	plan := &Plan{Items: make([]*PlanItem, 0, len(items))}
	h := sha256.New()
	for _, it := range items {
		pi := &PlanItem{Item: it, Current: current[it.Path()]}
		rec := byPath[it.Path()]
		if rec != nil && rec.KongID == it.KongID {
			pi.MigratedAt, pi.MigratedBy = &rec.MigratedAt, rec.MigratedBy
			pi.Changed = rec.BodyHash != bodyHash(it.Body)
		} else {
			rec = nil
		}
		ours := rec != nil || get(pi.Current, "labels."+LabelKongID) == it.KongID
		if it.Kind == KindCredential && pi.Current == nil && !it.Blocked {
			if other := duplicateKey(it, current); other != "" {
				it.note(LevelError, i18n.M("kong.note.key_collision", "the same key-auth key is already in APISIX credential {credential} (credential re-created in Kong?); remove it first", "credential", other))
			}
		}
		switch {
		case it.Blocked:
			pi.Action = ActionBlocked
		case pi.Current == nil:
			pi.Action = ActionCreate
		case ours && subset(generic(it.Body), pi.Current):
			pi.Action = ActionUnchanged
		case ours:
			pi.Action = ActionUpdate
		case set.OnConflict == "overwrite":
			pi.Action = ActionOverwrite
		default:
			pi.Action = ActionConflict
		}
		plan.Items = append(plan.Items, pi)
		_ = json.NewEncoder(h).Encode([]any{it.Path(), pi.Action, it.Body, pi.Current}) // hash writes never fail
	}
	plan.Hash = hex.EncodeToString(h.Sum(nil))
	return plan, nil
}

// current loads the APISIX objects the items would write, by path.
func (m *Migrator) current(ctx context.Context, items []*Item) (map[string]map[string]any, error) {
	out := map[string]map[string]any{}
	resources := map[string]bool{}
	credOwners := map[string]bool{}
	for _, it := range items {
		if it.Kind == KindCredential {
			credOwners[strings.SplitN(it.ID, "/", 2)[0]] = true
		} else {
			resources[it.Resource] = true
		}
	}
	for r := range resources {
		if err := m.list(ctx, r, out); err != nil {
			return nil, err
		}
	}
	// Credentials are listed per consumer, only for consumers that exist.
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error
	sem := make(chan struct{}, fetchers)
	for u := range credOwners {
		if out["consumers/"+u] == nil {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			part := map[string]map[string]any{}
			err := m.list(ctx, "consumers/"+url.PathEscape(u)+"/credentials", part)
			mu.Lock()
			defer mu.Unlock()
			if err != nil && firstErr == nil {
				firstErr = err
			}
			for k, v := range part {
				out[k] = v
			}
		}()
	}
	wg.Wait()
	return out, firstErr
}

// list reads every page of an Admin API collection into out, keyed by path.
func (m *Migrator) list(ctx context.Context, path string, out map[string]map[string]any) error {
	for page := 1; ; page++ {
		resp, err := m.APISIX.Do(ctx, http.MethodGet, path, url.Values{"page": {strconv.Itoa(page)}, "page_size": {"500"}}, nil)
		if err != nil {
			return fmt.Errorf("apisix: %w", err)
		}
		if resp.Status == http.StatusNotFound {
			return nil
		}
		if !resp.OK() {
			return fmt.Errorf("apisix: GET %s: HTTP %d", path, resp.Status)
		}
		var body struct {
			Total int             `json:"total"`
			List  json.RawMessage `json:"list"`
		}
		if err := json.Unmarshal(resp.Body, &body); err != nil {
			return fmt.Errorf("apisix: GET %s: %w", path, err)
		}
		// An empty collection is encoded as {} (empty Lua table).
		var list []struct {
			Key   string         `json:"key"`
			Value map[string]any `json:"value"`
		}
		if len(body.List) > 0 && body.List[0] == '[' {
			if err := json.Unmarshal(body.List, &list); err != nil {
				return fmt.Errorf("apisix: GET %s: %w", path, err)
			}
		}
		for _, e := range list {
			out[strings.TrimPrefix(e.Key, "/apisix/")] = e.Value
		}
		if len(list) == 0 || page*500 >= body.Total {
			return nil
		}
	}
}

// duplicateKey finds another credential of the same consumer with the same
// key-auth key, which APISIX rejects.
func duplicateKey(it *Item, current map[string]map[string]any) string {
	key, _ := get(it.Body, "plugins.key-auth.key").(string)
	if key == "" {
		return ""
	}
	prefix := it.Resource + "/" + strings.SplitN(it.ID, "/", 2)[0] + "/credentials/"
	for path, v := range current {
		if strings.HasPrefix(path, prefix) && path != it.Path() && get(v, "plugins.key-auth.key") == key {
			return strings.TrimPrefix(path, prefix)
		}
	}
	return ""
}

// subset reports whether every field of want has the same value in have:
// APISIX adds schema defaults to what it stores.
func subset(want, have any) bool {
	switch w := want.(type) {
	case map[string]any:
		h, ok := have.(map[string]any)
		if !ok {
			return false
		}
		for k, v := range w {
			if !subset(v, h[k]) {
				return false
			}
		}
		return true
	case []any:
		h, ok := have.([]any)
		if !ok || len(h) != len(w) {
			return false
		}
		for i := range w {
			if !subset(w[i], h[i]) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(want, have)
	}
}

func generic(v any) any {
	b, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(b, &out) // round-trip of a value we just marshaled
	return out
}

func get(v any, path string) any {
	for _, seg := range strings.Split(path, ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[seg]
	}
	return v
}

func bodyHash(body map[string]any) string {
	b, _ := json.Marshal(body)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// ---- apply ----

// LogEntry is one line of the apply log. Message is English; the UI
// translates Code/Params.
type LogEntry struct {
	TS      time.Time         `json:"ts"`
	Level   string            `json:"level"`
	Message string            `json:"message"`
	Code    string            `json:"code,omitempty"`
	Params  map[string]string `json:"params,omitempty"`
}

type Result struct {
	Kind     string `json:"kind"`
	KongName string `json:"kong_name"`
	Path     string `json:"path"`
	Action   string `json:"action"`
	Outcome  string `json:"outcome"` // success, failure, rolled_back, skipped
	Error    string `json:"error,omitempty"`
}

// Job is one apply run. Only the last one is kept (in memory); the audit
// trail and the migration records are the durable history.
type Job struct {
	ID         string        `json:"id"`
	Status     string        `json:"status"`
	Actor      string        `json:"actor"`
	StartedAt  time.Time     `json:"started_at"`
	FinishedAt *time.Time    `json:"finished_at,omitempty"`
	Total      int           `json:"total"`
	Done       int           `json:"done"`
	Error      string        `json:"error,omitempty"`
	ErrorMsg   *i18n.Message `json:"error_message,omitempty"`
	Log        []LogEntry    `json:"log"`
	Results    []Result      `json:"results"`
}

// Job returns a copy of the last apply run, or nil.
func (m *Migrator) Job() *Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.job == nil {
		return nil
	}
	j := *m.job
	// Copies, never nil: the UI expects arrays.
	j.Log = append(make([]LogEntry, 0, len(j.Log)), j.Log...)
	j.Results = append(make([]Result, 0, len(j.Results)), j.Results...)
	return &j
}

func (m *Migrator) update(fn func(j *Job)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fn(m.job)
}

func (m *Migrator) logf(level, code, template string, kv ...any) {
	msg := i18n.M(code, template, kv...)
	m.update(func(j *Job) {
		j.Log = append(j.Log, LogEntry{TS: time.Now(), Level: level, Message: msg.String(), Code: msg.Code, Params: msg.Params})
	})
}

// Apply re-plans the selection and, if it still matches the approved
// preview, writes it to APISIX in the background.
func (m *Migrator) Apply(ctx context.Context, sel Selection, hash string, a Actor) (*Job, error) {
	m.mu.Lock()
	if m.job != nil && m.job.Status == JobRunning {
		m.mu.Unlock()
		return nil, ErrBusy
	}
	// Reserve the slot while planning.
	m.job = &Job{ID: secure.RandomToken(9), Status: JobRunning, Actor: a.Name, StartedAt: time.Now(), Log: []LogEntry{}, Results: []Result{}}
	m.mu.Unlock()

	fail := func(err error) (*Job, error) {
		m.update(func(j *Job) {
			now := time.Now()
			j.Status, j.Error, j.FinishedAt = JobFailed, err.Error(), &now
		})
		return nil, err
	}
	plan, err := m.Plan(ctx, &sel)
	if err != nil {
		return fail(err)
	}
	if plan.Hash != hash {
		return fail(ErrStale)
	}
	total := 0
	for _, it := range plan.Items {
		if it.writes() {
			total++
		}
	}
	if total == 0 {
		return fail(&ValidationError{i18n.E("kong.nothing_to_apply", "nothing to apply: the selected items are blocked, in conflict or unchanged")})
	}
	m.update(func(j *Job) { j.Total = total })
	go m.run(plan, a)
	return m.Job(), nil
}

type applied struct {
	item   *PlanItem
	before map[string]any
	record *store.KongMigration // previous migration record, if any
}

func (m *Migrator) run(plan *Plan, a Actor) {
	ctx := m.ctx
	records, _ := m.Store.ListKongMigrations(ctx)
	prevRecord := map[string]*store.KongMigration{}
	for _, r := range records {
		prevRecord[r.Kind+"/"+r.KongID] = r
	}

	m.logf(LevelInfo, "kong.log.applying", "applying {count} objects to APISIX", "count", m.Job().Total)
	var done []applied
	for _, it := range plan.Items {
		if !it.writes() {
			m.update(func(j *Job) {
				j.Results = append(j.Results, Result{Kind: it.Kind, KongName: it.KongName, Path: it.Path(), Action: it.Action, Outcome: "skipped"})
			})
			continue
		}
		if err := m.write(ctx, it, a); err != nil {
			m.logf(LevelError, "kong.log.item_failed", "{kind} {name}: {error}", "kind", it.Kind, "name", it.KongName, "error", err.Error())
			m.update(func(j *Job) {
				j.Results = append(j.Results, Result{Kind: it.Kind, KongName: it.KongName, Path: it.Path(), Action: it.Action, Outcome: "failure", Error: err.Error()})
			})
			m.rollback(ctx, done, a)
			m.update(func(j *Job) {
				now := time.Now()
				j.Status, j.FinishedAt = JobFailed, &now
				msg := i18n.M("kong.job_failed", "{kind} {name} failed: {error}; the changes of this batch were rolled back", "kind", it.Kind, "name", it.KongName, "error", err.Error())
				j.Error, j.ErrorMsg = msg.String(), &msg
			})
			m.Audit.Record(ctx, store.AuditEvent{Actor: a.Name, IP: a.IP, Action: "kong.migration", Resource: "kong_migration",
				Outcome: audit.Failure, Detail: m.Job().Error})
			return
		}
		rec := &store.KongMigration{Kind: it.Kind, KongID: it.KongID, KongName: it.KongName, Resource: it.Resource, APISIXID: it.ID,
			BodyHash: bodyHash(it.Body), MigratedBy: a.Name, MigratedAt: time.Now()}
		if err := m.Store.UpsertKongMigration(ctx, rec); err != nil {
			slog.Error("kong migration: save record", "err", err)
		}
		done = append(done, applied{item: it, before: it.Current, record: prevRecord[it.Kind+"/"+it.KongID]})
		m.update(func(j *Job) {
			j.Done++
			j.Results = append(j.Results, Result{Kind: it.Kind, KongName: it.KongName, Path: it.Path(), Action: it.Action, Outcome: "success"})
		})
	}
	m.update(func(j *Job) {
		now := time.Now()
		j.Status, j.FinishedAt = JobSuccess, &now
	})
	m.logf(LevelInfo, "kong.log.done", "migration finished: {count} objects written", "count", len(done))
	m.Audit.Record(ctx, store.AuditEvent{Actor: a.Name, IP: a.IP, Action: "kong.migration", Resource: "kong_migration",
		Outcome: audit.Success, Detail: fmt.Sprintf("%d objects written", len(done))})
}

// writePath is where an item is PUT: consumers are written to the collection
// with the username in the body.
func writePath(it *Item) string {
	if it.Kind == KindConsumer {
		return "consumers"
	}
	return escapePath(it.Path())
}

func escapePath(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}

func (m *Migrator) write(ctx context.Context, it *PlanItem, a Actor) error {
	payload, _ := json.Marshal(it.Body)
	resp, err := m.APISIX.Do(ctx, http.MethodPut, writePath(it.Item), nil, payload)
	var after map[string]any
	detail := "kong " + it.Kind + " " + it.KongName + " (" + it.KongID + ")"
	outcome := audit.Success
	switch {
	case err != nil:
		outcome, err = audit.Failure, fmt.Errorf("admin api unreachable: %w", err)
	case !resp.OK():
		outcome = audit.Failure
		msg := http.StatusText(resp.Status)
		var e struct {
			ErrorMsg string `json:"error_msg"`
		}
		if json.Unmarshal(resp.Body, &e) == nil && e.ErrorMsg != "" {
			msg = e.ErrorMsg
		}
		err = fmt.Errorf("HTTP %d: %s", resp.Status, msg)
	default:
		var env struct {
			Value map[string]any `json:"value"`
		}
		_ = json.Unmarshal(resp.Body, &env) // non-JSON success body: audit records no "after"
		after = env.Value
	}
	if err != nil {
		detail += ": " + err.Error()
	}
	e := store.AuditEvent{Actor: a.Name, IP: a.IP, Action: "kong.migrate", Resource: it.Resource, ResourceID: it.ID, Outcome: outcome, Detail: detail}
	if it.Current != nil {
		e.Before, _ = json.Marshal(m.Mask(it.Resource, it.Current))
	}
	if after != nil {
		e.After, _ = json.Marshal(m.Mask(it.Resource, after))
	}
	m.Audit.Record(ctx, e)
	return err
}

// rollback undoes the objects written by this run, newest first: created
// objects are deleted, replaced ones restored.
func (m *Migrator) rollback(ctx context.Context, done []applied, a Actor) {
	if len(done) == 0 {
		return
	}
	m.logf(LevelWarn, "kong.log.rolling_back", "rolling back {count} objects written in this batch", "count", len(done))
	for i := len(done) - 1; i >= 0; i-- {
		d := done[i]
		it := d.item
		var resp *apisix.Response
		var err error
		if d.before == nil {
			resp, err = m.APISIX.Do(ctx, http.MethodDelete, escapePath(it.Path()), nil, nil)
		} else {
			prev := map[string]any{}
			for k, v := range d.before {
				if k != "create_time" && k != "update_time" {
					prev[k] = v
				}
			}
			payload, _ := json.Marshal(prev)
			resp, err = m.APISIX.Do(ctx, http.MethodPut, writePath(it.Item), nil, payload)
		}
		outcome := audit.Success
		if err == nil && !resp.OK() {
			err = fmt.Errorf("HTTP %d", resp.Status)
		}
		if err != nil {
			outcome = audit.Failure
			m.logf(LevelError, "kong.log.rollback_failed", "could not roll back {path}: {error} (fix it manually)", "path", it.Path(), "error", err.Error())
		}
		e := store.AuditEvent{Actor: a.Name, IP: a.IP, Action: "kong.rollback", Resource: it.Resource, ResourceID: it.ID, Outcome: outcome,
			Detail: "rolling back migration of kong " + it.Kind + " " + it.KongName}
		if d.before != nil {
			e.After, _ = json.Marshal(m.Mask(it.Resource, d.before))
		}
		m.Audit.Record(ctx, e)
		if err != nil {
			continue
		}
		var serr error
		if d.record != nil {
			serr = m.Store.UpsertKongMigration(ctx, d.record)
		} else {
			serr = m.Store.DeleteKongMigration(ctx, it.Kind, it.KongID)
		}
		if serr != nil {
			slog.Error("kong: rollback could not restore migration record", "kind", it.Kind, "kong_id", it.KongID, "err", serr)
		}
		m.update(func(j *Job) {
			for k := range j.Results {
				if j.Results[k].Path == it.Path() && j.Results[k].Outcome == "success" {
					j.Results[k].Outcome = "rolled_back"
				}
			}
		})
	}
}

// History lists what has been migrated.
func (m *Migrator) History(ctx context.Context) ([]*store.KongMigration, error) {
	return m.Store.ListKongMigrations(ctx)
}
