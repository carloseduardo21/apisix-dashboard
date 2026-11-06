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

// Credentials found in APISIX 3.14 plugin schemas that are not declared in
// encrypt_fields (see docs/security-review.md, M1).
func TestMaskUndeclaredCredentials(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, resource, body string
		secrets              []string // must not appear in the output
		kept                 []string // must still appear
	}{
		{
			name: "aws-lambda", resource: "routes",
			body:    `{"plugins":{"aws-lambda":{"function_uri":"https://lambda.example.com","authorization":{"apikey":"AK1","iam":{"accesskey":"AKIAEXAMPLE","secretkey":"SK1","aws_region":"us-east-1"}}}}}`,
			secrets: []string{"AK1", "SK1"}, kept: []string{"https://lambda.example.com", "AKIAEXAMPLE", "us-east-1"},
		},
		{
			name: "azure-functions", resource: "routes",
			body:    `{"plugins":{"azure-functions":{"function_uri":"https://fn.example.com","authorization":{"apikey":"AZ1","clientid":"client-1"}}}}`,
			secrets: []string{"AZ1"}, kept: []string{"https://fn.example.com", "client-1"},
		},
		{
			name: "http-logger", resource: "global_rules",
			body:    `{"plugins":{"http-logger":{"uri":"https://logs.example.com","auth_header":"Bearer HL1"}}}`,
			secrets: []string{"HL1"}, kept: []string{"https://logs.example.com"},
		},
		{
			name: "ai-proxy", resource: "routes",
			body:    `{"plugins":{"ai-proxy":{"provider":"openai","auth":{"header":{"Authorization":"Bearer sk-AI1"},"query":{"key":"AI2"}},"options":{"model":"m"}}}}`,
			secrets: []string{"sk-AI1", "AI2"}, kept: []string{"openai", `"model":"m"`},
		},
		{
			name: "proxy-rewrite headers", resource: "services",
			body:    `{"plugins":{"proxy-rewrite":{"uri":"/x","headers":{"set":{"Authorization":"Basic PR1","X-Api-Key":"PR2","X-Request-Source":"lintel"},"add":{"Private-Token":"PR3"}}}}}`,
			secrets: []string{"PR1", "PR2", "PR3"}, kept: []string{`"/x"`, "lintel", "X-Request-Source"},
		},
		{
			name: "forward-auth request headers are names only", resource: "routes",
			body: `{"plugins":{"forward-auth":{"uri":"https://auth.example.com","request_headers":["Authorization"]}}}`,
			kept: []string{"https://auth.example.com", "Authorization"},
		},
		{
			name: "key-auth header option is a name", resource: "consumers",
			body:    `{"plugins":{"key-auth":{"key":"KA1","header":"apikey","query":"apikey"}}}`,
			secrets: []string{"KA1"}, kept: []string{`"header":"apikey"`, `"query":"apikey"`},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			in := decode(t, tc.body)
			b, _ := json.Marshal(Mask(tc.resource, in, nil))
			s := string(b)
			for _, secret := range tc.secrets {
				if strings.Contains(s, secret) {
					t.Errorf("%q leaked: %s", secret, s)
				}
			}
			for _, k := range tc.kept {
				if !strings.Contains(s, k) {
					t.Errorf("%q lost: %s", k, s)
				}
			}
			// Masking works on a copy.
			orig, _ := json.Marshal(in)
			if !strings.Contains(string(orig), strings.Trim(firstOr(tc.secrets, ""), `"`)) {
				t.Errorf("Mask modified its input: %s", orig)
			}
		})
	}
}

func firstOr(s []string, def string) string {
	if len(s) == 0 {
		return def
	}
	return s[0]
}

func TestMaskPluginMetadata(t *testing.T) {
	t.Parallel()
	enc := func(plugin string) [][]string {
		if plugin == "example-logger" {
			return [][]string{{"auth", "pass"}} // not a static key: only encrypt_fields can mask it
		}
		return nil
	}
	v := decode(t, `{"id":"example-logger","host":"logs.example.com","auth":{"user":"u","pass":"PM1"}}`)
	b, _ := json.Marshal(Mask("plugin_metadata", v, enc))
	if strings.Contains(string(b), "PM1") || !strings.Contains(string(b), "logs.example.com") {
		t.Errorf("unexpected plugin_metadata mask: %s", b)
	}
}

// A masked credential is restored on save, like any other masked value.
func TestUnmaskHeaderCredential(t *testing.T) {
	t.Parallel()
	orig := decode(t, `{"plugins":{"proxy-rewrite":{"headers":{"set":{"Authorization":"Basic PR1"}}}}}`)
	b, _ := json.Marshal(Mask("routes", orig, nil))
	got, err := Unmask(decode(t, string(b)), orig)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, orig) {
		t.Errorf("got %v, want %v", got, orig)
	}
}
