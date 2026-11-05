package kong

import (
	"encoding/json"
	"errors"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/carloseduardo21/apisix-dashboard/internal/i18n"
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
		errs = append(errs, i18n.E("kong.settings.id_prefix", "ID prefix: up to 20 letters, digits, '.', '_' and '-'"))
	}
	if !oneOf(s.IDStrategy, "name", "kong_id") {
		errs = append(errs, i18n.E("kong.settings.id_strategy", "invalid ID strategy"))
	}
	if !oneOf(s.RouteStatus, "disabled", "enabled") {
		errs = append(errs, i18n.E("kong.settings.route_status", "invalid route status"))
	}
	if !oneOf(s.OnConflict, "skip", "overwrite") {
		errs = append(errs, i18n.E("kong.settings.on_conflict", "invalid conflict policy"))
	}
	if !oneOf(s.UnsupportedPlugins, "block", "skip") {
		errs = append(errs, i18n.E("kong.settings.unsupported_plugins", "invalid policy for plugins without translation"))
	}
	if !oneOf(s.DisabledPlugins, "skip", "keep") {
		errs = append(errs, i18n.E("kong.settings.disabled_plugins", "invalid policy for disabled plugins"))
	}
	for i, h := range s.HostRewrites {
		if !hostRe.MatchString(h.From) {
			errs = append(errs, i18n.E("kong.settings.rewrite_from", "host rewrite {n}: invalid source '{value}'", "n", i+1, "value", h.From))
		}
		if _, _, err := splitHostPort(h.To); err != nil {
			errs = append(errs, i18n.E("kong.settings.rewrite_to", "host rewrite {n}: invalid target '{value}' (use host or host:port)", "n", i+1, "value", h.To))
		}
	}
	for k, v := range s.Labels {
		if k == "" || reservedLabels[k] {
			errs = append(errs, i18n.E("kong.settings.label_reserved", "label '{key}' is reserved or empty", "key", k))
		}
		if !labelValueRe.MatchString(v) {
			errs = append(errs, i18n.E("kong.settings.label_value", "label '{key}': the value cannot be empty or contain spaces", "key", k))
		}
	}
	seen := map[string]bool{}
	for i, r := range s.PluginRules {
		n := i + 1
		if !pluginNameRe.MatchString(r.KongPlugin) {
			errs = append(errs, i18n.E("kong.settings.rule_kong_plugin", "rule {n}: invalid Kong plugin name", "n", n))
		}
		if seen[r.KongPlugin] {
			errs = append(errs, i18n.E("kong.settings.rule_duplicate", "rule {n}: a rule for '{plugin}' already exists", "n", n, "plugin", r.KongPlugin))
		}
		seen[r.KongPlugin] = true
		switch r.Action {
		case RuleIgnore:
		case RuleMap:
			if !pluginNameRe.MatchString(r.APISIXPlugin) {
				errs = append(errs, i18n.E("kong.settings.rule_apisix_plugin", "rule {n}: choose the APISIX plugin", "n", n))
			}
			if r.Template == nil {
				errs = append(errs, i18n.E("kong.settings.rule_template", "rule {n}: enter the template (JSON object)", "n", n))
			} else if err := checkTemplate(r.Template); err != nil {
				errs = append(errs, i18n.Wrap(err, "kong.settings.rule_template_invalid", "rule {n}: {detail}", "n", n, "detail", err.Error()))
			}
		default:
			errs = append(errs, i18n.E("kong.settings.rule_action", "rule {n}: invalid action", "n", n))
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
		return "", 0, errors.New("invalid port")
	}
	if !hostRe.MatchString(strings.Trim(h, "[]")) && net.ParseIP(h) == nil {
		return "", 0, errors.New("invalid host")
	}
	return h, port, nil
}
