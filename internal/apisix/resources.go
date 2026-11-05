package apisix

import (
	"net/url"
	"regexp"
	"strings"
)

// Resources exposed through the dashboard. Anything else in the Admin API is
// unreachable from the browser.
var resources = map[string]bool{
	"routes":          true,
	"services":        true,
	"upstreams":       true,
	"consumers":       true,
	"consumer_groups": true,
	"ssls":            true,
	"global_rules":    true,
	"plugin_configs":  true,
	"plugin_metadata": true,
	"stream_routes":   true,
	"protos":          true,
	"secrets":         true,
}

var segmentRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,256}$`)

// Path is a validated Admin API resource path.
type Path struct {
	Resource string   // e.g. "routes"
	Segments []string // everything after the resource
}

// ParsePath validates "routes/1", "secrets/vault/1",
// "consumers/jack/credentials/key-1", etc.
func ParsePath(raw string) (Path, bool) {
	segs := strings.Split(strings.Trim(raw, "/"), "/")
	if !resources[segs[0]] {
		return Path{}, false
	}
	rest := segs[1:]
	for _, s := range rest {
		if !segmentRe.MatchString(s) {
			return Path{}, false
		}
	}
	switch {
	case len(rest) <= 1:
	case segs[0] == "secrets" && len(rest) == 2:
	case segs[0] == "consumers" && (len(rest) == 2 || len(rest) == 3) && rest[1] == "credentials":
	default:
		return Path{}, false
	}
	return Path{Resource: segs[0], Segments: rest}, true
}

// ID is the resource id as shown in the audit trail ("" for collections).
func (p Path) ID() string { return strings.Join(p.Segments, "/") }

// IsItem reports whether the path addresses a single object.
func (p Path) IsItem() bool {
	switch {
	case p.Resource == "secrets":
		return len(p.Segments) == 2
	case p.Resource == "consumers" && len(p.Segments) >= 2:
		return len(p.Segments) == 3
	default:
		return len(p.Segments) == 1
	}
}

func (p Path) String() string {
	parts := []string{p.Resource}
	for _, s := range p.Segments {
		parts = append(parts, url.PathEscape(s))
	}
	return strings.Join(parts, "/")
}

// Child returns the path of an item inside this collection.
func (p Path) Child(id string) (Path, bool) {
	return ParsePath(p.String() + "/" + id)
}

var pluginNameRe = regexp.MustCompile(`^[a-z0-9_-]{1,128}$`)

// ValidPluginName reports whether name may be used in plugins/<name>.
func ValidPluginName(name string) bool { return pluginNameRe.MatchString(name) }
