package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/carloseduardo21/apisix-dashboard/internal/auth"
	"github.com/carloseduardo21/apisix-dashboard/internal/kong"
)

func withRestricted(names ...string) harnessOpt {
	return func(o *Options, _ *auth.Config) {
		o.RestrictedPlugins = map[string]bool{}
		for _, n := range names {
			o.RestrictedPlugins[n] = true
		}
	}
}

func TestRestrictedPlugins(t *testing.T) {
	t.Parallel()
	const fn = `"serverless-pre-function":{"phase":"rewrite","functions":["return function() end"]}`
	current := `{"value":{"id":"1","uri":"/a","plugins":{` + fn + `},"script":"return 1"}}`
	tests := []struct {
		name    string
		role    string
		current string // body the fake Admin API returns (current object)
		method  string
		path    string
		body    string
		want    int
		code    string
	}{
		{"operator cannot add a restricted plugin", "operator", `{}`, http.MethodPut, "/api/apisix/routes/1",
			`{"uri":"/a","plugins":{` + fn + `}}`, http.StatusForbidden, "apisix.plugin_restricted"},
		{"operator cannot create one with POST", "operator", `{}`, http.MethodPost, "/api/apisix/routes",
			`{"uri":"/a","plugins":{` + fn + `}}`, http.StatusForbidden, "apisix.plugin_restricted"},
		{"operator cannot change one", "operator", current, http.MethodPut, "/api/apisix/routes/1",
			`{"uri":"/a","plugins":{"serverless-pre-function":{"phase":"access","functions":["return function() end"]}},"script":"return 1"}`,
			http.StatusForbidden, "apisix.plugin_restricted"},
		{"operator may edit other fields", "operator", current, http.MethodPut, "/api/apisix/routes/1",
			`{"uri":"/b","plugins":{` + fn + `},"script":"return 1"}`, http.StatusOK, ""},
		{"operator may remove it", "operator", current, http.MethodPut, "/api/apisix/routes/1",
			`{"uri":"/a","plugins":{}}`, http.StatusOK, ""},
		{"operator may remove it with PATCH null", "operator", current, http.MethodPatch, "/api/apisix/routes/1",
			`{"plugins":{"serverless-pre-function":null}}`, http.StatusOK, ""},
		{"operator cannot set a route script", "operator", `{}`, http.MethodPut, "/api/apisix/routes/2",
			`{"uri":"/a","script":"os.execute('id')"}`, http.StatusForbidden, "apisix.script_restricted"},
		{"operator cannot change a route script", "operator", current, http.MethodPatch, "/api/apisix/routes/1",
			`{"script":"return 2"}`, http.StatusForbidden, "apisix.script_restricted"},
		{"restricted on every resource with plugins", "operator", `{}`, http.MethodPut, "/api/apisix/global_rules/1",
			`{"plugins":{` + fn + `}}`, http.StatusForbidden, "apisix.plugin_restricted"},
		{"restricted plugin metadata", "operator", `{}`, http.MethodPut, "/api/apisix/plugin_metadata/serverless-pre-function",
			`{"x":1}`, http.StatusForbidden, "apisix.plugin_restricted"},
		{"other plugins are fine", "operator", `{}`, http.MethodPut, "/api/apisix/routes/3",
			`{"uri":"/a","plugins":{"limit-count":{"count":1,"time_window":60}}}`, http.StatusOK, ""},
		{"admin may add one", "admin", `{}`, http.MethodPut, "/api/apisix/routes/1",
			`{"uri":"/a","plugins":{` + fn + `},"script":"return 1"}`, http.StatusOK, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, withRestricted("serverless-pre-function"))
			h.admin.body = tc.current
			s := h.login("user1", tc.role)
			rec := h.do(&s, tc.method, tc.path, tc.body)
			if rec.Code != tc.want {
				t.Fatalf("status %d, want %d (%s)", rec.Code, tc.want, rec.Body.String())
			}
			if tc.code == "" {
				if len(h.admin.writes()) != 1 {
					t.Errorf("expected one write to the Admin API, got %v", h.admin.writes())
				}
				return
			}
			if got := decodeError(t, rec).Code; got != tc.code {
				t.Errorf("code %q, want %q", got, tc.code)
			}
			if w := h.admin.writes(); len(w) != 0 {
				t.Errorf("denied write reached the Admin API: %v", w)
			}
			if !strings.Contains(h.audit.String(), `"outcome":"failure"`) {
				t.Error("denied write must be audited")
			}
		})
	}
}

func TestRestrictedKongRules(t *testing.T) {
	t.Parallel()
	s := New(Options{RestrictedPlugins: map[string]bool{"serverless-pre-function": true}})
	approved := kong.PluginRule{KongPlugin: "my-fn", Action: kong.RuleMap, APISIXPlugin: "serverless-pre-function", Template: map[string]any{"phase": "rewrite"}}
	changed := approved
	changed.Template = map[string]any{"phase": "access"}
	other := kong.PluginRule{KongPlugin: "my-limit", Action: kong.RuleMap, APISIXPlugin: "limit-count"}

	tests := []struct {
		name        string
		role        string
		next, saved []kong.PluginRule
		wantErr     bool
	}{
		{"operator adds a restricted mapping", "operator", []kong.PluginRule{approved}, nil, true},
		{"operator changes an approved mapping", "operator", []kong.PluginRule{changed}, []kong.PluginRule{approved}, true},
		{"operator keeps an approved mapping", "operator", []kong.PluginRule{approved, other}, []kong.PluginRule{approved}, false},
		{"operator maps to other plugins", "operator", []kong.PluginRule{other}, nil, false},
		{"admin adds a restricted mapping", "admin", []kong.PluginRule{approved}, nil, false},
	}
	for _, tc := range tests {
		err := s.restrictedKongRules(tc.role, tc.next, tc.saved)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", tc.name, err, tc.wantErr)
		}
	}
}

// A masked secret cannot be kept while its destination changes (M2).
func TestMaskedSecretRedirectRejected(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.admin.body = `{"value":{"id":"1","plugins":{"http-logger":{"uri":"https://logs.example.com","auth_header":"Bearer real-secret"}}}}`
	s := h.login("operator1", "operator")

	rec := h.do(&s, http.MethodPut, "/api/apisix/global_rules/1",
		`{"plugins":{"http-logger":{"uri":"https://attacker.example.net","auth_header":"******"}}}`)
	if rec.Code != http.StatusBadRequest || decodeError(t, rec).Code != "apisix.masked_secret_destination_changed" {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if w := h.admin.writes(); len(w) != 0 {
		t.Errorf("write reached the Admin API: %v", w)
	}

	rec = h.do(&s, http.MethodPut, "/api/apisix/global_rules/1",
		`{"plugins":{"http-logger":{"uri":"https://logs.example.com","auth_header":"******","batch_max_size":10}}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("unchanged destination: status %d body %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "real-secret") {
		t.Errorf("secret leaked in the response: %s", rec.Body.String())
	}
}
