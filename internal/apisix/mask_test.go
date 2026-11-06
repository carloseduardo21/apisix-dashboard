package apisix

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func decode(t *testing.T, s string) any {
	t.Helper()
	v, err := Decode([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestMaskConsumer(t *testing.T) {
	v := decode(t, `{"username":"jack","plugins":{"key-auth":{"key":"k1","header":"apikey"},
		"basic-auth":{"username":"jack","password":"p"},"jwt-auth":{"key":"user-key","secret":"$secret://vault/1/jwt"}}}`)
	got := Mask("consumers", v, nil).(map[string]any)
	pl := got["plugins"].(map[string]any)
	if pl["key-auth"].(map[string]any)["key"] != Masked {
		t.Error("key-auth key not masked")
	}
	if pl["key-auth"].(map[string]any)["header"] != "apikey" {
		t.Error("key-auth header must not be masked")
	}
	if pl["basic-auth"].(map[string]any)["password"] != Masked {
		t.Error("basic-auth password not masked")
	}
	if pl["jwt-auth"].(map[string]any)["key"] != "user-key" {
		t.Error("jwt-auth key is an identifier, not a secret")
	}
	if pl["jwt-auth"].(map[string]any)["secret"] != "$secret://vault/1/jwt" {
		t.Error("secret references must be kept")
	}
	// The original must be untouched.
	if v.(map[string]any)["plugins"].(map[string]any)["key-auth"].(map[string]any)["key"] != "k1" {
		t.Error("Mask modified its input")
	}
}

func TestMaskSSLAndUpstreamKey(t *testing.T) {
	ssl := Mask("ssls", decode(t, `{"cert":"C","key":"K","keys":["K2"],"snis":["a"]}`), nil).(map[string]any)
	if ssl["key"] != Masked || ssl["keys"].([]any)[0] != Masked || ssl["cert"] != "C" {
		t.Errorf("unexpected ssl mask: %v", ssl)
	}
	up := Mask("upstreams", decode(t, `{"type":"chash","key":"remote_addr","tls":{"client_cert":"C","client_key":"K"}}`), nil).(map[string]any)
	if up["key"] != "remote_addr" {
		t.Error("upstream chash key must not be masked")
	}
	if tls := up["tls"].(map[string]any); tls["client_key"] != Masked || tls["client_cert"] != "C" {
		t.Errorf("upstream mTLS key not masked: %v", tls)
	}
}

func TestUnmaskRoundTrip(t *testing.T) {
	orig := decode(t, `{"username":"jack","plugins":{"key-auth":{"key":"k1"}},"desc":"a"}`)
	masked := Mask("consumers", orig, nil)
	b, _ := json.Marshal(masked)
	edited := decode(t, string(b))
	edited.(map[string]any)["desc"] = "b"

	got, err := Unmask(edited, orig)
	if err != nil {
		t.Fatal(err)
	}
	want := decode(t, `{"username":"jack","plugins":{"key-auth":{"key":"k1"}},"desc":"b"}`)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestUnmaskWithoutOriginal(t *testing.T) {
	_, err := Unmask(decode(t, `{"plugins":{"key-auth":{"key":"******"}}}`), nil)
	if err == nil {
		t.Fatal("expected error for masked value without original")
	}
}

func TestParsePath(t *testing.T) {
	ok := []string{"routes", "routes/1", "secrets/vault/1", "consumers/jack", "consumers/jack/credentials", "consumers/jack/credentials/c1"}
	for _, p := range ok {
		if _, valid := ParsePath(p); !valid {
			t.Errorf("%q should be valid", p)
		}
	}
	bad := []string{
		"", "plugins/list", "routes/1/2", "routes/../ssls", "routes/a%2Fb", "consumers/jack/x", "unknown/1", "routes/a b",
		// Dot segments (reach the handler percent-encoded) would escape the resource.
		"routes/..", "routes/.", "secrets/../ssls", "secrets/./1", "secrets/vault/..",
		"consumers/../credentials/x", "consumers/jack/credentials/..", "consumers/./credentials/c1",
	}
	for _, p := range bad {
		if _, valid := ParsePath(p); valid {
			t.Errorf("%q should be rejected", p)
		}
	}
	p, _ := ParsePath("consumers/jack/credentials/c1")
	if !p.IsItem() || p.ID() != "jack/credentials/c1" {
		t.Errorf("unexpected %+v", p)
	}
}

func TestMaskEncryptFields(t *testing.T) {
	enc := func(plugin string) [][]string {
		switch plugin {
		case "ai-proxy-multi":
			return [][]string{{"instances", "auth", "header"}}
		case "loki-logger":
			return [][]string{{"headers"}}
		}
		return nil
	}
	v := decode(t, `{"plugins":{
		"ai-proxy-multi":{"instances":[{"name":"a","auth":{"header":{"Authorization":"Bearer x"}}},{"name":"b","auth":{"header":{"X-Key":"y"}}}]},
		"loki-logger":{"endpoint_addrs":["http://loki"],"headers":{"X-Scope":"t1"}}}}`)
	got := Mask("routes", v, enc)
	b, _ := json.Marshal(got)
	s := string(b)
	for _, secret := range []string{"Bearer x", `"y"`, "t1"} {
		if strings.Contains(s, secret) {
			t.Errorf("%s leaked: %s", secret, s)
		}
	}
	if !strings.Contains(s, "http://loki") || !strings.Contains(s, `"name":"b"`) {
		t.Errorf("non-sensitive data lost: %s", s)
	}
}

func TestMaskSSLShapeForAnyResource(t *testing.T) {
	t.Parallel()
	pem := `-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----` // JSON-escaped newlines
	tests := []struct {
		name, resource, body string
		masked               []string // top-level fields that must be masked
		kept                 []string // top-level fields that must be kept
	}{
		{"ssl resource", "ssls", `{"cert":"C","key":"` + pem + `","keys":["` + pem + `"]}`, []string{"key", "keys"}, []string{"cert"}},
		{"ssl shape under another resource", "secrets", `{"cert":"C","key":"` + pem + `"}`, []string{"key"}, []string{"cert"}},
		{"ssl shape with keys only", "routes", `{"cert":"C","keys":["` + pem + `"]}`, []string{"keys"}, []string{"cert"}},
		{"plain key field is not an ssl", "secrets", `{"key":"vault-path","uri":"http://vault"}`, nil, []string{"key", "uri"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, ok := Mask(tc.resource, decode(t, tc.body), nil).(map[string]any)
			if !ok {
				t.Fatal("expected an object")
			}
			for _, f := range tc.masked {
				if s := fmt.Sprint(out[f]); strings.Contains(s, "PRIVATE KEY") {
					t.Errorf("%s: %s not masked: %v", tc.name, f, out[f])
				}
			}
			for _, f := range tc.kept {
				if s := fmt.Sprint(out[f]); strings.Contains(s, Masked) {
					t.Errorf("%s: %s should not be masked: %v", tc.name, f, out[f])
				}
			}
		})
	}
}
