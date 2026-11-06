package kong

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/carloseduardo21/apisix-dashboard/internal/apisix"
	"github.com/carloseduardo21/apisix-dashboard/internal/audit"
	"github.com/carloseduardo21/apisix-dashboard/internal/store"
)

// memAdmin is an in-memory Admin API: objects by path, update_time bumped on
// every write, calls recorded in order.
type memAdmin struct {
	mu    sync.Mutex
	objs  map[string]map[string]any
	calls []string
	clock int
}

func (a *memAdmin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	path := strings.TrimPrefix(r.URL.Path, "/apisix/admin/")
	if r.Method != http.MethodGet {
		a.calls = append(a.calls, r.Method+" "+path)
	}
	switch r.Method {
	case http.MethodGet:
		o, ok := a.objs[path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error_msg":"not found"}`)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"value": o})
	case http.MethodDelete:
		if path == "services/in-use" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error_msg":"can not delete this service directly"}`)
			return
		}
		delete(a.objs, path)
		_, _ = io.WriteString(w, `{"deleted":"1"}`)
	case http.MethodPut:
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if path == "consumers" {
			path += "/" + body["username"].(string)
		}
		a.clock++
		body["update_time"] = float64(1000 + a.clock)
		a.objs[path] = body
		_ = json.NewEncoder(w).Encode(map[string]any{"value": body})
	}
}

type revertFixture struct {
	m     *Migrator
	st    *store.Store
	admin *memAdmin
	ids   map[string]int64 // record id by APISIX path
}

func newRevertFixture(t *testing.T) *revertFixture {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "kong.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	admin := &memAdmin{objs: map[string]map[string]any{}}
	srv := httptest.NewServer(admin)
	t.Cleanup(srv.Close)
	client, err := apisix.NewClient(srv.URL, "k", "", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	al := audit.New(st, io.Discard)
	m := NewMigrator(Options{Store: st, APISIX: client, Audit: al,
		Mask: func(resource string, v any) any { return apisix.Mask(resource, v, nil) }})
	return &revertFixture{m: m, st: st, admin: admin, ids: map[string]int64{}}
}

// migrated records a migration of resource/id: the migration record plus
// the audit trail events (before = "" means it was created).
func (f *revertFixture) migrated(t *testing.T, kind, resource, id string, current map[string]any, events ...store.AuditEvent) {
	t.Helper()
	ctx := context.Background()
	if err := f.st.UpsertKongMigration(ctx, &store.KongMigration{Kind: kind, KongID: "k-" + id, KongName: id, Resource: resource,
		APISIXID: id, BodyHash: "h", MigratedBy: "op", MigratedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		e.Resource, e.ResourceID, e.Outcome = resource, id, audit.Success
		if err := f.st.InsertAudit(ctx, &e); err != nil {
			t.Fatal(err)
		}
	}
	if current != nil {
		f.admin.objs[resource+"/"+id] = current
	}
	list, _ := f.st.ListKongMigrations(ctx)
	for _, r := range list {
		f.ids[r.Path()] = r.ID
	}
}

func migrate(before, after string) store.AuditEvent {
	e := store.AuditEvent{Action: "kong.migrate", Time: time.Now()}
	if before != "" {
		e.Before = json.RawMessage(before)
	}
	if after != "" {
		e.After = json.RawMessage(after)
	}
	return e
}

func (f *revertFixture) all() []int64 {
	out := []int64{}
	for _, id := range f.ids {
		out = append(out, id)
	}
	return out
}

func TestRevert(t *testing.T) {
	t.Parallel()
	f := newRevertFixture(t)
	ctx := context.Background()
	at := func(n int) map[string]any { return map[string]any{"name": "x", "update_time": float64(n)} }

	f.migrated(t, KindRoute, "routes", "created", at(100), migrate("", `{"update_time":100}`))
	f.migrated(t, KindRoute, "routes", "edited", at(200), migrate("", `{"update_time":100}`))
	f.migrated(t, KindService, "services", "replaced", at(100),
		migrate(`{"name":"original","update_time":5}`, `{"update_time":50}`), migrate(`{"name":"first migration"}`, `{"update_time":100}`))
	f.migrated(t, KindConsumer, "consumers", "secret", map[string]any{"username": "secret", "update_time": float64(100)},
		migrate(`{"username":"secret","plugins":{"key-auth":{"key":"******"}}}`, `{"update_time":100}`))
	f.migrated(t, KindRoute, "routes", "gone", nil, migrate("", `{"update_time":100}`))
	f.migrated(t, KindRoute, "routes", "recreated", at(100),
		migrate("", `{"update_time":10}`), store.AuditEvent{Action: "kong.rollback"}, migrate(`{"name":"pre-existing"}`, `{"update_time":100}`))
	f.migrated(t, KindRoute, "routes", "noaudit", at(100))
	f.migrated(t, KindService, "services", "in-use", at(100), migrate("", `{"update_time":100}`))

	plan, err := f.m.RevertPlan(ctx, f.all())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]*RevertItem{}
	for _, it := range plan.Items {
		got[it.Path] = it
	}
	want := map[string]struct {
		action  string
		changed bool
	}{
		"routes/created":    {RevertDelete, false},
		"routes/edited":     {RevertDelete, true},
		"services/replaced": {RevertRestore, false},
		"consumers/secret":  {RevertManual, false},
		"routes/gone":       {RevertMissing, false},
		"routes/recreated":  {RevertRestore, false},
		"routes/noaudit":    {RevertManual, false},
		"services/in-use":   {RevertDelete, false},
	}
	for path, w := range want {
		it := got[path]
		if it == nil || it.Action != w.action || it.Changed != w.changed {
			t.Errorf("%s: got %+v, want action %s changed %v", path, it, w.action, w.changed)
		}
	}
	// Routes are reverted before services.
	lastRoute, firstService := -1, len(plan.Items)
	for i, it := range plan.Items {
		if it.Kind == KindRoute {
			lastRoute = i
		}
		if it.Kind == KindService && i < firstService {
			firstService = i
		}
	}
	if lastRoute > firstService {
		t.Error("routes must be reverted before services")
	}
	if n := plan.Affected(false); n != 4 { // created, replaced, recreated, in-use
		t.Fatalf("affected = %d, want 4", n)
	}

	// Server-side confirmation and stale protection.
	if _, err := f.m.Revert(ctx, f.all(), plan.Hash, false, 3, Actor{Name: "op"}); err == nil {
		t.Fatal("a wrong confirmation count must be rejected")
	}
	if _, err := f.m.Revert(ctx, f.all(), "stale", false, 4, Actor{Name: "op"}); !errors.Is(err, ErrStale) {
		t.Fatalf("stale hash: got %v", err)
	}

	results, err := f.m.Revert(ctx, f.all(), plan.Hash, false, 4, Actor{Name: "op"})
	if err != nil {
		t.Fatal(err)
	}
	outcome := map[string]string{}
	for _, r := range results {
		outcome[r.Path] = r.Outcome
	}
	for path, o := range map[string]string{
		"routes/created": "success", "routes/edited": "skipped", "services/replaced": "success", "consumers/secret": "skipped",
		"routes/gone": "success", "routes/recreated": "success", "routes/noaudit": "skipped", "services/in-use": "failure",
	} {
		if outcome[path] != o {
			t.Errorf("%s: outcome %q, want %q", path, outcome[path], o)
		}
	}
	if _, ok := f.admin.objs["routes/created"]; ok {
		t.Error("created route not deleted")
	}
	if f.admin.objs["services/replaced"]["name"] != "original" {
		t.Errorf("service not restored to its pre-migration state: %v", f.admin.objs["services/replaced"])
	}
	if f.admin.objs["routes/recreated"]["name"] != "pre-existing" {
		t.Errorf("after a rollback, the next migration's original must be restored: %v", f.admin.objs["routes/recreated"])
	}
	if _, ok := f.admin.objs["routes/edited"]; !ok {
		t.Error("an object changed after the migration must not be touched")
	}

	// Records of reverted and missing objects are gone; the others stay.
	left := map[string]bool{}
	list, _ := f.st.ListKongMigrations(ctx)
	for _, r := range list {
		left[r.Path()] = true
	}
	for path, keep := range map[string]bool{
		"routes/created": false, "routes/gone": false, "services/replaced": false,
		"routes/edited": true, "consumers/secret": true, "routes/noaudit": true, "services/in-use": true,
	} {
		if left[path] != keep {
			t.Errorf("%s: record kept = %v, want %v", path, left[path], keep)
		}
	}

	// Including changed objects reverts the edited route too.
	plan, err = f.m.RevertPlan(ctx, []int64{f.ids["routes/edited"]})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.m.Revert(ctx, []int64{f.ids["routes/edited"]}, plan.Hash, true, 1, Actor{Name: "op"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.admin.objs["routes/edited"]; ok {
		t.Error("included changed route not deleted")
	}
}

func TestRevertPlanValidation(t *testing.T) {
	t.Parallel()
	f := newRevertFixture(t)
	if _, err := f.m.RevertPlan(context.Background(), nil); err == nil {
		t.Error("empty selection must be rejected")
	}
	if _, err := f.m.RevertPlan(context.Background(), []int64{42}); err == nil {
		t.Error("unknown record must be rejected")
	}
}

func TestRevertExcludesApply(t *testing.T) {
	t.Parallel()
	f := newRevertFixture(t)
	f.m.job = &Job{Status: JobRunning}
	if _, err := f.m.Revert(context.Background(), []int64{1}, "h", false, 0, Actor{}); !errors.Is(err, ErrBusy) {
		t.Errorf("revert during an apply: got %v, want ErrBusy", err)
	}
}
