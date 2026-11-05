package kong

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strings"
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

func (c *pctx) note(level, format string, args ...any) {
	c.it.note(level, c.p.Name+": "+format, args...)
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

var errUnsupported = errors.New("sem tradução automática")

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
		unsupported := func(reason string) {
			if t.set.UnsupportedPlugins == "block" {
				it.note(LevelError, "%s: %s (o objeto fica bloqueado para não migrar sem o plugin)", p.Name, reason)
				pm.Status = "error"
			} else {
				it.note(LevelWarn, "%s: %s; migrado sem o plugin", p.Name, reason)
				pm.Status = "warn"
			}
		}
		if !p.Enabled && t.set.DisabledPlugins == "skip" {
			it.note(LevelInfo, "%s: desabilitado no Kong; não migrado", p.Name)
			pm.Status = "disabled"
			it.Plugins = append(it.Plugins, pm)
			continue
		}
		rule := t.rules[p.Name]
		if rule != nil && rule.Action == RuleIgnore {
			it.note(LevelInfo, "%s: ignorado pela regra da migração", p.Name)
			pm.Status, pm.Custom = "ignored", true
			it.Plugins = append(it.Plugins, pm)
			continue
		}
		var res map[string]any
		var err error
		switch {
		case p.ConsumerGroup != nil:
			err = errors.New("plugins por consumer group não são suportados")
		case p.Consumer != nil && scope.level != "consumer":
			err = errors.New("plugin restrito a um consumer numa rota/service não tem equivalente no APISIX")
		case rule != nil:
			pm.Custom = true
			res, err = c.custom(rule)
		case builtins[p.Name] != nil:
			res, err = builtins[p.Name](c, cloneMap(p.Config))
		default:
			err = errUnsupported
		}
		if err != nil {
			unsupported(err.Error())
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
				c.note(LevelWarn, "outro plugin do Kong também gera %s; esta configuração prevalece", name)
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
		c.note(LevelInfo, "consumer anônimo %q: migre-o também", c.t.usernames[u.ID])
		return
	}
	c.note(LevelWarn, "consumer anônimo %q não encontrado no Kong", ref)
}

// ---- authentication ----

func keyAuth(c *pctx, conf map[string]any) (map[string]any, error) {
	names := strs(conf, "key_names")
	name := "apikey"
	if len(names) > 0 {
		name = names[0]
	}
	if len(names) > 1 {
		c.note(LevelWarn, "o APISIX aceita um único nome de chave; usando %q (de %s)", name, strings.Join(names, ", "))
	}
	out := map[string]any{"header": name, "query": name}
	if v, ok := boolean(conf, "hide_credentials"); ok {
		out["hide_credentials"] = v
	}
	if v, _ := boolean(conf, "key_in_body"); v {
		c.note(LevelWarn, "chave no corpo (key_in_body) não é suportada")
	}
	if v, ok := boolean(conf, "key_in_query"); ok && !v {
		c.note(LevelInfo, "o APISIX também aceita a chave na query string")
	}
	if v, ok := boolean(conf, "key_in_header"); ok && !v {
		c.note(LevelInfo, "o APISIX também aceita a chave no header")
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
	c.note(LevelWarn, "as senhas não migram do Kong (hash); recadastre as credenciais basic-auth")
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
				c.note(LevelWarn, "%s: o APISIX aceita um único valor; usando %q", k, l[0])
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
		c.note(LevelWarn, "secret_is_base64: no APISIX é por credencial (base64_secret); ajuste as credenciais jwt-auth")
	}
	if n, _ := num(conf, "maximum_expiration"); n > 0 {
		c.note(LevelWarn, "maximum_expiration (%vs) não tem equivalente", n)
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
	c.note(LevelWarn, "o header de assinatura muda (Kong: hmac username=...; APISIX: Signature keyId=...); os clientes precisam se adaptar")
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
		return nil, errors.New("acl sem allow nem deny")
	}
	if v, _ := boolean(conf, "hide_groups_header"); !v {
		c.note(LevelInfo, "o APISIX não envia o header X-Consumer-Groups ao upstream")
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
			c.note(LevelInfo, "limit_by=credential vira limite por consumer")
		}
	case "ip":
		key, keyType = "remote_addr", "var"
	case "header":
		key, keyType = "http_"+strings.ReplaceAll(strings.ToLower(str(conf, "header_name")), "-", "_"), "var"
	case "path":
		key, keyType = "uri", "var"
		if p := str(conf, "path"); p != "" {
			c.note(LevelWarn, "limit_by=path com path fixo %q: o APISIX limita por URI de cada requisição", p)
		}
	case "service":
		key, keyType = "service", "constant"
	default:
		return nil, fmt.Errorf("limit_by=%s não suportado", by)
	}
	if key == "consumer_name" && c.scope.level != "consumer" {
		c.note(LevelInfo, "sem consumer autenticado o limite passa a ser por IP")
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
		c.note(LevelInfo, "%d janelas de limite viram regras do limit-count (headers com prefixo por janela)", len(rules))
	}

	switch policy := str(conf, "policy"); policy {
	case "", "local":
		out["policy"] = "local"
		c.note(LevelInfo, "policy local: o limite é contado por instância do APISIX")
	case "cluster":
		out["policy"] = "local"
		c.note(LevelWarn, "policy cluster (banco do Kong) não existe no APISIX; usando local (por instância). Considere redis")
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
			return nil, errors.New("policy redis sem host")
		}
	default:
		return nil, fmt.Errorf("policy %s não suportada", policy)
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
		c.note(LevelWarn, "origem * com credentials: o APISIX usa \"**\", que aceita qualquer origem com credenciais")
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
		c.note(LevelWarn, "preflight_continue não tem equivalente (o APISIX responde o preflight)")
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
			c.note(LevelWarn, "o APISIX não combina allow e deny; usando só allow (%d faixas de deny descartadas)", len(deny))
		}
	case len(deny) > 0:
		out["blacklist"] = toAny(deny)
	default:
		return nil, errors.New("sem allow nem deny")
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
			c.note(LevelWarn, "template Lua em %q não é suportado; o valor é copiado como texto", k)
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
		c.note(LevelInfo, "add.headers vira set: o header é sobrescrito mesmo se já existir")
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
			c.note(LevelWarn, "%s de querystring/body não é suportado pelo proxy-rewrite", part)
		}
	}
	if len(out) == 0 {
		c.note(LevelInfo, "nenhuma transformação suportada configurada")
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
			c.note(LevelWarn, "%s.json (corpo da resposta) não é suportado", part)
		}
	}
	if len(headers) == 0 {
		c.note(LevelInfo, "nenhuma transformação suportada configurada")
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
		c.note(LevelInfo, "gerador %s vira uuid", g)
	}
	return one("request-id", out), nil
}

func prometheus(c *pctx, conf map[string]any) (map[string]any, error) {
	c.note(LevelInfo, "as métricas do APISIX têm outros nomes (apisix_*); ajuste dashboards e alertas")
	return one("prometheus", map[string]any{"prefer_name": true}), nil
}

func logNote(c *pctx, conf map[string]any) {
	c.note(LevelInfo, "o formato do log do APISIX é diferente do Kong; ajuste o parser no destino")
	if len(obj(conf, "custom_fields_by_lua")) > 0 {
		c.note(LevelWarn, "custom_fields_by_lua não é suportado (use log_format)")
	}
}

func fileLog(c *pctx, conf map[string]any) (map[string]any, error) {
	p := str(conf, "path")
	if p == "" {
		return nil, errors.New("sem path")
	}
	logNote(c, conf)
	return one("file-logger", map[string]any{"path": p}), nil
}

func httpLog(c *pctx, conf map[string]any) (map[string]any, error) {
	u := str(conf, "http_endpoint")
	if u == "" {
		return nil, errors.New("sem http_endpoint")
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
			c.note(LevelWarn, "header %q não é suportado pelo http-logger (só Authorization)", k)
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
		c.note(LevelWarn, "tls_sni %q não tem equivalente no tcp-logger", s)
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
		return nil, fmt.Errorf("strategy %s não suportada", s)
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
		c.note(LevelWarn, "vary_headers/vary_query_params: ajuste cache_key manualmente")
	}
	if len(anySlice(conf["content_type"])) > 0 {
		c.note(LevelInfo, "o filtro por content_type não existe no APISIX")
	}
	c.note(LevelInfo, "requer a zona memory_cache em apisix.proxy_cache no config.yaml")
	return one("proxy-cache", out), nil
}

func requestSizeLimiting(c *pctx, conf map[string]any) (map[string]any, error) {
	size := numOr(conf, "allowed_payload_size", 128)
	mult := map[string]float64{"": 1 << 20, "megabytes": 1 << 20, "kilobytes": 1 << 10, "bytes": 1}[str(conf, "size_unit")]
	if mult == 0 {
		mult = 1 << 20
	}
	if v, _ := boolean(conf, "require_content_length"); v {
		c.note(LevelWarn, "require_content_length não tem equivalente")
	}
	return one("client-control", map[string]any{"max_body_size": int(size * mult)}), nil
}

func botDetection(c *pctx, conf map[string]any) (map[string]any, error) {
	allow, deny := strs(conf, "allow"), strs(conf, "deny")
	c.note(LevelWarn, "o Kong bloqueia uma lista embutida de bots; o APISIX só usa as listas configuradas")
	switch {
	case len(deny) > 0:
		if len(allow) > 0 {
			c.note(LevelWarn, "o APISIX não combina allowlist e denylist; usando só deny")
		}
		return one("ua-restriction", map[string]any{"denylist": toAny(deny)}), nil
	case len(allow) > 0:
		c.note(LevelWarn, "só allow no Kong libera exceções à lista embutida; no APISIX vira allowlist (bloqueia todo o resto)")
		return one("ua-restriction", map[string]any{"allowlist": toAny(allow)}), nil
	}
	return nil, errors.New("sem listas allow/deny: a lista embutida do Kong não existe no APISIX")
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
		c.note(LevelWarn, "echo não é suportado")
	}
	if t := str(conf, "trigger"); t != "" {
		c.note(LevelWarn, "trigger %q não é suportado: a rota sempre responde", t)
	}
	return one("fault-injection", map[string]any{"abort": abort}), nil
}

func openIDConnect(c *pctx, conf map[string]any) (map[string]any, error) {
	issuer := str(conf, "issuer")
	ids := strs(conf, "client_id")
	if issuer == "" || len(ids) == 0 {
		return nil, errors.New("sem issuer ou client_id")
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
	c.note(LevelWarn, "o openid-connect do Kong Enterprise tem muitas opções; revise a configuração gerada")
	return one("openid-connect", out), nil
}
