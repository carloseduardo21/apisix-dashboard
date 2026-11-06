package apisix

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/carloseduardo21/apisix-dashboard/internal/i18n"
)

// Masked replaces secret values in everything sent to the browser and in the
// audit trail (PCI DSS 3 / 8.3.2). When an object containing Masked is
// written back, the original value is restored from APISIX (see Unmask).
const Masked = "******"

// Field names that hold secrets anywhere in an object, e.g. basic-auth
// password, jwt-auth secret, openid-connect client_secret, vault token.
var sensitiveKeys = map[string]bool{
	"password":               true,
	"redis_password":         true,
	"secret":                 true,
	"secret_key":             true,
	"private_key":            true,
	"client_key":             true, // upstream mTLS private key
	"client_secret":          true,
	"client_rsa_private_key": true,
	"secret_access_key":      true,
	"session_token":          true,
	"token":                  true,
	"api_key":                true,
	"access_token":           true,
	"apikey":                 true, // aws-lambda, azure-functions authorization
	"secretkey":              true, // aws-lambda authorization.iam
	"auth_header":            true, // http-logger
}

// Maps whose entries are HTTP headers (or query parameters) set by a plugin,
// e.g. proxy-rewrite headers.set, ai-proxy auth.header. Entries whose names
// look like credentials are masked.
var headerMaps = map[string]bool{
	"headers":         true,
	"header":          true,
	"set":             true,
	"add":             true,
	"request_headers": true,
	"extra_headers":   true,
	"query":           true,
}

// credentialName reports whether a header or parameter name usually carries
// a credential (Authorization, Cookie, X-API-Key, Private-Token...).
func credentialName(name string) bool {
	n := strings.ToLower(name)
	for _, s := range []string{"authorization", "cookie", "api-key", "apikey", "api_key", "token", "secret", "password", "credential", "x-functions-key"} {
		if strings.Contains(n, s) {
			return true
		}
	}
	return false
}

// Whole objects that hold credentials, by plugin: every string inside is
// masked. ai-proxy declares no encrypt_fields but its auth block carries the
// LLM provider API keys (header and query maps).
var secretBlocks = map[string]map[string]bool{
	"ai-proxy": {"auth": true},
}

func isSensitive(key, parent string, topSSL bool) bool {
	switch {
	case sensitiveKeys[key]:
		return true
	case topSSL && (key == "key" || key == "keys"):
		return true // SSL private keys
	case parent == "key-auth" && key == "key":
		return true
	case headerMaps[parent] && credentialName(key):
		return true
	}
	return false
}

// looksLikeSSL reports whether v has the shape of an SSL object (a
// certificate with its private key), so its keys are masked even when it is
// returned for another resource. Defense in depth: paths are validated, but a
// private key must never depend on that alone.
func looksLikeSSL(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	_, cert := m["cert"]
	_, key := m["key"]
	_, keys := m["keys"]
	return cert && (key || keys)
}

// References to APISIX secret managers are not secrets themselves.
func isReference(s string) bool {
	return strings.HasPrefix(s, "$secret://") || strings.HasPrefix(s, "$env://")
}

// EncryptFieldsFunc returns the sensitive field paths of a plugin (see
// Catalog.EncryptFields). It may be nil.
type EncryptFieldsFunc func(plugin string) [][]string

// Mask returns a copy of an object of the given resource with secrets masked:
// fields matched by the static rules above plus, for each plugin in
// "plugins", the encrypt_fields declared by APISIX.
func Mask(resource string, v any, enc EncryptFieldsFunc) any {
	out := maskValue(v, resource == "ssls" || looksLikeSSL(v), "")
	if enc == nil {
		return out
	}
	if m, ok := out.(map[string]any); ok {
		// A plugin_metadata object is the metadata of the plugin named by its id.
		if id, ok := m["id"].(string); ok && resource == "plugin_metadata" {
			for _, path := range enc(id) {
				maskPath(m, path)
			}
		}
		if plugins, ok := m["plugins"].(map[string]any); ok {
			for name, conf := range plugins {
				for _, path := range enc(name) {
					maskPath(conf, path)
				}
			}
		}
	}
	return out
}

// maskPath masks the value at a dotted path; arrays along the way are
// traversed element by element ("instances.auth.header").
func maskPath(v any, path []string) {
	switch t := v.(type) {
	case map[string]any:
		val, ok := t[path[0]]
		if !ok {
			return
		}
		if len(path) == 1 {
			t[path[0]] = maskAll(val)
		} else {
			maskPath(val, path[1:])
		}
	case []any:
		for _, item := range t {
			maskPath(item, path)
		}
	}
}

// maskAll masks every string inside v (used when a whole object, such as a
// headers map, is declared sensitive).
func maskAll(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			t[k] = maskAll(val)
		}
		return t
	case []any:
		for i, val := range t {
			t[i] = maskAll(val)
		}
		return t
	}
	return maskLeaf(v)
}

func maskValue(v any, topSSL bool, parent string) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			switch {
			case secretBlocks[parent][k]:
				out[k] = maskAll(maskValue(val, false, k)) // masks the copy, not the input
			case isSensitive(k, parent, topSSL):
				out[k] = maskLeaf(val)
			default:
				out[k] = maskValue(val, false, k)
			}
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = maskValue(val, false, parent)
		}
		return out
	}
	return v
}

func maskLeaf(v any) any {
	switch t := v.(type) {
	case string:
		if t == "" || isReference(t) {
			return t
		}
		return Masked
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = maskLeaf(val)
		}
		return out
	case map[string]any:
		return maskValue(t, false, "")
	}
	return v
}

// Unmask replaces Masked values in next with the value at the same path in
// prev (the current object in APISIX). next is modified in place.
func Unmask(next, prev any) (any, error) {
	return unmask(next, prev, "")
}

func unmask(next, prev any, path string) (any, error) {
	switch t := next.(type) {
	case string:
		if t != Masked {
			return t, nil
		}
		if s, ok := prev.(string); ok {
			return s, nil
		}
		return nil, i18n.E("apisix.masked_without_original", "field {field} contains a masked value with no original value; enter the real value", "field", strings.TrimPrefix(path, "."))
	case map[string]any:
		pm, _ := prev.(map[string]any)
		for k, val := range t {
			r, err := unmask(val, pm[k], path+"."+k)
			if err != nil {
				return nil, err
			}
			t[k] = r
		}
		return t, nil
	case []any:
		pa, _ := prev.([]any)
		for i, val := range t {
			var p any
			if i < len(pa) {
				p = pa[i]
			}
			r, err := unmask(val, p, fmt.Sprintf("%s[%d]", path, i))
			if err != nil {
				return nil, err
			}
			t[i] = r
		}
		return t, nil
	}
	return next, nil
}

// Decode parses JSON keeping numbers exact.
func Decode(b []byte) (any, error) {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// MaskEnvelope masks an Admin API response: either {"value": ...} or
// {"list": [{"value": ...}, ...]}.
func MaskEnvelope(resource string, env any, enc EncryptFieldsFunc) any {
	m, ok := env.(map[string]any)
	if !ok {
		return env
	}
	if v, ok := m["value"]; ok {
		m["value"] = Mask(resource, v, enc)
	}
	if list, ok := m["list"].([]any); ok {
		for _, item := range list {
			if im, ok := item.(map[string]any); ok {
				if v, ok := im["value"]; ok {
					im["value"] = Mask(resource, v, enc)
				}
			}
		}
	}
	return m
}

// Fields that decide where a plugin, secret manager or plugin metadata sends
// data (and so the secrets configured next to them).
var destinationKeys = map[string]bool{
	"uri":                    true,
	"url":                    true,
	"base_url":               true,
	"endpoint":               true,
	"endpoints":              true,
	"endpoint_addr":          true,
	"endpoint_addrs":         true,
	"discovery":              true,
	"function_uri":           true,
	"token_endpoint":         true,
	"introspection_endpoint": true,
	"ldap_uri":               true,
	"brokers":                true,
	"broker_list":            true,
	"redis_host":             true,
	"redis_cluster_nodes":    true,
	"host":                   true,
	"server":                 true,
	"servers":                true,
	"address":                true,
	"addr":                   true,
}

// CheckMaskedDestinations rejects a write that keeps a masked secret while
// changing where that secret is sent: otherwise someone who cannot see a
// secret could point its plugin at a host they control and receive it. The
// scope is each plugin configuration, and the whole object for secret
// managers and plugin metadata.
func CheckMaskedDestinations(resource string, next, prev any) error {
	nm, _ := next.(map[string]any)
	pm, _ := prev.(map[string]any)
	if nm == nil || pm == nil {
		return nil // nothing to restore from: Unmask reports masked values
	}
	if resource == "secrets" || resource == "plugin_metadata" {
		if err := checkScope(nm, pm, resource); err != nil {
			return err
		}
	}
	np, _ := nm["plugins"].(map[string]any)
	pp, _ := pm["plugins"].(map[string]any)
	for name, conf := range np {
		if err := checkScope(conf, pp[name], "plugins."+name); err != nil {
			return err
		}
	}
	return nil
}

func checkScope(next, prev any, scope string) error {
	if !containsMasked(next) {
		return nil
	}
	nd, pd := map[string]string{}, map[string]string{}
	collectDestinations(next, "", nd)
	collectDestinations(prev, "", pd)
	if len(nd) != len(pd) {
		return destinationChanged(scope)
	}
	for k, v := range nd {
		if pd[k] != v {
			return destinationChanged(scope)
		}
	}
	return nil
}

func destinationChanged(scope string) error {
	return i18n.E("apisix.masked_secret_destination_changed",
		"{scope}: the destination changed, so masked secrets cannot be kept; enter them again", "scope", scope)
}

func containsMasked(v any) bool {
	switch t := v.(type) {
	case string:
		return t == Masked
	case map[string]any:
		for _, x := range t {
			if containsMasked(x) {
				return true
			}
		}
	case []any:
		for _, x := range t {
			if containsMasked(x) {
				return true
			}
		}
	}
	return false
}

// collectDestinations records the JSON value of every destination field by
// its path.
func collectDestinations(v any, path string, out map[string]string) {
	switch t := v.(type) {
	case map[string]any:
		for k, x := range t {
			p := path + "." + k
			if destinationKeys[k] {
				b, _ := json.Marshal(x)
				out[p] = string(b)
				continue
			}
			collectDestinations(x, p, out)
		}
	case []any:
		for i, x := range t {
			collectDestinations(x, fmt.Sprintf("%s[%d]", path, i), out)
		}
	}
}
