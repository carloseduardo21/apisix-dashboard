package kong

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

// Settings are the migration defaults chosen in the UI. They are applied to
// every step, so services migrated on different days follow the same rules.
type Settings struct {
	// IDPrefix is prepended to the ids of routes, services, upstreams and
	// global rules (not to consumer usernames, which are identities).
	IDPrefix string `json:"id_prefix"`
	// IDStrategy: "name" uses the Kong name (falling back to the Kong id),
	// "kong_id" always uses the Kong id.
	IDStrategy string `json:"id_strategy"`
	// RouteStatus: "disabled" creates routes with status 0, to be enabled at
	// cutover; "enabled" creates them active.
	RouteStatus string `json:"route_status"`
	// OnConflict applies when an object with the same id exists in APISIX and
	// was not created by this migration: "skip" or "overwrite".
	OnConflict string `json:"on_conflict"`
	// UnsupportedPlugins: "block" stops the migration of the object (safe
	// default: a missing auth plugin would expose the route), "skip" migrates
	// it without the plugin.
	UnsupportedPlugins string `json:"unsupported_plugins"`
	// DisabledPlugins: "skip" ignores plugins disabled in Kong, "keep"
	// migrates them disabled (_meta.disable).
	DisabledPlugins string `json:"disabled_plugins"`
	// EnableWebsocket sets enable_websocket on routes: Kong proxies WebSocket
	// upgrades on any HTTP route, APISIX only when enabled.
	EnableWebsocket bool `json:"enable_websocket"`
	// HostRewrites replace upstream hosts, e.g. a Kong-side DNS name by the
	// Swarm service name reachable from APISIX.
	HostRewrites []HostRewrite `json:"host_rewrites"`
	// Labels are added to every migrated object that supports labels.
	Labels map[string]string `json:"labels"`
	// PluginRules override the built-in plugin translation, for custom
	// plugins or a different mapping of standard ones.
	PluginRules []PluginRule `json:"plugin_rules"`
}

type HostRewrite struct {
	From string `json:"from"`
	To   string `json:"to"` // host or host:port
}

const (
	RuleMap    = "map"
	RuleIgnore = "ignore"
)

// PluginRule maps a Kong plugin to an APISIX plugin through a JSON template.
// In the template, a string "${config.field}" is replaced by that value of
// the Kong plugin (keeping its type) and "${config.field|default}" gives a
// default; placeholders inside longer strings are interpolated. Fields whose
// value is missing are omitted. See render for the available roots.
type PluginRule struct {
	KongPlugin   string         `json:"kong_plugin"`
	Action       string         `json:"action"`
	APISIXPlugin string         `json:"apisix_plugin,omitempty"`
	Template     map[string]any `json:"template,omitempty"`
}

func DefaultSettings() Settings {
	return Settings{
		IDStrategy:         "name",
		RouteStatus:        "disabled",
		OnConflict:         "skip",
		UnsupportedPlugins: "block",
		DisabledPlugins:    "skip",
		EnableWebsocket:    true,
		HostRewrites:       []HostRewrite{},
		Labels:             map[string]string{},
		PluginRules:        []PluginRule{},
	}
}

var (
	prefixRe     = regexp.MustCompile(`^[A-Za-z0-9._-]{0,20}$`)
	labelValueRe = regexp.MustCompile(`^\S{1,256}$`)
	pluginNameRe = regexp.MustCompile(`^[a-z0-9_-]{1,128}$`)
	hostRe       = regexp.MustCompile(`^[A-Za-z0-9._-]{1,253}$`)
)

// reservedLabels are set by the migration itself.
var reservedLabels = map[string]bool{LabelMigratedFrom: true, LabelKongID: true, LabelGroups: true}

func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

// Validate checks the settings; its messages are shown in the UI.
func (s *Settings) Validate() error {
	var errs []error
	if !prefixRe.MatchString(s.IDPrefix) {
		errs = append(errs, errors.New("prefixo de ID: até 20 caracteres entre letras, números, '.', '_' e '-'"))
	}
	if !oneOf(s.IDStrategy, "name", "kong_id") {
		errs = append(errs, errors.New("estratégia de ID inválida"))
	}
	if !oneOf(s.RouteStatus, "disabled", "enabled") {
		errs = append(errs, errors.New("status das rotas inválido"))
	}
	if !oneOf(s.OnConflict, "skip", "overwrite") {
		errs = append(errs, errors.New("política de conflito inválida"))
	}
	if !oneOf(s.UnsupportedPlugins, "block", "skip") {
		errs = append(errs, errors.New("política para plugins sem tradução inválida"))
	}
	if !oneOf(s.DisabledPlugins, "skip", "keep") {
		errs = append(errs, errors.New("política para plugins desabilitados inválida"))
	}
	for i, h := range s.HostRewrites {
		if !hostRe.MatchString(h.From) {
			errs = append(errs, fmt.Errorf("reescrita de host %d: origem inválida %q", i+1, h.From))
		}
		if _, _, err := splitHostPort(h.To); err != nil {
			errs = append(errs, fmt.Errorf("reescrita de host %d: destino inválido %q (use host ou host:porta)", i+1, h.To))
		}
	}
	for k, v := range s.Labels {
		if k == "" || reservedLabels[k] {
			errs = append(errs, fmt.Errorf("label %q é reservado ou vazio", k))
		}
		if !labelValueRe.MatchString(v) {
			errs = append(errs, fmt.Errorf("label %q: o valor não pode ser vazio nem ter espaços", k))
		}
	}
	seen := map[string]bool{}
	for i, r := range s.PluginRules {
		n := i + 1
		if !pluginNameRe.MatchString(r.KongPlugin) {
			errs = append(errs, fmt.Errorf("regra %d: nome do plugin do Kong inválido", n))
		}
		if seen[r.KongPlugin] {
			errs = append(errs, fmt.Errorf("regra %d: já existe uma regra para %q", n, r.KongPlugin))
		}
		seen[r.KongPlugin] = true
		switch r.Action {
		case RuleIgnore:
		case RuleMap:
			if !pluginNameRe.MatchString(r.APISIXPlugin) {
				errs = append(errs, fmt.Errorf("regra %d: informe o plugin do APISIX", n))
			}
			if r.Template == nil {
				errs = append(errs, fmt.Errorf("regra %d: informe o template (objeto JSON)", n))
			} else if err := checkTemplate(r.Template); err != nil {
				errs = append(errs, fmt.Errorf("regra %d: %v", n, err))
			}
		default:
			errs = append(errs, fmt.Errorf("regra %d: ação inválida", n))
		}
	}
	return errors.Join(errs...)
}

// Normalize fills nil collections so the JSON shape is stable.
func (s *Settings) Normalize() {
	if s.HostRewrites == nil {
		s.HostRewrites = []HostRewrite{}
	}
	if s.Labels == nil {
		s.Labels = map[string]string{}
	}
	if s.PluginRules == nil {
		s.PluginRules = []PluginRule{}
	}
}

// ParseSettings decodes stored settings over the defaults, so fields added
// later get their default value.
func ParseSettings(b []byte) (Settings, error) {
	s := DefaultSettings()
	if len(b) > 0 {
		if err := json.Unmarshal(b, &s); err != nil {
			return s, err
		}
	}
	s.Normalize()
	return s, nil
}

// splitHostPort accepts "host" or "host:port"; port is 0 when absent.
func splitHostPort(v string) (string, int, error) {
	if hostRe.MatchString(v) {
		return v, 0, nil
	}
	h, p, err := net.SplitHostPort(v)
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(p)
	if err != nil || port < 1 || port > 65535 {
		return "", 0, errors.New("porta inválida")
	}
	if !hostRe.MatchString(strings.Trim(h, "[]")) && net.ParseIP(h) == nil {
		return "", 0, errors.New("host inválido")
	}
	return h, port, nil
}
