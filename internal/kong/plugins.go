package kong

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/carloseduardo21/apisix-dashboard/internal/i18n"
)

type pluginScope struct {
	level    string // service, route, consumer, global
	service  *Service
	route    *Route
	consumer *Consumer
}

// pctx is what a plugin translator gets.
type pctx struct {
	t      *translator
	it     *Item
	p      *Plugin
	scope  pluginScope
	warned bool
}

func (c *pctx) note(level string, m i18n.Message) {
	c.it.pluginNote(level, c.p.Name, m, nil)
	if level != LevelInfo {
		c.warned = true
	}
}

// pluginFunc returns the APISIX plugins (name → config) for a Kong plugin
// config. An error means the plugin cannot be translated.
type pluginFunc func(c *pctx, conf map[string]any) (map[string]any, error)

var builtins map[string]pluginFunc

func init() {
	builtins = map[string]pluginFunc{
		"key-auth":              keyAuth,
		"basic-auth":            basicAuth,
		"jwt":                   jwtAuth,
		"hmac-auth":             hmacAuth,
		"acl":                   acl,
		"rate-limiting":         rateLimiting,
		"cors":                  cors,
		"ip-restriction":        ipRestriction,
		"request-transformer":   requestTransformer,
		"response-transformer":  responseTransformer,
		"correlation-id":        correlationID,
		"prometheus":            prometheus,
		"file-log":              fileLog,
		"http-log":              httpLog,
		"tcp-log":               tcpLog,
		"udp-log":               udpLog,
		"proxy-cache":           proxyCache,
		"request-size-limiting": requestSizeLimiting,
		"bot-detection":         botDetection,
		"request-termination":   requestTermination,
		"openid-connect":        openIDConnect,
	}
}

// BuiltinPlugins lists the Kong plugins translated without a custom rule.
func BuiltinPlugins() []string {
	return slices.Sorted(maps.Keys(builtins))
}

var errUnsupported = i18n.E("kong.reason.no_translation", "no automatic translation")

// reasonOf is the user-facing reason a plugin could not be translated.
func reasonOf(err error) i18n.Message {
	if msgs, ok := i18n.Messages(err); ok {
		return msgs[0]
	}
	return i18n.M("kong.reason.other", "{detail}", "detail", err.Error())
}

// plugins translates the Kong plugins matched by match into an APISIX
// plugins object, recording on it how each one was handled.
func (t *translator) plugins(it *Item, scope pluginScope, match func(*Plugin) bool) map[string]any {
	out := map[string]any{}
	var list []*Plugin
	for i := range t.snap.Plugins {
		if p := &t.snap.Plugins[i]; match(p) {
			list = append(list, p)
		}
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].Name < list[j].Name })

	for _, p := range list {
		c := &pctx{t: t, it: it, p: p, scope: scope}
		pm := PluginMap{Kong: p.Name, Status: "ok"}
		unsupported := func(err error) {
			reason := reasonOf(err)
			if t.set.UnsupportedPlugins == "block" {
				it.pluginNote(LevelError, p.Name, i18n.M("kong.note.plugin_blocks", "the object is blocked so it does not migrate without the plugin"), &reason)
				pm.Status = "error"
			} else {
				it.pluginNote(LevelWarn, p.Name, i18n.M("kong.note.plugin_skipped", "migrated without the plugin"), &reason)
				pm.Status = "warn"
			}
		}
		if !p.Enabled && t.set.DisabledPlugins == "skip" {
			it.pluginNote(LevelInfo, p.Name, i18n.M("kong.note.plugin_disabled", "disabled in Kong; not migrated"), nil)
			pm.Status = "disabled"
			it.Plugins = append(it.Plugins, pm)
			continue
		}
		rule := t.rules[p.Name]
		if rule != nil && rule.Action == RuleIgnore {
			it.pluginNote(LevelInfo, p.Name, i18n.M("kong.note.plugin_ignored", "ignored by the migration rule"), nil)
			pm.Status, pm.Custom = "ignored", true
			it.Plugins = append(it.Plugins, pm)
			continue
		}
		var res map[string]any
		var err error
		switch {
		case p.ConsumerGroup != nil:
			err = i18n.E("kong.reason.consumer_group", "consumer group plugins are not supported")
		case p.Consumer != nil && scope.level != "consumer":
			err = i18n.E("kong.reason.consumer_scoped", "a plugin scoped to a consumer on a route/service has no equivalent in APISIX")
		case rule != nil:
			pm.Custom = true
			res, err = c.custom(rule)
		case builtins[p.Name] != nil:
			res, err = builtins[p.Name](c, cloneMap(p.Config))
		default:
			err = errUnsupported
		}
		if err != nil {
			unsupported(err)
			it.Plugins = append(it.Plugins, pm)
			continue
		}
		for _, name := range slices.Sorted(maps.Keys(res)) {
			conf, _ := res[name].(map[string]any)
			if conf == nil {
				conf = map[string]any{}
			}
			if !p.Enabled {
				conf["_meta"] = map[string]any{"disable": true}
			}
			if prev, ok := out[name].(map[string]any); ok {
				if name == "proxy-rewrite" {
					mergeRewrite(prev, conf)
					continue
				}
				c.note(LevelWarn, i18n.M("kong.note.plugin_overrides", "another Kong plugin also produces {plugin}; this configuration wins", "plugin", name))
			}
			out[name] = conf
			pm.APISIX = append(pm.APISIX, name)
		}
		if c.warned {
			pm.Status = "warn"
		}
		it.Plugins = append(it.Plugins, pm)
	}
	return out
}

func (c *pctx) custom(rule *PluginRule) (map[string]any, error) {
	ctx := map[string]any{
		"config": cloneMap(c.p.Config),
		"plugin": map[string]any{"id": c.p.ID, "name": c.p.Name, "enabled": c.p.Enabled, "protocols": c.p.Protocols, "tags": c.p.Tags},
		"apisix": map[string]any{},
	}
	ids := ctx["apisix"].(map[string]any)
	if s := c.scope.service; s != nil {
		ctx["service"] = toMap(s)
		ids["service_id"] = c.t.ids[s.ID]
		ids["upstream_id"] = c.t.ids[s.ID]
	}
	if r := c.scope.route; r != nil {
		ctx["route"] = toMap(r)
		ids["route_id"] = c.t.ids[r.ID]
	}
	if u := c.scope.consumer; u != nil {
		ctx["consumer"] = toMap(u)
		ids["consumer"] = c.t.usernames[u.ID]
	}
	conf, _ := render(rule.Template, ctx).(map[string]any)
	return map[string]any{rule.APISIXPlugin: conf}, nil
}

// ---- config helpers ----

func str(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

func num(m map[string]any, k string) (float64, bool) {
	n, ok := m[k].(float64)
	return n, ok
}

func numOr(m map[string]any, k string, def float64) float64 {
	if n, ok := num(m, k); ok {
		return n
	}
	return def
}

func boolean(m map[string]any, k string) (bool, bool) {
	b, ok := m[k].(bool)
	return b, ok
}

func obj(m map[string]any, k string) map[string]any {
	o, _ := m[k].(map[string]any)
	return o
}

func anySlice(v any) []any {
	s, _ := v.([]any)
	return s
}

func strs(m map[string]any, k string) []string {
	var out []string
	for _, v := range anySlice(m[k]) {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func cloneMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return toMap(m)
}

func one(name string, conf map[string]any) map[string]any { return map[string]any{name: conf} }

// anonymous resolves Kong's anonymous consumer (id or username).
func (c *pctx) anonymous(conf map[string]any, dst map[string]any) {
	ref := str(conf, "anonymous")
	if ref == "" {
		return
	}
	if u := c.t.snap.consumer(ref); u != nil && c.t.usernames[u.ID] != "" {
		dst["anonymous_consumer"] = c.t.usernames[u.ID]
		c.note(LevelInfo, i18n.M("kong.note.anonymous_migrate", "anonymous consumer '{consumer}': migrate it too", "consumer", c.t.usernames[u.ID]))
		return
	}
	c.note(LevelWarn, i18n.M("kong.note.anonymous_missing", "anonymous consumer '{consumer}' not found in Kong", "consumer", ref))
}

// ---- authentication ----

func keyAuth(c *pctx, conf map[string]any) (map[string]any, error) {
	names := strs(conf, "key_names")
	name := "apikey"
	if len(names) > 0 {
		name = names[0]
	}
	if len(names) > 1 {
		c.note(LevelWarn, i18n.M("kong.note.key_name_single", "APISIX accepts a single key name; using '{name}' (of {names})", "name", name, "names", strings.Join(names, ", ")))
	}
	out := map[string]any{"header": name, "query": name}
	if v, ok := boolean(conf, "hide_credentials"); ok {
		out["hide_credentials"] = v
	}
	if v, _ := boolean(conf, "key_in_body"); v {
		c.note(LevelWarn, i18n.M("kong.note.key_in_body", "key in body (key_in_body) is not supported"))
	}
	if v, ok := boolean(conf, "key_in_query"); ok && !v {
		c.note(LevelInfo, i18n.M("kong.note.key_also_query", "APISIX also accepts the key in the query string"))
	}
	if v, ok := boolean(conf, "key_in_header"); ok && !v {
		c.note(LevelInfo, i18n.M("kong.note.key_also_header", "APISIX also accepts the key in the header"))
	}
	c.anonymous(conf, out)
	return one("key-auth", out), nil
}

func basicAuth(c *pctx, conf map[string]any) (map[string]any, error) {
	out := map[string]any{}
	if v, ok := boolean(conf, "hide_credentials"); ok {
		out["hide_credentials"] = v
	}
	c.anonymous(conf, out)
	c.note(LevelWarn, i18n.M("kong.note.basic_auth_reset", "passwords do not migrate from Kong (hashed); re-create the basic-auth credentials"))
	return one("basic-auth", out), nil
}

func jwtAuth(c *pctx, conf map[string]any) (map[string]any, error) {
	out := map[string]any{"key_claim_name": "iss"}
	if v := str(conf, "key_claim_name"); v != "" {
		out["key_claim_name"] = v
	}
	first := func(k, apisixKey string) {
		if l := strs(conf, k); len(l) > 0 {
			out[apisixKey] = l[0]
			if len(l) > 1 {
				c.note(LevelWarn, i18n.M("kong.note.single_value", "{field}: APISIX accepts a single value; using '{value}'", "field", k, "value", l[0]))
			}
		}
	}
	first("header_names", "header")
	first("uri_param_names", "query")
	first("cookie_names", "cookie")
	var claims []any
	for _, cl := range strs(conf, "claims_to_verify") {
		if cl == "exp" || cl == "nbf" {
			claims = append(claims, cl)
		}
	}
	if len(claims) > 0 {
		out["claims_to_verify"] = claims
	}
	if v, _ := boolean(conf, "secret_is_base64"); v {
		c.note(LevelWarn, i18n.M("kong.note.secret_base64", "secret_is_base64: in APISIX it is per credential (base64_secret); adjust the jwt-auth credentials"))
	}
	if n, _ := num(conf, "maximum_expiration"); n > 0 {
		c.note(LevelWarn, i18n.M("kong.note.max_expiration", "maximum_expiration ({seconds}s) has no equivalent", "seconds", n))
	}
	c.anonymous(conf, out)
	return one("jwt-auth", out), nil
}

func hmacAuth(c *pctx, conf map[string]any) (map[string]any, error) {
	out := map[string]any{}
	if n, ok := num(conf, "clock_skew"); ok && n >= 1 {
		out["clock_skew"] = int(n)
	}
	if h := strs(conf, "enforce_headers"); len(h) > 0 {
		out["signed_headers"] = h
	}
	if a := strs(conf, "algorithms"); len(a) > 0 {
		out["allowed_algorithms"] = a
	}
	for _, k := range []string{"validate_request_body", "hide_credentials"} {
		if v, ok := boolean(conf, k); ok {
			out[k] = v
		}
	}
	c.note(LevelWarn, i18n.M("kong.note.hmac_header", "the signature header changes (Kong: hmac username=...; APISIX: Signature keyId=...); clients must adapt"))
	c.anonymous(conf, out)
	return one("hmac-auth", out), nil
}

// acl maps Kong ACL groups to the APISIX acl plugin, which checks the
// consumer label "groups" (set from the Kong ACLs on migration).
func acl(c *pctx, conf map[string]any) (map[string]any, error) {
	out := map[string]any{}
	if a := strs(conf, "allow"); len(a) > 0 {
		out["allow_labels"] = map[string]any{LabelGroups: toAny(groupLabels(a))}
	}
	if d := strs(conf, "deny"); len(d) > 0 {
		out["deny_labels"] = map[string]any{LabelGroups: toAny(groupLabels(d))}
	}
	if len(out) == 0 {
		return nil, i18n.E("kong.reason.acl_empty", "acl without allow or deny")
	}
	if v, _ := boolean(conf, "hide_groups_header"); !v {
		c.note(LevelInfo, i18n.M("kong.note.consumer_groups_header", "APISIX does not send the X-Consumer-Groups header upstream"))
	}
	return one("acl", out), nil
}

func toAny(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

// ---- traffic control ----

var windows = []struct {
	name    string
	seconds int
}{{"second", 1}, {"minute", 60}, {"hour", 3600}, {"day", 86400}, {"month", 2592000}, {"year", 31536000}}

func rateLimiting(c *pctx, conf map[string]any) (map[string]any, error) {
	var key, keyType string
	switch by := str(conf, "limit_by"); by {
	case "", "consumer", "credential":
		key, keyType = "consumer_name", "var"
		if by == "credential" {
			c.note(LevelInfo, i18n.M("kong.note.limit_by_credential", "limit_by=credential becomes a per-consumer limit"))
		}
	case "ip":
		key, keyType = "remote_addr", "var"
	case "header":
		key, keyType = "http_"+strings.ReplaceAll(strings.ToLower(str(conf, "header_name")), "-", "_"), "var"
	case "path":
		key, keyType = "uri", "var"
		if p := str(conf, "path"); p != "" {
			c.note(LevelWarn, i18n.M("kong.note.limit_by_path", "limit_by=path with fixed path '{path}': APISIX limits by each request URI", "path", p))
		}
	case "service":
		key, keyType = "service", "constant"
	default:
		return nil, i18n.E("kong.reason.limit_by", "limit_by={value} is not supported", "value", by)
	}
	if key == "consumer_name" && c.scope.level != "consumer" {
		c.note(LevelInfo, i18n.M("kong.note.limit_fallback_ip", "without an authenticated consumer the limit falls back to the client IP"))
	}

	out := map[string]any{}
	var rules []any
	for _, w := range windows {
		n, ok := num(conf, w.name)
		if !ok || n <= 0 {
			continue
		}
		rk := "$" + key + ":" + w.name
		if keyType == "constant" {
			rk = key + ":" + w.name
		}
		rules = append(rules, map[string]any{"count": int(n), "time_window": w.seconds, "key": rk, "header_prefix": w.name})
	}
	switch len(rules) {
	case 0:
		return nil, errors.New("nenhum limite definido")
	case 1:
		r := rules[0].(map[string]any)
		out["count"], out["time_window"] = r["count"], r["time_window"]
		out["key"], out["key_type"] = key, keyType
	default:
		// One counter per window, keyed by the same variable.
		out["rules"] = rules
		c.note(LevelInfo, i18n.M("kong.note.limit_windows", "{count} limit windows become limit-count rules (headers prefixed per window)", "count", len(rules)))
	}

	switch policy := str(conf, "policy"); policy {
	case "", "local":
		out["policy"] = "local"
		c.note(LevelInfo, i18n.M("kong.note.policy_local", "local policy: the limit is counted per APISIX instance"))
	case "cluster":
		out["policy"] = "local"
		c.note(LevelWarn, i18n.M("kong.note.policy_cluster", "cluster policy (Kong database) does not exist in APISIX; using local (per instance). Consider redis"))
	case "redis":
		redis := obj(conf, "redis")
		get := func(nested, flat string) any {
			if v, ok := redis[nested]; ok && v != nil {
				return v
			}
			return conf[flat]
		}
		out["policy"] = "redis"
		for apisixKey, src := range map[string][2]string{
			"redis_host": {"host", "redis_host"}, "redis_port": {"port", "redis_port"}, "redis_username": {"username", "redis_username"},
			"redis_password": {"password", "redis_password"}, "redis_database": {"database", "redis_database"},
			"redis_timeout": {"timeout", "redis_timeout"}, "redis_ssl": {"ssl", "redis_ssl"}, "redis_ssl_verify": {"ssl_verify", "redis_ssl_verify"},
			"redis_server_name": {"server_name", "redis_server_name"},
		} {
			v := get(src[0], src[1])
			if v == nil || v == "" {
				continue
			}
			if f, ok := v.(float64); ok {
				v = int(f)
			}
			out[apisixKey] = v
		}
		if out["redis_host"] == nil {
			return nil, i18n.E("kong.reason.redis_no_host", "redis policy without a host")
		}
	default:
		return nil, i18n.E("kong.reason.policy", "policy {value} is not supported", "value", policy)
	}
	if v, ok := boolean(conf, "fault_tolerant"); ok {
		out["allow_degradation"] = v
	}
	if v, ok := boolean(conf, "hide_client_headers"); ok {
		out["show_limit_quota_header"] = !v
	}
	out["rejected_code"] = 429
	if n, ok := num(conf, "error_code"); ok {
		out["rejected_code"] = int(n)
	}
	if m := str(conf, "error_message"); m != "" {
		out["rejected_msg"] = m
	}
	return one("limit-count", out), nil
}

var regexChars = regexp.MustCompile(`[\\^$()|\[\]+?{}]`)

func cors(c *pctx, conf map[string]any) (map[string]any, error) {
	out := map[string]any{}
	origins := strs(conf, "origins")
	var plain, regex []string
	for _, o := range origins {
		if o == "*" {
			plain = []string{"*"}
			regex = nil
			break
		}
		if regexChars.MatchString(o) {
			regex = append(regex, "^"+strings.TrimPrefix(o, "^"))
		} else {
			plain = append(plain, o)
		}
	}
	credentials, _ := boolean(conf, "credentials")
	wildcard := len(origins) == 0 || (len(plain) == 1 && plain[0] == "*")
	switch {
	case wildcard && credentials:
		// APISIX forbids "*" with credentials; "**" reflects the origin, as Kong does.
		out["allow_origins"] = "**"
		c.note(LevelWarn, i18n.M("kong.note.cors_wildcard_credentials", "origin * with credentials: APISIX uses \"**\", which accepts any origin with credentials"))
	case wildcard:
		out["allow_origins"] = "*"
	default:
		if len(plain) > 0 {
			out["allow_origins"] = strings.Join(plain, ",")
		}
		if len(regex) > 0 {
			out["allow_origins_by_regex"] = toAny(regex)
		}
	}
	join := func(k, apisixKey string, def string) {
		if l := strs(conf, k); len(l) > 0 {
			out[apisixKey] = strings.Join(l, ",")
		} else if def != "" {
			out[apisixKey] = def
		}
	}
	join("methods", "allow_methods", "GET,HEAD,PUT,PATCH,POST,DELETE,OPTIONS,TRACE,CONNECT")
	join("headers", "allow_headers", "")
	join("exposed_headers", "expose_headers", "")
	if credentials {
		out["allow_credential"] = true
		if out["allow_headers"] == nil {
			out["allow_headers"] = "**"
		}
		if out["allow_origins"] != "**" {
			out["allow_methods"] = strings.ReplaceAll(out["allow_methods"].(string), "*", "**")
		}
	}
	if n, ok := num(conf, "max_age"); ok {
		out["max_age"] = int(n)
	}
	if v, _ := boolean(conf, "preflight_continue"); v {
		c.note(LevelWarn, i18n.M("kong.note.preflight_continue", "preflight_continue has no equivalent (APISIX answers the preflight)"))
	}
	return one("cors", out), nil
}

func ipRestriction(c *pctx, conf map[string]any) (map[string]any, error) {
	out := map[string]any{}
	allow, deny := strs(conf, "allow"), strs(conf, "deny")
	switch {
	case len(allow) > 0:
		out["whitelist"] = toAny(allow)
		if len(deny) > 0 {
			c.note(LevelWarn, i18n.M("kong.note.ip_allow_deny", "APISIX does not combine allow and deny; using allow only ({count} deny ranges dropped)", "count", len(deny)))
		}
	case len(deny) > 0:
		out["blacklist"] = toAny(deny)
	default:
		return nil, i18n.E("kong.reason.no_allow_deny", "no allow or deny list")
	}
	if n, ok := num(conf, "status"); ok {
		out["response_code"] = int(n)
	}
	if m := str(conf, "message"); m != "" {
		out["message"] = m
	}
	return one("ip-restriction", out), nil
}

// headerPairs reads Kong "name:value" lists.
func headerPairs(c *pctx, list []string) map[string]any {
	out := map[string]any{}
	for _, h := range list {
		k, v, ok := strings.Cut(h, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		if strings.Contains(v, "$(") {
			c.note(LevelWarn, i18n.M("kong.note.lua_template", "Lua template in '{field}' is not supported; the value is copied as text", "field", k))
		}
		out[strings.TrimSpace(k)] = v
	}
	return out
}

func requestTransformer(c *pctx, conf map[string]any) (map[string]any, error) {
	out := map[string]any{}
	headers := map[string]any{}
	set := map[string]any{}
	for _, part := range []string{"add", "replace"} {
		maps.Copy(set, headerPairs(c, strs(obj(conf, part), "headers")))
	}
	if len(strs(obj(conf, "add"), "headers")) > 0 {
		c.note(LevelInfo, i18n.M("kong.note.add_headers_set", "add.headers becomes set: the header is overwritten even if it exists"))
	}
	for _, pair := range strs(obj(conf, "rename"), "headers") {
		from, to, ok := strings.Cut(pair, ":")
		if !ok {
			continue
		}
		from, to = strings.TrimSpace(from), strings.TrimSpace(to)
		set[to] = "$http_" + strings.ReplaceAll(strings.ToLower(from), "-", "_")
		headers["remove"] = append(anySlice(headers["remove"]), from)
	}
	if len(set) > 0 {
		headers["set"] = set
	}
	if add := headerPairs(c, strs(obj(conf, "append"), "headers")); len(add) > 0 {
		headers["add"] = add
	}
	if rm := strs(obj(conf, "remove"), "headers"); len(rm) > 0 {
		headers["remove"] = append(anySlice(headers["remove"]), toAny(rm)...)
	}
	if len(headers) > 0 {
		out["headers"] = headers
	}
	if u := str(obj(conf, "replace"), "uri"); u != "" {
		out["uri"] = u
	}
	if m := str(conf, "http_method"); m != "" {
		out["method"] = strings.ToUpper(m)
	}
	for _, part := range []string{"add", "append", "replace", "remove", "rename"} {
		p := obj(conf, part)
		if len(strs(p, "querystring")) > 0 || len(strs(p, "body")) > 0 {
			c.note(LevelWarn, i18n.M("kong.note.rewrite_unsupported_part", "{part} of querystring/body is not supported by proxy-rewrite", "part", part))
		}
	}
	if len(out) == 0 {
		c.note(LevelInfo, i18n.M("kong.note.no_supported_transform", "no supported transformation configured"))
		return map[string]any{}, nil
	}
	return one("proxy-rewrite", out), nil
}

func responseTransformer(c *pctx, conf map[string]any) (map[string]any, error) {
	headers := map[string]any{}
	set := map[string]any{}
	for _, part := range []string{"add", "replace"} {
		maps.Copy(set, headerPairs(c, strs(obj(conf, part), "headers")))
	}
	for _, pair := range strs(obj(conf, "rename"), "headers") {
		from, to, ok := strings.Cut(pair, ":")
		if !ok {
			continue
		}
		from, to = strings.TrimSpace(from), strings.TrimSpace(to)
		set[to] = "$upstream_http_" + strings.ReplaceAll(strings.ToLower(from), "-", "_")
		headers["remove"] = append(anySlice(headers["remove"]), from)
	}
	if len(set) > 0 {
		headers["set"] = set
	}
	if add := headerPairs(c, strs(obj(conf, "append"), "headers")); len(add) > 0 {
		headers["add"] = add
	}
	if rm := strs(obj(conf, "remove"), "headers"); len(rm) > 0 {
		headers["remove"] = append(anySlice(headers["remove"]), toAny(rm)...)
	}
	for _, part := range []string{"add", "append", "replace", "remove", "rename"} {
		if len(strs(obj(conf, part), "json")) > 0 {
			c.note(LevelWarn, i18n.M("kong.note.response_json", "{part}.json (response body) is not supported", "part", part))
		}
	}
	if len(headers) == 0 {
		c.note(LevelInfo, i18n.M("kong.note.no_supported_transform", "no supported transformation configured"))
		return map[string]any{}, nil
	}
	return one("response-rewrite", map[string]any{"headers": headers}), nil
}

func correlationID(c *pctx, conf map[string]any) (map[string]any, error) {
	out := map[string]any{"header_name": "Kong-Request-ID", "algorithm": "uuid"}
	if h := str(conf, "header_name"); h != "" {
		out["header_name"] = h
	}
	if v, ok := boolean(conf, "echo_downstream"); ok {
		out["include_in_response"] = v
	}
	if g := str(conf, "generator"); g != "" && g != "uuid" {
		c.note(LevelInfo, i18n.M("kong.note.generator_uuid", "generator {generator} becomes uuid", "generator", g))
	}
	return one("request-id", out), nil
}

func prometheus(c *pctx, conf map[string]any) (map[string]any, error) {
	c.note(LevelInfo, i18n.M("kong.note.metrics_names", "APISIX metrics have different names (apisix_*); adjust dashboards and alerts"))
	return one("prometheus", map[string]any{"prefer_name": true}), nil
}

func logNote(c *pctx, conf map[string]any) {
	c.note(LevelInfo, i18n.M("kong.note.log_format", "the APISIX log format differs from Kong; adjust the parser at the destination"))
	if len(obj(conf, "custom_fields_by_lua")) > 0 {
		c.note(LevelWarn, i18n.M("kong.note.custom_fields_lua", "custom_fields_by_lua is not supported (use log_format)"))
	}
}

func fileLog(c *pctx, conf map[string]any) (map[string]any, error) {
	p := str(conf, "path")
	if p == "" {
		return nil, i18n.E("kong.reason.no_path", "no path")
	}
	logNote(c, conf)
	return one("file-logger", map[string]any{"path": p}), nil
}

func httpLog(c *pctx, conf map[string]any) (map[string]any, error) {
	u := str(conf, "http_endpoint")
	if u == "" {
		return nil, i18n.E("kong.reason.no_http_endpoint", "no http_endpoint")
	}
	out := map[string]any{"uri": u}
	if n, ok := num(conf, "timeout"); ok && n > 0 {
		out["timeout"] = max(1, int(n/1000))
	}
	queue := obj(conf, "queue")
	if n, ok := num(queue, "max_batch_size"); ok && n > 0 {
		out["batch_max_size"] = int(n)
	}
	if n, ok := num(queue, "max_coalescing_delay"); ok && n >= 1 {
		out["inactive_timeout"] = int(n)
	}
	if n, ok := num(queue, "max_retry_time"); ok && n > 0 {
		out["max_retry_count"] = 3
	}
	for k, v := range obj(conf, "headers") {
		if strings.EqualFold(k, "authorization") {
			out["auth_header"] = fmt.Sprint(v)
		} else {
			c.note(LevelWarn, i18n.M("kong.note.http_logger_header", "header '{header}' is not supported by http-logger (Authorization only)", "header", k))
		}
	}
	logNote(c, conf)
	return one("http-logger", out), nil
}

func tcpLog(c *pctx, conf map[string]any) (map[string]any, error) {
	out := map[string]any{"host": str(conf, "host")}
	if n, ok := num(conf, "port"); ok {
		out["port"] = int(n)
	}
	if v, ok := boolean(conf, "tls"); ok {
		out["tls"] = v
	}
	if s := str(conf, "tls_sni"); s != "" {
		c.note(LevelWarn, i18n.M("kong.note.tcp_logger_sni", "tls_sni '{sni}' has no equivalent in tcp-logger", "sni", s))
	}
	if n, ok := num(conf, "timeout"); ok && n > 0 {
		out["timeout"] = int(n)
	}
	logNote(c, conf)
	return one("tcp-logger", out), nil
}

func udpLog(c *pctx, conf map[string]any) (map[string]any, error) {
	out := map[string]any{"host": str(conf, "host")}
	if n, ok := num(conf, "port"); ok {
		out["port"] = int(n)
	}
	if n, ok := num(conf, "timeout"); ok && n > 0 {
		out["timeout"] = max(1, int(n/1000))
	}
	logNote(c, conf)
	return one("udp-logger", out), nil
}

func proxyCache(c *pctx, conf map[string]any) (map[string]any, error) {
	if s := str(conf, "strategy"); s != "" && s != "memory" {
		return nil, i18n.E("kong.reason.strategy", "strategy {value} is not supported", "value", s)
	}
	out := map[string]any{"cache_strategy": "memory", "cache_zone": "memory_cache"}
	if n, ok := num(conf, "cache_ttl"); ok {
		out["cache_ttl"] = int(n)
	}
	if l := anySlice(conf["request_method"]); len(l) > 0 {
		out["cache_method"] = l
	}
	if l := anySlice(conf["response_code"]); len(l) > 0 {
		out["cache_http_status"] = l
	}
	if v, ok := boolean(conf, "cache_control"); ok {
		out["cache_control"] = v
	}
	if len(anySlice(conf["vary_headers"])) > 0 || len(anySlice(conf["vary_query_params"])) > 0 {
		c.note(LevelWarn, i18n.M("kong.note.cache_vary", "vary_headers/vary_query_params: adjust cache_key manually"))
	}
	if len(anySlice(conf["content_type"])) > 0 {
		c.note(LevelInfo, i18n.M("kong.note.cache_content_type", "the content_type filter does not exist in APISIX"))
	}
	c.note(LevelInfo, i18n.M("kong.note.cache_zone", "requires the memory_cache zone in apisix.proxy_cache in config.yaml"))
	return one("proxy-cache", out), nil
}

func requestSizeLimiting(c *pctx, conf map[string]any) (map[string]any, error) {
	size := numOr(conf, "allowed_payload_size", 128)
	mult := map[string]float64{"": 1 << 20, "megabytes": 1 << 20, "kilobytes": 1 << 10, "bytes": 1}[str(conf, "size_unit")]
	if mult == 0 {
		mult = 1 << 20
	}
	if v, _ := boolean(conf, "require_content_length"); v {
		c.note(LevelWarn, i18n.M("kong.note.require_content_length", "require_content_length has no equivalent"))
	}
	return one("client-control", map[string]any{"max_body_size": int(size * mult)}), nil
}

func botDetection(c *pctx, conf map[string]any) (map[string]any, error) {
	allow, deny := strs(conf, "allow"), strs(conf, "deny")
	c.note(LevelWarn, i18n.M("kong.note.bot_builtin_list", "Kong blocks a built-in bot list; APISIX only uses the configured lists"))
	switch {
	case len(deny) > 0:
		if len(allow) > 0 {
			c.note(LevelWarn, i18n.M("kong.note.bot_allow_deny", "APISIX does not combine allowlist and denylist; using deny only"))
		}
		return one("ua-restriction", map[string]any{"denylist": toAny(deny)}), nil
	case len(allow) > 0:
		c.note(LevelWarn, i18n.M("kong.note.bot_allow_only", "allow-only in Kong whitelists exceptions to the built-in list; in APISIX it becomes an allowlist (blocks everything else)"))
		return one("ua-restriction", map[string]any{"allowlist": toAny(allow)}), nil
	}
	return nil, i18n.E("kong.reason.bot_no_lists", "no allow/deny lists: Kong's built-in list does not exist in APISIX")
}

func requestTermination(c *pctx, conf map[string]any) (map[string]any, error) {
	abort := map[string]any{"http_status": int(numOr(conf, "status_code", 503))}
	switch {
	case str(conf, "body") != "":
		abort["body"] = str(conf, "body")
		if ct := str(conf, "content_type"); ct != "" {
			abort["headers"] = map[string]any{"Content-Type": ct}
		}
	case str(conf, "message") != "":
		b := fmt.Sprintf(`{"message":%q}`, str(conf, "message"))
		abort["body"] = b
		abort["headers"] = map[string]any{"Content-Type": "application/json; charset=utf-8"}
	}
	if v, _ := boolean(conf, "echo"); v {
		c.note(LevelWarn, i18n.M("kong.note.echo_unsupported", "echo is not supported"))
	}
	if t := str(conf, "trigger"); t != "" {
		c.note(LevelWarn, i18n.M("kong.note.trigger_unsupported", "trigger '{trigger}' is not supported: the route always answers", "trigger", t))
	}
	return one("fault-injection", map[string]any{"abort": abort}), nil
}

func openIDConnect(c *pctx, conf map[string]any) (map[string]any, error) {
	issuer := str(conf, "issuer")
	ids := strs(conf, "client_id")
	if issuer == "" || len(ids) == 0 {
		return nil, i18n.E("kong.reason.oidc_incomplete", "missing issuer or client_id")
	}
	discovery := issuer
	if !strings.Contains(discovery, "/.well-known/") {
		discovery = strings.TrimSuffix(discovery, "/") + "/.well-known/openid-configuration"
	}
	out := map[string]any{"discovery": discovery, "client_id": ids[0]}
	if s := strs(conf, "client_secret"); len(s) > 0 {
		out["client_secret"] = s[0]
	}
	if s := strs(conf, "scopes"); len(s) > 0 {
		out["scope"] = strings.Join(s, " ")
	}
	methods := strs(conf, "auth_methods")
	if len(methods) > 0 && !slices.Contains(methods, "authorization_code") && !slices.Contains(methods, "session") {
		out["bearer_only"] = true
	}
	if r := strs(conf, "redirect_uri"); len(r) > 0 {
		out["redirect_uri"] = r[0]
	}
	c.note(LevelWarn, i18n.M("kong.note.oidc_review", "Kong Enterprise openid-connect has many options; review the generated configuration"))
	return one("openid-connect", out), nil
}
