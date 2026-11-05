package kong

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"net"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/carloseduardo21/apisix-dashboard/internal/i18n"
)

const (
	KindUpstream   = "upstream"
	KindService    = "service"
	KindConsumer   = "consumer"
	KindCredential = "credential"
	KindRoute      = "route"
	KindGlobal     = "global_plugin"

	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"

	// Labels set on every migrated object that supports labels.
	LabelMigratedFrom = "migrated-from"
	LabelKongID       = "kong-id"
	// LabelGroups holds the consumer's Kong ACL groups, read by the APISIX
	// acl plugin.
	LabelGroups = "groups"
)

// Note explains how an object was translated. Message is English; the UI
// translates Code/Params (and Reason, when a plugin could not be translated).
type Note struct {
	Level   string            `json:"level"`
	Message string            `json:"message"`
	Code    string            `json:"code"`
	Params  map[string]string `json:"params,omitempty"`
	Plugin  string            `json:"plugin,omitempty"`
	Reason  *i18n.Message     `json:"reason,omitempty"`
}

// PluginMap tells how one Kong plugin was translated.
type PluginMap struct {
	Kong   string   `json:"kong"`
	APISIX []string `json:"apisix,omitempty"`
	Status string   `json:"status"` // ok, warn, error, ignored, disabled
	Custom bool     `json:"custom,omitempty"`
}

// Item is one APISIX object produced from Kong, or the reason it cannot be.
type Item struct {
	Kind     string         `json:"kind"`
	KongID   string         `json:"kong_id"`
	KongName string         `json:"kong_name"`
	Detail   string         `json:"detail,omitempty"`
	Parent   string         `json:"parent,omitempty"` // Kong id of the service or consumer
	Resource string         `json:"resource"`         // APISIX resource
	ID       string         `json:"id"`               // APISIX id ("user/credentials/id" for credentials)
	Body     map[string]any `json:"body,omitempty"`
	Plugins  []PluginMap    `json:"plugins,omitempty"`
	Notes    []Note         `json:"notes"`
	Blocked  bool           `json:"blocked"`
}

func (it *Item) note(level string, m i18n.Message) {
	it.addNote(Note{Level: level, Message: m.String(), Code: m.Code, Params: m.Params})
}

// pluginNote records a note about one Kong plugin; reason, when set, says
// why it could not be translated.
func (it *Item) pluginNote(level, plugin string, m i18n.Message, reason *i18n.Message) {
	text := plugin + ": " + m.String()
	if reason != nil {
		text = plugin + ": " + reason.String() + " (" + m.String() + ")"
	}
	it.addNote(Note{Level: level, Message: text, Code: m.Code, Params: m.Params, Plugin: plugin, Reason: reason})
}

func (it *Item) addNote(n Note) {
	it.Notes = append(it.Notes, n)
	level := n.Level
	if level == LevelError {
		it.Blocked = true
	}
}

// Path is the Admin API path of the object.
func (it *Item) Path() string { return it.Resource + "/" + it.ID }

// Selection picks what to migrate, by Kong id. Routes bring their service
// (and its upstream); services bring all their routes.
type Selection struct {
	Services  []string `json:"services"`
	Routes    []string `json:"routes"`
	Consumers []string `json:"consumers"`
	Globals   []string `json:"globals"` // plugin ids
}

// All selects the whole Kong configuration (used by the inventory).
func All(s *Snapshot) Selection {
	var sel Selection
	for _, x := range s.Services {
		sel.Services = append(sel.Services, x.ID)
	}
	for _, x := range s.Routes {
		sel.Routes = append(sel.Routes, x.ID)
	}
	for _, x := range s.Consumers {
		sel.Consumers = append(sel.Consumers, x.ID)
	}
	for _, p := range s.Plugins {
		if isGlobal(&p) {
			sel.Globals = append(sel.Globals, p.ID)
		}
	}
	return sel
}

func (s Selection) Empty() bool {
	return len(s.Services)+len(s.Routes)+len(s.Consumers)+len(s.Globals) == 0
}

func isGlobal(p *Plugin) bool {
	return p.Service == nil && p.Route == nil && p.Consumer == nil && p.ConsumerGroup == nil
}

type translator struct {
	snap  *Snapshot
	set   Settings
	rules map[string]*PluginRule

	ids        map[string]string // Kong id → APISIX id (services, routes, globals)
	usernames  map[string]string // Kong consumer id → APISIX username
	collisions map[string]string // Kong id → name of the entity that already took its id

	svcItems   map[string]*Item
	svcRewrite map[string]map[string]any // proxy-rewrite from service-level plugins
	upstreamOf map[string]int            // Kong upstream id → number of services using it
}

// Translate converts the selected part of a Kong snapshot. Items come in the
// order they must be written to APISIX.
func Translate(snap *Snapshot, set Settings, sel Selection) []*Item {
	snap.index()
	t := &translator{
		snap: snap, set: set, rules: map[string]*PluginRule{},
		ids: map[string]string{}, usernames: map[string]string{}, collisions: map[string]string{},
		svcItems: map[string]*Item{}, svcRewrite: map[string]map[string]any{}, upstreamOf: map[string]int{},
	}
	for i := range set.PluginRules {
		t.rules[set.PluginRules[i].KongPlugin] = &set.PluginRules[i]
	}
	t.assignIDs()

	services, routes, consumers, globals := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, id := range sel.Services {
		services[id] = true
		for _, r := range snap.routesBySvc[id] {
			routes[r.ID] = true
		}
	}
	for _, id := range sel.Routes {
		routes[id] = true
	}
	for _, r := range snap.Routes {
		if routes[r.ID] && r.Service != nil {
			services[r.Service.ID] = true
		}
	}
	for _, id := range sel.Consumers {
		consumers[id] = true
	}
	for _, id := range sel.Globals {
		globals[id] = true
	}

	var ups, svcs, cons, creds, rts, globs []*Item
	for i := range snap.Services {
		s := &snap.Services[i]
		if !services[s.ID] {
			continue
		}
		svc, up := t.service(s)
		svcs = append(svcs, svc)
		ups = append(ups, up)
	}
	for i := range snap.Consumers {
		c := &snap.Consumers[i]
		if !consumers[c.ID] {
			continue
		}
		ci, cr := t.consumer(c)
		cons = append(cons, ci)
		creds = append(creds, cr...)
	}
	for i := range snap.Routes {
		r := &snap.Routes[i]
		if routes[r.ID] {
			rts = append(rts, t.route(r))
		}
	}
	for i := range snap.Plugins {
		p := &snap.Plugins[i]
		if globals[p.ID] && isGlobal(p) {
			globs = append(globs, t.global(p))
		}
	}
	return slices.Concat(ups, svcs, cons, creds, rts, globs)
}

// ---- ids ----

var (
	idInvalidRe   = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
	userInvalidRe = regexp.MustCompile(`[^A-Za-z0-9_-]+`)
)

// makeID turns s into a valid APISIX id (^[a-zA-Z0-9-_.]+$, up to 64 chars).
// Long values are shortened with a hash suffix so they stay unique.
func makeID(s string) string {
	id := idInvalidRe.ReplaceAllString(s, "-")
	if len(id) > 64 {
		sum := sha256.Sum256([]byte(s))
		id = id[:55] + "-" + hex.EncodeToString(sum[:4])
	}
	return id
}

func makeUsername(s string) string {
	u := userInvalidRe.ReplaceAllString(s, "_")
	if len(u) > 256 {
		u = u[:256]
	}
	return u
}

func (t *translator) idFor(kongID, name string) string {
	base := kongID
	if t.set.IDStrategy == "name" && name != "" {
		base = name
	}
	return makeID(t.set.IDPrefix + base)
}

// assignIDs computes the APISIX id of every Kong entity up front, so
// references resolve and collisions are detected whatever the selection.
func (t *translator) assignIDs() {
	assign := func(kongID, name, id string, taken map[string]string, names map[string]string) {
		if other, ok := taken[id]; ok && other != kongID {
			t.collisions[kongID] = names[other]
			return
		}
		taken[id] = kongID
		names[kongID] = name
		t.ids[kongID] = id
	}
	svcTaken, routeTaken, globTaken, names := map[string]string{}, map[string]string{}, map[string]string{}, map[string]string{}
	for _, s := range t.snap.Services {
		assign(s.ID, s.Name, t.idFor(s.ID, s.Name), svcTaken, names)
	}
	for _, r := range t.snap.Routes {
		assign(r.ID, r.Name, t.idFor(r.ID, r.Name), routeTaken, names)
	}
	for _, p := range t.snap.Plugins {
		if isGlobal(&p) {
			assign(p.ID, p.Name, makeID(t.set.IDPrefix+"global-"+p.Name), globTaken, names)
		}
	}
	userTaken := map[string]string{}
	for _, c := range t.snap.Consumers {
		name := c.Username
		if name == "" {
			name = c.CustomID
		}
		u := makeUsername(name)
		if u == "" {
			continue
		}
		if other, ok := userTaken[u]; ok {
			t.collisions[c.ID] = names[other]
			continue
		}
		userTaken[u] = c.ID
		names[c.ID] = name
		t.usernames[c.ID] = u
	}
	for _, s := range t.snap.Services {
		if u := t.snap.upstreamByName[s.Host]; u != nil {
			t.upstreamOf[u.ID]++
		}
	}
}

func (t *translator) checkID(it *Item) bool {
	if other, ok := t.collisions[it.KongID]; ok {
		it.note(LevelError, i18n.M("kong.note.id_collision", "the generated APISIX ID collides with the one of '{other}'; rename it in Kong or use the Kong ID strategy", "other", other))
		return false
	}
	return true
}

func (t *translator) labels(kongID string) map[string]any {
	l := map[string]any{LabelMigratedFrom: "kong", LabelKongID: kongID}
	for k, v := range t.set.Labels {
		l[k] = v
	}
	return l
}

func desc(kind string, tags []string, extra ...string) string {
	d := "Migrado do Kong (" + kind + ")"
	parts := slices.Clone(extra)
	if len(tags) > 0 {
		parts = append(parts, "tags: "+strings.Join(tags, ", "))
	}
	if len(parts) > 0 {
		d += ". " + strings.Join(parts, "; ")
	}
	if len(d) > 256 {
		d = d[:253] + "..."
	}
	return d
}

// ---- services and upstreams ----

func (t *translator) rewriteHost(host string, port int) (string, int, bool) {
	for _, h := range t.set.HostRewrites {
		if strings.EqualFold(h.From, host) {
			nh, np, _ := splitHostPort(h.To)
			if np == 0 {
				np = port
			}
			return nh, np, true
		}
	}
	return host, port, false
}

var schemes = map[string]string{"http": "http", "https": "https", "grpc": "grpc", "grpcs": "grpcs", "ws": "http", "wss": "https"}

func seconds(ms int) float64 { return float64(ms) / 1000 }

func (t *translator) service(s *Service) (*Item, *Item) {
	id := t.ids[s.ID]
	svc := &Item{Kind: KindService, KongID: s.ID, KongName: nameOr(s.Name, s.ID), Resource: "services", ID: id, Notes: []Note{},
		Detail: fmt.Sprintf("%s://%s:%d%s", s.Protocol, s.Host, s.Port, s.Path)}
	up := &Item{Kind: KindUpstream, KongID: s.ID, KongName: nameOr(s.Name, s.ID), Parent: s.ID, Resource: "upstreams", ID: id, Notes: []Note{}}
	t.svcItems[s.ID] = svc
	t.checkID(svc)

	scheme, ok := schemes[s.Protocol]
	if !ok {
		svc.note(LevelError, i18n.M("kong.note.service_stream_protocol", "protocol '{protocol}' (TCP/UDP/TLS) uses stream_routes in APISIX; not supported by the migration", "protocol", s.Protocol))
	}
	if s.Enabled != nil && !*s.Enabled {
		svc.note(LevelWarn, i18n.M("kong.note.service_disabled", "service disabled in Kong: its routes will be created disabled"))
	}

	body := map[string]any{"scheme": scheme, "labels": t.labels(s.ID)}
	if s.ConnectTimeout > 0 || s.WriteTimeout > 0 || s.ReadTimeout > 0 {
		body["timeout"] = map[string]any{"connect": seconds(s.ConnectTimeout), "send": seconds(s.WriteTimeout), "read": seconds(s.ReadTimeout)}
	}
	if s.Retries != nil {
		body["retries"] = *s.Retries
	}
	if s.TLSVerify != nil {
		body["tls"] = map[string]any{"verify": *s.TLSVerify}
	}
	if s.ClientCertificate != nil {
		up.note(LevelWarn, i18n.M("kong.note.service_client_cert", "the service uses a client certificate (mTLS) in Kong: set tls.client_cert_id on the upstream"))
	}

	// preserve_host is per route in Kong and pass_host per upstream in APISIX.
	routes := t.snap.routesBySvc[s.ID]
	preserve := 0
	for _, r := range routes {
		if r.PreserveHost {
			preserve++
		}
	}
	// Strict majority: on a tie, keep Kong's default (preserve_host=false).
	passHost := preserve*2 > len(routes)
	if preserve > 0 && preserve < len(routes) {
		up.note(LevelWarn, i18n.M("kong.note.preserve_host_mixed", "{count} of {total} routes use preserve_host; in APISIX this is set on the upstream (pass_host={pass_host} for all)", "count", preserve, "total", len(routes), "pass_host", map[bool]string{true: "pass", false: "node"}[passHost]))
	}

	if ku := t.snap.upstreamByName[s.Host]; ku != nil {
		t.upstreamFromKong(ku, body, up)
		up.Detail = fmt.Sprintf("upstream %s · %d targets", ku.Name, len(t.snap.Targets[ku.ID]))
		if passHost {
			body["pass_host"] = "pass"
		} else if ku.HostHeader != "" {
			body["pass_host"] = "rewrite"
			body["upstream_host"] = ku.HostHeader
		} else {
			body["pass_host"] = "node"
		}
	} else {
		host, port, rewritten := t.rewriteHost(s.Host, s.Port)
		body["type"] = "roundrobin"
		body["nodes"] = []any{map[string]any{"host": host, "port": port, "weight": 1}}
		up.Detail = fmt.Sprintf("%s:%d", host, port)
		switch {
		case passHost:
			body["pass_host"] = "pass"
		case rewritten:
			// Kong sent the original host; keep it.
			body["pass_host"] = "rewrite"
			body["upstream_host"] = s.Host
			up.note(LevelInfo, i18n.M("kong.note.host_rewritten", "host rewritten from {from} to {to}; the Host header stays {from_header}", "from", s.Host, "to", up.Detail, "from_header", s.Host))
		default:
			body["pass_host"] = "node"
		}
	}
	if s.Name != "" {
		body["name"] = s.Name
	}
	body["desc"] = desc("service", s.Tags)
	up.Body = body

	svcBody := map[string]any{"upstream_id": id, "labels": t.labels(s.ID), "desc": desc("service", s.Tags)}
	if s.Name != "" {
		svcBody["name"] = s.Name
	}
	if s.Protocol == "ws" || s.Protocol == "wss" || t.set.EnableWebsocket {
		svcBody["enable_websocket"] = true
	}
	plugins := t.plugins(svc, pluginScope{level: "service", service: s}, func(p *Plugin) bool {
		return p.Service != nil && p.Service.ID == s.ID && p.Route == nil
	})
	// APISIX does not merge proxy-rewrite between service and route, and routes
	// always need their own (strip_path). Apply the service's on every route.
	if pr, ok := plugins["proxy-rewrite"]; ok {
		t.svcRewrite[s.ID] = pr.(map[string]any)
		delete(plugins, "proxy-rewrite")
		svc.note(LevelInfo, i18n.M("kong.note.service_rewrite_per_route", "the service request rewrite (proxy-rewrite) is applied to each route"))
	}
	if len(plugins) > 0 {
		svcBody["plugins"] = plugins
	}
	svc.Body = svcBody

	if up.Blocked {
		svc.note(LevelError, i18n.M("kong.note.upstream_blocked", "the upstream cannot be migrated"))
	}
	if svc.Blocked && !up.Blocked {
		up.note(LevelError, i18n.M("kong.note.service_blocked", "the service cannot be migrated"))
	}
	return svc, up
}

var algorithms = map[string]string{"": "roundrobin", "round-robin": "roundrobin", "least-connections": "least_conn", "latency": "ewma"}

func (t *translator) upstreamFromKong(ku *Upstream, body map[string]any, it *Item) {
	if n := t.upstreamOf[ku.ID]; n > 1 {
		it.note(LevelInfo, i18n.M("kong.note.upstream_shared", "Kong upstream {name} is used by {count} services; each gets its own APISIX upstream (duplicated health checks)", "name", ku.Name, "count", n))
	}
	if ku.ClientCertificate != nil {
		it.note(LevelWarn, i18n.M("kong.note.upstream_client_cert", "the upstream uses a client certificate (mTLS) in Kong: set tls.client_cert_id"))
	}
	if typ, ok := algorithms[ku.Algorithm]; ok {
		body["type"] = typ
	} else if ku.Algorithm == "consistent-hashing" {
		body["type"] = "chash"
		switch ku.HashOn {
		case "consumer":
			body["hash_on"] = "consumer"
		case "ip":
			body["hash_on"], body["key"] = "vars", "remote_addr"
		case "header":
			body["hash_on"], body["key"] = "header", ku.HashOnHeader
		case "cookie":
			body["hash_on"], body["key"] = "cookie", ku.HashOnCookie
			it.note(LevelWarn, i18n.M("kong.note.hash_cookie", "cookie hashing: Kong creates cookie '{cookie}' when missing, APISIX does not", "cookie", ku.HashOnCookie))
		case "path":
			body["hash_on"], body["key"] = "vars", "uri"
		case "query_arg":
			body["hash_on"], body["key"] = "vars", "arg_"+ku.HashOnQueryArg
		default:
			body["type"] = "roundrobin"
			if ku.HashOn != "none" {
				it.note(LevelWarn, i18n.M("kong.note.hash_on_unsupported", "hash_on={value} has no equivalent; using roundrobin", "value", ku.HashOn))
			}
		}
		if ku.HashFallback != "" && ku.HashFallback != "none" {
			it.note(LevelWarn, i18n.M("kong.note.hash_fallback_unsupported", "hash_fallback={value} has no equivalent in APISIX", "value", ku.HashFallback))
		}
	} else {
		body["type"] = "roundrobin"
		it.note(LevelWarn, i18n.M("kong.note.algorithm_unknown", "unknown algorithm '{algorithm}'; using roundrobin", "algorithm", ku.Algorithm))
	}

	nodes := []any{}
	rewrites := 0
	for _, tg := range t.snap.Targets[ku.ID] {
		host, port, err := parseTarget(tg.Target)
		if err != nil {
			it.note(LevelError, i18n.M("kong.note.target_invalid", "invalid target '{target}'", "target", tg.Target))
			continue
		}
		var rw bool
		if host, port, rw = t.rewriteHost(host, port); rw {
			rewrites++
		}
		nodes = append(nodes, map[string]any{"host": host, "port": port, "weight": tg.Weight})
	}
	if len(nodes) == 0 {
		it.note(LevelWarn, i18n.M("kong.note.upstream_no_targets", "upstream {name} has no targets in Kong", "name", ku.Name))
	}
	if rewrites > 0 {
		it.note(LevelInfo, i18n.M("kong.note.targets_rewritten", "{count} targets with a rewritten host", "count", rewrites))
	}
	body["nodes"] = nodes
	if c := checks(ku.Healthchecks, it); c != nil {
		body["checks"] = c
	}
}

// parseTarget reads "host:port", "[ipv6]:port" or "host" (Kong defaults to 8000).
func parseTarget(s string) (string, int, error) {
	if h, p, err := net.SplitHostPort(s); err == nil {
		port, err := strconv.Atoi(p)
		if err != nil {
			return "", 0, err
		}
		return h, port, nil
	}
	if s == "" {
		return "", 0, fmt.Errorf("empty target")
	}
	return strings.Trim(s, "[]"), 8000, nil
}

func checks(hc map[string]any, it *Item) map[string]any {
	active, passive := obj(hc, "active"), obj(hc, "passive")
	ah, au := obj(active, "healthy"), obj(active, "unhealthy")
	activeOn := numOr(ah, "interval", 0) > 0 || numOr(au, "interval", 0) > 0
	ph, pu := obj(passive, "healthy"), obj(passive, "unhealthy")
	passiveOn := numOr(pu, "http_failures", 0) > 0 || numOr(pu, "tcp_failures", 0) > 0 || numOr(pu, "timeouts", 0) > 0
	if !activeOn && !passiveOn {
		return nil
	}
	if !activeOn {
		it.note(LevelWarn, i18n.M("kong.note.passive_check_dropped", "APISIX requires an active health check alongside the passive one; the passive health check was not migrated"))
		return nil
	}
	typ := str(active, "type")
	if typ != "http" && typ != "https" && typ != "tcp" {
		it.note(LevelWarn, i18n.M("kong.note.active_check_unsupported", "active health check of type '{type}' is not supported; not migrated", "type", typ))
		return nil
	}
	counters := func(dst, src map[string]any, keys ...string) {
		for _, k := range keys {
			if n := numOr(src, k, 0); n > 0 {
				dst[k] = int(math.Min(n, 254))
			}
		}
	}
	interval := func(src map[string]any) int {
		return int(math.Max(1, math.Ceil(numOr(src, "interval", 0))))
	}
	a := map[string]any{"type": typ}
	for _, k := range []string{"timeout", "concurrency"} {
		if n, ok := num(active, k); ok && n > 0 {
			a[k] = n
		}
	}
	if p := str(active, "http_path"); p != "" {
		a["http_path"] = p
	}
	if typ == "https" {
		if v, ok := boolean(active, "https_verify_certificate"); ok {
			a["https_verify_certificate"] = v
		}
		if sni := str(active, "https_sni"); sni != "" {
			a["host"] = sni
		}
	}
	if hdrs := obj(active, "headers"); len(hdrs) > 0 {
		var list []any
		for k, vals := range hdrs {
			for _, v := range anySlice(vals) {
				list = append(list, fmt.Sprintf("%s: %v", k, v))
			}
		}
		sort.Slice(list, func(i, j int) bool { return list[i].(string) < list[j].(string) })
		a["req_headers"] = list
	}
	healthy := map[string]any{"interval": interval(ah)}
	if s := anySlice(ah["http_statuses"]); len(s) > 0 {
		healthy["http_statuses"] = s
	}
	counters(healthy, ah, "successes")
	unhealthy := map[string]any{"interval": interval(au)}
	if s := anySlice(au["http_statuses"]); len(s) > 0 {
		unhealthy["http_statuses"] = s
	}
	counters(unhealthy, au, "http_failures", "tcp_failures", "timeouts")
	a["healthy"], a["unhealthy"] = healthy, unhealthy
	out := map[string]any{"active": a}

	if passiveOn {
		pt := str(passive, "type")
		if pt != "http" && pt != "https" && pt != "tcp" {
			pt = "http"
		}
		p := map[string]any{"type": pt}
		h := map[string]any{}
		if s := anySlice(ph["http_statuses"]); len(s) > 0 {
			h["http_statuses"] = s
		}
		counters(h, ph, "successes")
		u := map[string]any{}
		if s := anySlice(pu["http_statuses"]); len(s) > 0 {
			u["http_statuses"] = s
		}
		counters(u, pu, "http_failures", "tcp_failures", "timeouts")
		if len(h) > 0 {
			p["healthy"] = h
		}
		if len(u) > 0 {
			p["unhealthy"] = u
		}
		out["passive"] = p
	}
	return out
}

// ---- routes ----

func (t *translator) route(r *Route) *Item {
	it := &Item{Kind: KindRoute, KongID: r.ID, KongName: nameOr(r.Name, r.ID), Resource: "routes", ID: t.ids[r.ID], Notes: []Note{}, Detail: routeDetail(r)}
	if r.Service != nil {
		it.Parent = r.Service.ID
	}
	t.checkID(it)
	body := map[string]any{"labels": t.labels(r.ID), "desc": desc("route", r.Tags)}
	if r.Name != "" {
		body["name"] = r.Name
	}

	var svc *Service
	if r.Service != nil {
		svc = t.snap.services[r.Service.ID]
		if svc == nil {
			it.note(LevelError, i18n.M("kong.note.route_service_missing", "the route's service {id} does not exist in Kong", "id", r.Service.ID))
		} else {
			body["service_id"] = t.ids[svc.ID]
			if si := t.svcItems[svc.ID]; si != nil && si.Blocked {
				it.note(LevelError, i18n.M("kong.note.route_service_blocked", "service {name} cannot be migrated", "name", si.KongName))
			}
		}
	}

	if r.Expression != "" {
		it.note(LevelError, i18n.M("kong.note.expression_routes", "expression routes (router expressions) are not supported"))
	}
	if len(r.Sources) > 0 || len(r.Destinations) > 0 {
		it.note(LevelError, i18n.M("kong.note.stream_routes", "source/destination (stream) routes are not supported"))
	}

	var vars []any
	hasHTTP, hasHTTPS := false, false
	for _, p := range r.Protocols {
		switch p {
		case "http", "grpc", "ws":
			hasHTTP = true
		case "https", "grpcs", "wss":
			hasHTTPS = true
		default:
			it.note(LevelError, i18n.M("kong.note.route_stream_protocol", "protocol '{protocol}' uses stream_routes in APISIX; not supported", "protocol", p))
		}
	}
	plugins := map[string]any{}
	switch {
	case hasHTTPS && !hasHTTP:
		code := r.HTTPSRedirectStatusCode
		if code == 301 || code == 302 || code == 307 || code == 308 {
			plugins["redirect"] = map[string]any{"http_to_https": true}
			if code != 301 {
				it.note(LevelInfo, i18n.M("kong.note.https_redirect_code", "the HTTPS redirect uses 301 (GET) or 308 in APISIX instead of {code}", "code", code))
			}
		} else {
			vars = append(vars, []any{"scheme", "==", "https"})
			it.note(LevelInfo, i18n.M("kong.note.https_only", "HTTPS-only route: HTTP requests get 404 instead of {code}", "code", code))
		}
	case hasHTTP && !hasHTTPS:
		vars = append(vars, []any{"scheme", "==", "http"})
	}

	if len(r.Methods) > 0 {
		body["methods"] = r.Methods
	}
	hostVars := t.hosts(r, body, it)
	if hostVars != nil {
		vars = append(vars, hostVars)
	}
	if len(r.SNIs) > 0 {
		it.note(LevelWarn, i18n.M("kong.note.snis_ignored", "APISIX does not route HTTP by SNI; ignored SNIs: {snis}", "snis", strings.Join(r.SNIs, ", ")))
	}
	names := make([]string, 0, len(r.Headers))
	for k := range r.Headers {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		vars = append(vars, headerExpr(k, r.Headers[k]))
	}

	svcPath := ""
	if svc != nil {
		svcPath = svc.Path
	}
	uris, pathVars, regexURI := paths(r, svcPath, it)
	body["uris"] = uris
	if pathVars != nil {
		vars = append(vars, pathVars)
	}
	if len(vars) > 0 {
		body["vars"] = vars
	}
	if r.RegexPriority != 0 {
		body["priority"] = r.RegexPriority
	}
	if r.RequestBuffering != nil && !*r.RequestBuffering {
		it.note(LevelWarn, i18n.M("kong.note.request_buffering", "request_buffering=false is not migrated (configure proxy_request_buffering in APISIX)"))
	}
	if r.ResponseBuffering != nil && !*r.ResponseBuffering {
		it.note(LevelInfo, i18n.M("kong.note.response_buffering", "response_buffering=false: use the proxy-buffering plugin if needed"))
	}

	status := 1
	if t.set.RouteStatus == "disabled" || (svc != nil && svc.Enabled != nil && !*svc.Enabled) {
		status = 0
	}
	body["status"] = status
	if t.set.EnableWebsocket {
		body["enable_websocket"] = true
	}

	for k, v := range t.plugins(it, pluginScope{level: "route", service: svc, route: r}, func(p *Plugin) bool {
		return p.Route != nil && p.Route.ID == r.ID
	}) {
		plugins[k] = v
	}
	// Request rewrite: path (strip_path / service path) + the Kong
	// request-transformer of the route, or else of the service.
	rewrite := map[string]any{}
	if len(regexURI) > 0 {
		rewrite["regex_uri"] = regexURI
	}
	if pr, ok := plugins["proxy-rewrite"].(map[string]any); ok {
		mergeRewrite(rewrite, pr)
	} else if svc != nil && t.svcRewrite[svc.ID] != nil {
		mergeRewrite(rewrite, t.svcRewrite[svc.ID])
	}
	if len(rewrite) > 0 {
		plugins["proxy-rewrite"] = rewrite
	} else {
		delete(plugins, "proxy-rewrite")
	}
	if len(plugins) > 0 {
		body["plugins"] = plugins
	}
	if svc == nil && len(plugins) == 0 {
		it.note(LevelError, i18n.M("kong.note.route_no_target", "route without a Kong service and without a plugin that answers the request"))
	}
	it.Body = body
	return it
}

func routeDetail(r *Route) string {
	var parts []string
	if len(r.Methods) > 0 {
		parts = append(parts, strings.Join(r.Methods, ","))
	}
	if len(r.Paths) > 0 {
		parts = append(parts, strings.Join(r.Paths, " "))
	}
	if len(r.Hosts) > 0 {
		parts = append(parts, "("+strings.Join(r.Hosts, ", ")+")")
	}
	return strings.Join(parts, " ")
}

// hosts sets body.hosts, or returns a vars expression when Kong suffix
// wildcards ("example.*") are used, which APISIX hosts do not support.
func (t *translator) hosts(r *Route, body map[string]any, it *Item) any {
	if len(r.Hosts) == 0 {
		return nil
	}
	suffix := false
	hosts := make([]string, 0, len(r.Hosts))
	for _, h := range r.Hosts {
		if hp, _, err := net.SplitHostPort(h); err == nil {
			it.note(LevelWarn, i18n.M("kong.note.host_port_dropped", "APISIX does not match the port in the host; '{host}' becomes '{result}'", "host", h, "result", hp))
			h = hp
		}
		if strings.HasSuffix(h, ".*") {
			suffix = true
		}
		hosts = append(hosts, h)
	}
	if !suffix {
		body["hosts"] = hosts
		return nil
	}
	var exprs []any
	for _, h := range hosts {
		rx := "^" + regexp.QuoteMeta(h) + "$"
		rx = strings.Replace(rx, `^\*\.`, `^[^.]+\.`, 1)
		rx = strings.Replace(rx, `\.\*$`, `\.[^.]+(\.[^.]+)*$`, 1)
		exprs = append(exprs, []any{"host", "~*", rx})
	}
	return or(exprs)
}

func or(exprs []any) any {
	if len(exprs) == 1 {
		return exprs[0]
	}
	return append([]any{"OR"}, exprs...)
}

// headerExpr matches Kong route headers: any of the values, "~*" for regex.
func headerExpr(name string, values []string) any {
	v := "http_" + strings.ReplaceAll(strings.ToLower(name), "-", "_")
	var exprs []any
	var plain []any
	for _, x := range values {
		if rx, ok := strings.CutPrefix(x, "~*"); ok {
			exprs = append(exprs, []any{v, "~*", rx})
		} else {
			plain = append(plain, x)
		}
	}
	switch len(plain) {
	case 0:
	case 1:
		exprs = append(exprs, []any{v, "==", plain[0]})
	default:
		exprs = append(exprs, []any{v, "in", plain})
	}
	return or(exprs)
}

// paths translates Kong paths. Kong matches plain paths by prefix and "~"
// paths as regexes anchored at the start; strip_path removes the matched
// part and the service path is prepended (path_handling v0).
func paths(r *Route, svcPath string, it *Item) (uris []string, vars any, regexURI []string) {
	type kp struct {
		re    string // anchored regex matching the path prefix
		plain string
		regex bool
	}
	var list []kp
	for _, p := range r.Paths {
		if rx, ok := strings.CutPrefix(p, "~"); ok {
			rx = strings.TrimPrefix(rx, "^")
			list = append(list, kp{re: "^(?:" + rx + ")", regex: true})
		} else {
			list = append(list, kp{re: "^" + regexp.QuoteMeta(strings.TrimSuffix(p, "/")), plain: p})
		}
	}
	anyRegex := slices.ContainsFunc(list, func(k kp) bool { return k.regex })
	switch {
	case len(list) == 0:
		uris = []string{"/*"}
	case anyRegex:
		uris = []string{"/*"}
		var exprs []any
		for _, k := range list {
			exprs = append(exprs, []any{"uri", "~~", k.re})
		}
		vars = or(exprs)
	default:
		for _, k := range list {
			uris = append(uris, k.plain+"*")
		}
	}
	if r.PathHandling == "v1" && r.StripPath && svcPath != "" && svcPath != "/" {
		it.note(LevelInfo, i18n.M("kong.note.path_handling_v1", "path_handling v1: check how the service path is joined with the route path"))
	}

	root := svcPath == "" || svcPath == "/"
	base := strings.TrimSuffix(svcPath, "/")
	if !r.StripPath || len(list) == 0 {
		if root {
			return
		}
		return uris, vars, []string{"^/(.*)", base + "/$1"}
	}
	// Longest prefix first, so the most specific path is stripped.
	sort.SliceStable(list, func(i, j int) bool { return len(list[i].re) > len(list[j].re) })
	exact := svcPath
	if root {
		exact = "/"
	}
	for _, k := range list {
		if k.regex {
			n := captureGroups(k.re)
			regexURI = append(regexURI, k.re+"/?(.*)", base+"/$"+strconv.Itoa(n+1))
			continue
		}
		if k.re == "^" { // path "/"
			if !root {
				regexURI = append(regexURI, "^/(.*)", base+"/$1")
			}
			continue
		}
		regexURI = append(regexURI,
			k.re+"/?$", exact,
			k.re+"/(.*)", base+"/$1",
			k.re+"(.*)", base+"/$1",
		)
	}
	if anyRegex {
		it.note(LevelInfo, i18n.M("kong.note.regex_strip_path", "regex path with strip_path: check the generated rewrite (regex_uri)"))
	}
	return
}

// captureGroups counts the capturing groups of a regex.
func captureGroups(re string) int {
	n := 0
	inClass := false
	for i := 0; i < len(re); i++ {
		switch re[i] {
		case '\\':
			i++
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '(':
			if inClass {
				continue
			}
			if i+1 < len(re) && re[i+1] == '?' {
				// (?<name>...) and (?P<name>...) capture; (?:...), lookarounds do not.
				if i+2 < len(re) && (re[i+2] == 'P' || (re[i+2] == '<' && i+3 < len(re) && re[i+3] != '=' && re[i+3] != '!')) {
					n++
				}
				continue
			}
			n++
		}
	}
	return n
}

func mergeRewrite(dst, src map[string]any) {
	for k, v := range src {
		if k != "headers" {
			dst[k] = v
			continue
		}
		dh, _ := dst["headers"].(map[string]any)
		if dh == nil {
			dh = map[string]any{}
		}
		sh, _ := v.(map[string]any)
		for op, val := range sh {
			switch op {
			case "remove":
				prev, _ := dh[op].([]any)
				dh[op] = append(prev, anySlice(val)...)
			default:
				m, _ := dh[op].(map[string]any)
				if m == nil {
					m = map[string]any{}
				}
				for hk, hv := range val.(map[string]any) {
					m[hk] = hv
				}
				dh[op] = m
			}
		}
		dst["headers"] = dh
	}
}

// ---- consumers ----

func (t *translator) consumer(c *Consumer) (*Item, []*Item) {
	name := nameOr(c.Username, c.CustomID)
	username := t.usernames[c.ID]
	it := &Item{Kind: KindConsumer, KongID: c.ID, KongName: nameOr(name, c.ID), Resource: "consumers", ID: username, Notes: []Note{}}
	if c.CustomID != "" {
		it.Detail = "custom_id " + c.CustomID
	}
	t.checkID(it)
	if username == "" && !it.Blocked {
		it.note(LevelError, i18n.M("kong.note.consumer_no_name", "consumer without username or custom_id"))
	}
	if username != "" && username != name {
		it.note(LevelInfo, i18n.M("kong.note.username_adjusted", "username adjusted to the APISIX format: '{from}' → '{to}'", "from", name, "to", username))
	}

	var extra []string
	if c.CustomID != "" {
		extra = append(extra, "custom_id: "+c.CustomID)
	}
	labels := t.labels(c.ID)
	if groups := t.snap.groups[c.ID]; len(groups) > 0 {
		labels[LabelGroups] = strings.Join(groupLabels(groups), ",")
	}
	body := map[string]any{"username": username, "labels": labels, "desc": desc("consumer", c.Tags, extra...)}
	plugins := t.plugins(it, pluginScope{level: "consumer", consumer: c}, func(p *Plugin) bool {
		return p.Consumer != nil && p.Consumer.ID == c.ID && p.Route == nil && p.Service == nil
	})
	if len(plugins) > 0 {
		body["plugins"] = plugins
	}
	it.Body = body

	var creds []*Item
	cred := func(kongID, plugin, detail string, conf map[string]any) *Item {
		ci := &Item{Kind: KindCredential, KongID: kongID, KongName: nameOr(name, c.ID) + " · " + plugin, Detail: detail, Parent: c.ID,
			Resource: "consumers", ID: username + "/credentials/" + makeID(kongID), Notes: []Note{}}
		if conf != nil {
			ci.Body = map[string]any{"plugins": map[string]any{plugin: conf}, "labels": t.labels(kongID), "desc": desc(plugin, nil)}
		}
		if it.Blocked {
			ci.note(LevelError, i18n.M("kong.note.consumer_blocked", "the consumer cannot be migrated"))
		}
		creds = append(creds, ci)
		return ci
	}
	for _, k := range t.snap.KeyAuths {
		if k.Consumer.ID == c.ID {
			ci := cred(k.ID, "key-auth", "", map[string]any{"key": k.Key})
			if k.TTL > 0 {
				ci.note(LevelWarn, i18n.M("kong.note.key_ttl", "the key expires in Kong (ttl {ttl}s); in APISIX it does not expire", "ttl", k.TTL))
			}
		}
	}
	for _, b := range t.snap.BasicAuths {
		if b.Consumer.ID == c.ID {
			ci := cred(b.ID, "basic-auth", b.Username, nil)
			ci.note(LevelError, i18n.M("kong.note.basic_auth_password", "Kong stores only the password hash; set a new password for '{username}' in APISIX", "username", b.Username))
		}
	}
	for _, j := range t.snap.JWTs {
		if j.Consumer.ID != c.ID {
			continue
		}
		alg := nameOr(j.Algorithm, "HS256")
		conf := map[string]any{"key": j.Key, "algorithm": alg}
		if strings.HasPrefix(alg, "HS") {
			conf["secret"] = j.Secret
		} else {
			conf["public_key"] = j.RSAPublicKey
		}
		ci := cred(j.ID, "jwt-auth", j.Key+" · "+alg, conf)
		if !jwtAlgorithms[alg] {
			ci.note(LevelError, i18n.M("kong.note.jwt_algorithm", "algorithm {algorithm} is not supported by APISIX jwt-auth", "algorithm", alg))
		} else if !strings.HasPrefix(alg, "HS") && j.RSAPublicKey == "" {
			ci.note(LevelError, i18n.M("kong.note.jwt_no_public_key", "{algorithm} credential without a public key", "algorithm", alg))
		}
	}
	for _, h := range t.snap.HMACAuths {
		if h.Consumer.ID == c.ID {
			cred(h.ID, "hmac-auth", h.Username, map[string]any{"key_id": h.Username, "secret_key": h.Secret})
		}
	}
	return it, creds
}

var jwtAlgorithms = map[string]bool{"HS256": true, "HS384": true, "HS512": true, "RS256": true, "RS384": true, "RS512": true,
	"ES256": true, "ES384": true, "ES512": true, "PS256": true, "PS384": true, "PS512": true, "EdDSA": true}

var groupInvalidRe = regexp.MustCompile(`[\s,]+`)

// groupLabels makes ACL group names valid in a comma separated label value.
func groupLabels(groups []string) []string {
	out := make([]string, len(groups))
	for i, g := range groups {
		out[i] = groupInvalidRe.ReplaceAllString(g, "_")
	}
	return out
}

// ---- global plugins ----

func (t *translator) global(p *Plugin) *Item {
	it := &Item{Kind: KindGlobal, KongID: p.ID, KongName: p.Name, Detail: "global plugin", Resource: "global_rules", ID: t.ids[p.ID], Notes: []Note{}}
	t.checkID(it)
	plugins := t.plugins(it, pluginScope{level: "global"}, func(x *Plugin) bool { return x.ID == p.ID })
	if len(plugins) == 0 && !it.Blocked {
		// Ignored or disabled: nothing to write.
		it.Blocked = true
	}
	it.Body = map[string]any{"plugins": plugins}
	return it
}

func nameOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
