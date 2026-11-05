package kong

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/carloseduardo21/apisix-dashboard/internal/i18n"
)

// A small Kong configuration, as returned by the Admin API.
const fixture = `{
 "services": [
  {"id":"s1","name":"pagamentos","protocol":"https","host":"pagamentos-up","port":443,"path":"/v1","retries":3,
   "connect_timeout":5000,"write_timeout":60000,"read_timeout":30000,"tags":["pci"]},
  {"id":"s2","name":"legado","protocol":"http","host":"legado.interno","port":8080,"path":null,"connect_timeout":60000,"write_timeout":60000,"read_timeout":60000}
 ],
 "routes": [
  {"id":"r1","name":"pag-api","protocols":["http","https"],"methods":["GET","POST"],"hosts":["api.exemplo.com"],"paths":["/pagamentos"],
   "strip_path":true,"preserve_host":false,"regex_priority":0,"https_redirect_status_code":426,"service":{"id":"s1"}},
  {"id":"r2","name":"pag-regex","protocols":["https"],"paths":["~/pedidos/(?<id>\\d+)$"],"headers":{"x-versao":["2","3"]},
   "strip_path":false,"preserve_host":true,"https_redirect_status_code":301,"service":{"id":"s1"}},
  {"id":"r3","name":"legado","protocols":["http","https"],"paths":["/legado/"],"strip_path":true,"https_redirect_status_code":426,"service":{"id":"s2"}}
 ],
 "upstreams": [
  {"id":"u1","name":"pagamentos-up","algorithm":"consistent-hashing","hash_on":"header","hash_on_header":"x-cliente","hash_fallback":"none",
   "healthchecks":{"active":{"type":"https","http_path":"/health","timeout":1,"concurrency":10,"https_verify_certificate":true,
     "healthy":{"interval":5,"successes":2,"http_statuses":[200,302]},"unhealthy":{"interval":5,"http_failures":3,"tcp_failures":0,"timeouts":3,"http_statuses":[500,503]}},
    "passive":{"type":"http","healthy":{"successes":0},"unhealthy":{"http_failures":0,"tcp_failures":0,"timeouts":0}}}}
 ],
 "targets": {"u1":[{"id":"t1","target":"10.0.0.1:8443","weight":100},{"id":"t2","target":"pag-b.interno:8443","weight":50}]},
 "plugins": [
  {"id":"p1","name":"key-auth","enabled":true,"service":{"id":"s1"},"config":{"key_names":["x-api-key"],"hide_credentials":true,"key_in_body":false}},
  {"id":"p2","name":"rate-limiting","enabled":true,"route":{"id":"r1"},"config":{"minute":100,"hour":1000,"limit_by":"consumer","policy":"local","hide_client_headers":false}},
  {"id":"p3","name":"acl","enabled":true,"route":{"id":"r1"},"config":{"allow":["parceiros"],"hide_groups_header":true}},
  {"id":"p4","name":"request-transformer","enabled":true,"service":{"id":"s1"},"config":{"add":{"headers":["x-origem:kong"]},"remove":{"headers":["x-debug"]}}},
  {"id":"p5","name":"prometheus","enabled":true,"config":{}},
  {"id":"p6","name":"meu-plugin-custom","enabled":true,"route":{"id":"r3"},"config":{"nivel":"alto","limite":10}},
  {"id":"p7","name":"cors","enabled":false,"service":{"id":"s2"},"config":{"origins":["*"]}},
  {"id":"p8","name":"rate-limiting","enabled":true,"consumer":{"id":"c1"},"config":{"second":5,"policy":"local"}}
 ],
 "consumers": [
  {"id":"c1","username":"loja.online@exemplo","custom_id":"L-1"},
  {"id":"c2","username":"parceiro-x"}
 ],
 "keyauth_credentials": [{"id":"k1","key":"segredo-1","consumer":{"id":"c1"}}],
 "basicauth_credentials": [{"id":"b1","username":"parceiro","consumer":{"id":"c2"}}],
 "jwt_secrets": [{"id":"j1","key":"emissor","secret":"s3gr3d0","algorithm":"HS256","consumer":{"id":"c2"}}],
 "hmacauth_credentials": [],
 "acls": [{"id":"a1","group":"parceiros","consumer":{"id":"c1"}},{"id":"a2","group":"grupo com espaço","consumer":{"id":"c1"}}]
}`

func snapshot(t *testing.T) *Snapshot {
	t.Helper()
	var s Snapshot
	if err := json.Unmarshal([]byte(fixture), &s); err != nil {
		t.Fatal(err)
	}
	return &s
}

// at reads a dotted path ("plugins.key-auth.header"); numeric segments index arrays.
func at(v any, path string) any {
	for _, seg := range strings.Split(path, ".") {
		switch c := v.(type) {
		case map[string]any:
			v = c[seg]
		case []any:
			i := 0
			for _, ch := range seg {
				i = i*10 + int(ch-'0')
			}
			if i >= len(c) {
				return nil
			}
			v = c[i]
		default:
			return nil
		}
	}
	return v
}

func find(items []*Item, kind, kongID string) *Item {
	for _, it := range items {
		if it.Kind == kind && it.KongID == kongID {
			return it
		}
	}
	return nil
}

func expect(t *testing.T, it *Item, path string, want any) {
	t.Helper()
	got := at(generic(it.Body), path)
	if !reflect.DeepEqual(got, generic(want)) {
		t.Errorf("%s %s: %s = %#v, want %#v", it.Kind, it.KongName, path, got, generic(want))
	}
}

// hasNote reports whether it has a note with this level and i18n code
// (the plugin name is matched too when given as "plugin/code").
func hasNote(it *Item, level, code string) bool {
	plugin, c, scoped := strings.Cut(code, "/")
	if !scoped {
		plugin, c = "", code
	}
	for _, n := range it.Notes {
		if n.Level == level && n.Code == c && (plugin == "" || n.Plugin == plugin) {
			return true
		}
	}
	return false
}

func TestServiceAndUpstream(t *testing.T) {
	set := DefaultSettings()
	set.IDPrefix = "kong-"
	items := Translate(snapshot(t), set, Selection{Services: []string{"s1"}})

	up := find(items, KindUpstream, "s1")
	if up == nil || up.ID != "kong-pagamentos" || up.Blocked {
		t.Fatalf("upstream: %+v", up)
	}
	expect(t, up, "type", "chash")
	expect(t, up, "hash_on", "header")
	expect(t, up, "key", "x-cliente")
	expect(t, up, "scheme", "https")
	expect(t, up, "retries", 3)
	expect(t, up, "timeout", map[string]any{"connect": 5, "send": 60, "read": 30})
	expect(t, up, "nodes.1", map[string]any{"host": "pag-b.interno", "port": 8443, "weight": 50})
	expect(t, up, "checks.active.http_path", "/health")
	expect(t, up, "checks.active.unhealthy", map[string]any{"interval": 5, "http_failures": 3, "timeouts": 3, "http_statuses": []any{500, 503}})
	if at(generic(up.Body), "checks.passive") != nil {
		t.Error("disabled passive check must not be migrated")
	}
	// One of the two routes preserves the host: a tie keeps Kong's default.
	expect(t, up, "pass_host", "node")
	if !hasNote(up, LevelWarn, "kong.note.preserve_host_mixed") {
		t.Error("diverging preserve_host must be reported")
	}
	expect(t, up, "labels.kong-id", "s1")

	svc := find(items, KindService, "s1")
	expect(t, svc, "upstream_id", "kong-pagamentos")
	expect(t, svc, "plugins.key-auth", map[string]any{"header": "x-api-key", "query": "x-api-key", "hide_credentials": true})
	if at(generic(svc.Body), "plugins.proxy-rewrite") != nil {
		t.Error("service proxy-rewrite must move to the routes")
	}

	// Selecting a service brings its routes.
	r1 := find(items, KindRoute, "r1")
	if r1 == nil || find(items, KindRoute, "r2") == nil || find(items, KindRoute, "r3") != nil {
		t.Fatal("service selection must include exactly its routes")
	}
	// Items are ordered upstream → service → route.
	if items[0].Kind != KindUpstream || items[1].Kind != KindService {
		t.Errorf("unexpected order: %s, %s", items[0].Kind, items[1].Kind)
	}
}

func TestRouteTranslation(t *testing.T) {
	items := Translate(snapshot(t), DefaultSettings(), Selection{Routes: []string{"r1", "r2"}})
	r1 := find(items, KindRoute, "r1")
	expect(t, r1, "uris", []string{"/pagamentos*"})
	expect(t, r1, "hosts", []string{"api.exemplo.com"})
	expect(t, r1, "methods", []string{"GET", "POST"})
	expect(t, r1, "service_id", "pagamentos")
	expect(t, r1, "status", 0)
	expect(t, r1, "enable_websocket", true)
	// strip_path + service path /v1, plus the service's request-transformer.
	expect(t, r1, "plugins.proxy-rewrite", map[string]any{
		"regex_uri": []string{"^/pagamentos/?$", "/v1", "^/pagamentos/(.*)", "/v1/$1", "^/pagamentos(.*)", "/v1/$1"},
		"headers":   map[string]any{"set": map[string]any{"x-origem": "kong"}, "remove": []string{"x-debug"}},
	})
	expect(t, r1, "plugins.limit-count.rules", []any{
		map[string]any{"count": 100, "time_window": 60, "key": "$consumer_name:minute", "header_prefix": "minute"},
		map[string]any{"count": 1000, "time_window": 3600, "key": "$consumer_name:hour", "header_prefix": "hour"},
	})
	expect(t, r1, "plugins.limit-count.rejected_code", 429)
	expect(t, r1, "plugins.acl.allow_labels.groups", []string{"parceiros"})

	// Regex path, header match, https only with redirect.
	r2 := find(items, KindRoute, "r2")
	expect(t, r2, "uris", []string{"/*"})
	expect(t, r2, "vars", []any{
		[]any{"http_x_versao", "in", []any{"2", "3"}},
		[]any{"uri", "~~", `^(?:/pedidos/(?<id>\d+)$)`},
	})
	expect(t, r2, "plugins.redirect.http_to_https", true)
	// No strip_path: only the service path is prepended.
	expect(t, r2, "plugins.proxy-rewrite.regex_uri", []string{"^/(.*)", "/v1/$1"})
}

func TestRootServicePathAndCustomRule(t *testing.T) {
	set := DefaultSettings()
	set.RouteStatus = "enabled"
	set.HostRewrites = []HostRewrite{{From: "legado.interno", To: "legado_app:9000"}}
	set.PluginRules = []PluginRule{{KongPlugin: "meu-plugin-custom", Action: RuleMap, APISIXPlugin: "meu-plugin-apisix",
		Template: map[string]any{"level": "${config.nivel}", "max": "${config.limite}", "timeout": "${config.timeout|30}",
			"route": "rota ${apisix.route_id}", "absent": "${config.nao_existe}"}}}
	items := Translate(snapshot(t), set, Selection{Services: []string{"s2"}})

	up := find(items, KindUpstream, "s2")
	expect(t, up, "nodes", []any{map[string]any{"host": "legado_app", "port": 9000, "weight": 1}})
	expect(t, up, "pass_host", "rewrite")
	expect(t, up, "upstream_host", "legado.interno")

	r3 := find(items, KindRoute, "r3")
	expect(t, r3, "status", 1)
	expect(t, r3, "uris", []string{"/legado/*"})
	expect(t, r3, "plugins.proxy-rewrite.regex_uri", []string{"^/legado/?$", "/", "^/legado/(.*)", "/$1", "^/legado(.*)", "/$1"})
	expect(t, r3, "plugins.meu-plugin-apisix", map[string]any{"level": "alto", "max": 10, "timeout": 30, "route": "rota legado"})

	// The disabled cors plugin is skipped by default.
	svc := find(items, KindService, "s2")
	if at(generic(svc.Body), "plugins") != nil || !hasNote(svc, LevelInfo, "kong.note.plugin_disabled") {
		t.Errorf("disabled plugin: %+v", svc)
	}
}

func TestUnsupportedPluginPolicy(t *testing.T) {
	items := Translate(snapshot(t), DefaultSettings(), Selection{Routes: []string{"r3"}})
	r3 := find(items, KindRoute, "r3")
	if !r3.Blocked || !hasNote(r3, LevelError, "meu-plugin-custom/kong.note.plugin_blocks") {
		t.Fatalf("unknown plugin must block by default: %+v", r3.Notes)
	}

	set := DefaultSettings()
	set.UnsupportedPlugins = "skip"
	r3 = find(Translate(snapshot(t), set, Selection{Routes: []string{"r3"}}), KindRoute, "r3")
	if r3.Blocked || !hasNote(r3, LevelWarn, "kong.note.plugin_skipped") {
		t.Fatalf("skip policy: %+v", r3.Notes)
	}

	set.PluginRules = []PluginRule{{KongPlugin: "meu-plugin-custom", Action: RuleIgnore}}
	r3 = find(Translate(snapshot(t), set, Selection{Routes: []string{"r3"}}), KindRoute, "r3")
	if r3.Blocked || r3.Plugins[0].Status != "ignored" {
		t.Fatalf("ignore rule: %+v", r3.Plugins)
	}
}

func TestConsumers(t *testing.T) {
	items := Translate(snapshot(t), DefaultSettings(), Selection{Consumers: []string{"c1", "c2"}})
	c1 := find(items, KindConsumer, "c1")
	if c1.ID != "loja_online_exemplo" || c1.Blocked {
		t.Fatalf("consumer: %+v", c1)
	}
	expect(t, c1, "username", "loja_online_exemplo")
	expect(t, c1, "labels.groups", "parceiros,grupo_com_espaço")
	expect(t, c1, "plugins.limit-count.count", 5)
	expect(t, c1, "plugins.limit-count.time_window", 1)

	k1 := find(items, KindCredential, "k1")
	if k1.ID != "loja_online_exemplo/credentials/k1" || k1.Resource != "consumers" {
		t.Fatalf("credential path: %s", k1.Path())
	}
	expect(t, k1, "plugins.key-auth.key", "segredo-1")

	if b1 := find(items, KindCredential, "b1"); !b1.Blocked {
		t.Error("basic-auth credentials cannot be migrated")
	}
	j1 := find(items, KindCredential, "j1")
	expect(t, j1, "plugins.jwt-auth", map[string]any{"key": "emissor", "secret": "s3gr3d0", "algorithm": "HS256"})

	// Consumers come before credentials.
	if items[0].Kind != KindConsumer || items[len(items)-1].Kind != KindCredential {
		t.Error("consumers must be written before their credentials")
	}
}

func TestIDCollision(t *testing.T) {
	s := snapshot(t)
	s.Routes = append(s.Routes, Route{ID: "r9", Name: "pag api", Paths: []string{"/x"}, Protocols: []string{"http"}, Service: &Ref{ID: "s1"}})
	items := Translate(s, DefaultSettings(), Selection{Routes: []string{"r9"}})
	if r := find(items, KindRoute, "r9"); !r.Blocked || !hasNote(r, LevelError, "kong.note.id_collision") {
		t.Fatalf("expected collision: %+v", r.Notes)
	}
	set := DefaultSettings()
	set.IDStrategy = "kong_id"
	if r := find(Translate(s, set, Selection{Routes: []string{"r9"}}), KindRoute, "r9"); r.Blocked || r.ID != "r9" {
		t.Fatalf("kong_id strategy: %+v", r)
	}
}

func TestGlobalPlugin(t *testing.T) {
	items := Translate(snapshot(t), DefaultSettings(), Selection{Globals: []string{"p5"}})
	if len(items) != 1 || items[0].ID != "global-prometheus" || items[0].Resource != "global_rules" {
		t.Fatalf("global: %+v", items)
	}
	expect(t, items[0], "plugins.prometheus.prefer_name", true)
}

func TestMakeID(t *testing.T) {
	if got := makeID("api v2/pagamentos~x"); got != "api-v2-pagamentos-x" {
		t.Errorf("makeID = %q", got)
	}
	long := makeID(strings.Repeat("a", 100))
	if len(long) != 64 || long == makeID(strings.Repeat("a", 101)) {
		t.Errorf("long ids must be shortened and stay unique: %q", long)
	}
}

func TestCaptureGroups(t *testing.T) {
	for re, want := range map[string]int{
		`^/a/(\d+)`:           1,
		`^(?:/a)/(?<id>\d+)`:  1,
		`^/(a|b)/([0-9]+)\(x`: 2,
		`^/[(]x(?=y)`:         0,
	} {
		if got := captureGroups(re); got != want {
			t.Errorf("captureGroups(%q) = %d, want %d", re, got, want)
		}
	}
}

func TestTemplate(t *testing.T) {
	ctx := map[string]any{"config": map[string]any{"a": 1.0, "list": []any{"x", "y"}, "obj": map[string]any{"k": true}}}
	got := render(map[string]any{
		"num": "${config.a}", "second": "${config.list.1}", "nested": map[string]any{"k": "${config.obj.k}"},
		"text": "valor=${config.a}", "def": "${config.b|\"padrão\"}", "gone": "${config.b}", "arr": []any{"${config.b}", "fixo"},
		"all": "${config.obj}",
	}, ctx)
	want := map[string]any{"num": 1.0, "second": "y", "nested": map[string]any{"k": true}, "text": "valor=1",
		"def": "padrão", "arr": []any{"fixo"}, "all": map[string]any{"k": true}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("render = %#v", got)
	}
}

func TestSettingsValidate(t *testing.T) {
	s := DefaultSettings()
	if err := s.Validate(); err != nil {
		t.Fatalf("defaults must be valid: %v", err)
	}
	s.IDPrefix = "com espaço"
	s.Labels = map[string]string{"kong-id": "x", "time": "a b"}
	s.HostRewrites = []HostRewrite{{From: "a", To: "b:99999"}}
	s.PluginRules = []PluginRule{{KongPlugin: "x", Action: RuleMap}}
	err := s.Validate()
	if err == nil {
		t.Fatal("expected errors")
	}
	msgs, ok := i18n.Messages(err)
	if !ok {
		t.Fatalf("validation errors must carry user messages: %v", err)
	}
	codes := map[string]bool{}
	for _, m := range msgs {
		codes[m.Code] = true
	}
	for _, want := range []string{"kong.settings.id_prefix", "kong.settings.label_reserved", "kong.settings.label_value", "kong.settings.rewrite_to", "kong.settings.rule_apisix_plugin"} {
		if !codes[want] {
			t.Errorf("missing %q in %v", want, msgs)
		}
	}
}
