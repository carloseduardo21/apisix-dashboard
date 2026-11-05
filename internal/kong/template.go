package kong

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/carloseduardo21/apisix-dashboard/internal/i18n"
)

// Templates of PluginRule. Available roots:
//
//	config    the Kong plugin config
//	plugin    the Kong plugin (name, id, enabled, protocols, tags)
//	service   the Kong service the plugin applies to, if any
//	route     the Kong route the plugin applies to, if any
//	consumer  the Kong consumer the plugin applies to, if any
//	apisix    ids of the APISIX objects being created (service_id, route_id, upstream_id, consumer)
//
// "${config}" copies the whole Kong config.

var placeholderRe = regexp.MustCompile(`\$\{([^}]*)\}`)

// missing marks a value whose placeholder did not resolve; it is dropped
// from the rendered object.
type missing struct{}

func isMissing(v any) bool {
	_, ok := v.(missing)
	return ok
}

func checkTemplate(tpl map[string]any) error {
	var err error
	walkStrings(tpl, func(s string) {
		for _, m := range placeholderRe.FindAllStringSubmatch(s, -1) {
			path, _, _ := strings.Cut(m[1], "|")
			if strings.TrimSpace(path) == "" && err == nil {
				err = i18n.E("kong.settings.empty_placeholder", "empty placeholder in '{value}'", "value", s)
			}
		}
	})
	return err
}

func walkStrings(v any, fn func(string)) {
	switch t := v.(type) {
	case string:
		fn(t)
	case map[string]any:
		for _, x := range t {
			walkStrings(x, fn)
		}
	case []any:
		for _, x := range t {
			walkStrings(x, fn)
		}
	}
}

// render evaluates a template against ctx.
func render(tpl any, ctx map[string]any) any {
	switch t := tpl.(type) {
	case string:
		if m := placeholderRe.FindStringSubmatch(t); m != nil && m[0] == t {
			// The whole string is one placeholder: keep the value's type.
			return resolve(m[1], ctx)
		}
		return placeholderRe.ReplaceAllStringFunc(t, func(s string) string {
			v := resolve(s[2:len(s)-1], ctx)
			switch x := v.(type) {
			case missing:
				return ""
			case string:
				return x
			default:
				b, _ := json.Marshal(x)
				return string(b)
			}
		})
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, v := range t {
			if r := render(v, ctx); !isMissing(r) {
				out[k] = r
			}
		}
		return out
	case []any:
		out := make([]any, 0, len(t))
		for _, v := range t {
			if r := render(v, ctx); !isMissing(r) {
				out = append(out, r)
			}
		}
		return out
	default:
		return tpl
	}
}

// resolve looks up "a.b.0.c|default". A default is parsed as JSON when
// possible (numbers, booleans, objects) and used as a string otherwise.
func resolve(expr string, ctx map[string]any) any {
	path, def, hasDef := strings.Cut(expr, "|")
	var cur any = ctx
	for _, seg := range strings.Split(strings.TrimSpace(path), ".") {
		switch c := cur.(type) {
		case map[string]any:
			cur = c[seg]
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(c) {
				cur = nil
			} else {
				cur = c[i]
			}
		default:
			cur = nil
		}
		if cur == nil {
			break
		}
	}
	if cur != nil {
		return cur
	}
	if !hasDef {
		return missing{}
	}
	def = strings.TrimSpace(def)
	var v any
	if err := json.Unmarshal([]byte(def), &v); err == nil {
		return v
	}
	return def
}

// toMap converts a struct to the generic form templates navigate.
func toMap(v any) map[string]any {
	b, _ := json.Marshal(v)
	var m map[string]any
	_ = json.Unmarshal(b, &m) // round-trip of a value we just marshaled
	return m
}
