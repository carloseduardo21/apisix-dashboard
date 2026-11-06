package server

import (
	"bytes"
	"encoding/json"

	"github.com/carloseduardo21/apisix-dashboard/internal/apisix"
	"github.com/carloseduardo21/apisix-dashboard/internal/auth"
	"github.com/carloseduardo21/apisix-dashboard/internal/i18n"
	"github.com/carloseduardo21/apisix-dashboard/internal/kong"
)

// Restricted plugins run arbitrary code inside the APISIX workers (or hand
// requests to external code), which can read the admin key, the data
// encryption keyring and every secret. Only admins may add or change them;
// operators may still edit other fields of objects that use them, and remove
// them. The route "script" field (Lua) is restricted the same way.
// See DASH_RESTRICTED_PLUGINS.

// restrictedChange returns a user-facing error when a non-admin write adds or
// changes a restricted plugin or a route script. body is the restored request
// body and before the current object (nil when creating).
func (s *Server) restrictedChange(role string, path apisix.Path, body, before any) error {
	if auth.IsAdmin(role) {
		return nil
	}
	if path.Resource == "plugin_metadata" && s.RestrictedPlugins[path.ID()] {
		return i18n.E("apisix.plugin_restricted", "only administrators can add or change the {plugin} plugin", "plugin", path.ID())
	}
	next, _ := body.(map[string]any)
	prev, _ := before.(map[string]any)
	nextPlugins, _ := next["plugins"].(map[string]any)
	prevPlugins, _ := prev["plugins"].(map[string]any)
	for name, conf := range nextPlugins {
		if s.RestrictedPlugins[name] && conf != nil && !sameJSON(conf, prevPlugins[name]) {
			return i18n.E("apisix.plugin_restricted", "only administrators can add or change the {plugin} plugin", "plugin", name)
		}
	}
	if path.Resource == "routes" {
		if script, ok := next["script"]; ok && script != nil && script != "" && !sameJSON(script, prev["script"]) {
			return i18n.E("apisix.script_restricted", "only administrators can add or change route scripts")
		}
	}
	return nil
}

// restrictedKongRules rejects, for non-admins, Kong plugin rules that map to
// a restricted APISIX plugin and are new or changed compared to saved ones.
func (s *Server) restrictedKongRules(role string, next, saved []kong.PluginRule) error {
	if auth.IsAdmin(role) {
		return nil
	}
	for _, r := range next {
		if r.Action != kong.RuleMap || !s.RestrictedPlugins[r.APISIXPlugin] {
			continue
		}
		approved := false
		for _, old := range saved {
			if sameJSON(r, old) {
				approved = true
				break
			}
		}
		if !approved {
			return i18n.E("kong.settings.plugin_restricted", "only administrators can map Kong plugins to {plugin}", "plugin", r.APISIXPlugin)
		}
	}
	return nil
}

// sameJSON compares two values by their JSON encoding (map keys are sorted,
// numbers keep their exact text through apisix.Decode).
func sameJSON(a, b any) bool {
	ja, errA := json.Marshal(a)
	jb, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(ja, jb)
}
