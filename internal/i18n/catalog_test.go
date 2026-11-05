package i18n_test

// Keeps the backend messages and the UI catalogs (web/src/i18n/locales) in
// sync:
//   - every message code emitted by Go code exists in en.json under
//     "server.<code>" with exactly the Go English template;
//   - en, pt-BR and es have the same keys and the same {placeholders}.

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const localesDir = "../../web/src/i18n/locales"

var (
	codeRe        = regexp.MustCompile(`^[a-z][a-z_]*(\.[a-z0-9_]+)+$`)
	placeholderRe = regexp.MustCompile(`\{(\w+)\}`)
	// Calls whose arguments include a (code, English template) pair.
	messageFuncs = map[string]bool{
		"M": true, "E": true, "Wrap": true, // i18n package
		"validationErr": true, "writeError": true, // auth, server
		"info": true, "warn": true, "logf": true, // ACME and Kong logs
	}
)

// goMessages returns code -> English template for every message in the
// non-test Go sources under internal/.
func goMessages(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	fset := token.NewFileSet()
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			var name string
			switch fn := call.Fun.(type) {
			case *ast.Ident:
				name = fn.Name
			case *ast.SelectorExpr:
				name = fn.Sel.Name
			}
			if !messageFuncs[name] {
				return true
			}
			for i := 0; i+1 < len(call.Args); i++ {
				code, ok1 := stringLit(call.Args[i])
				tmpl, ok2 := stringLit(call.Args[i+1])
				if ok1 && ok2 && codeRe.MatchString(code) {
					if prev, dup := out[code]; dup && prev != tmpl {
						t.Errorf("%s: code %q has two templates: %q and %q", fset.Position(call.Pos()), code, prev, tmpl)
					}
					out[code] = tmpl
					break
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func stringLit(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	return s, err == nil
}

// flatten turns a nested catalog into dotted keys.
func flatten(prefix string, v any, out map[string]string) {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			flatten(key, child, out)
		}
	case string:
		out[prefix] = x
	}
}

func loadCatalog(t *testing.T, lang string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(localesDir, lang+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("%s.json: %v", lang, err)
	}
	out := map[string]string{}
	flatten("", raw, out)
	return out
}

func placeholders(s string) []string {
	var out []string
	for _, m := range placeholderRe.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func TestServerCodesAreInCatalog(t *testing.T) {
	en := loadCatalog(t, "en")
	msgs := goMessages(t)
	if len(msgs) < 100 {
		t.Fatalf("found only %d message codes; is the extraction broken?", len(msgs))
	}
	for code, tmpl := range msgs {
		got, ok := en["server."+code]
		switch {
		case !ok:
			t.Errorf("code %q is missing from en.json (server.%s = %q)", code, code, tmpl)
		case got != tmpl:
			t.Errorf("server.%s in en.json is %q, Go template is %q", code, got, tmpl)
		}
	}
	for key := range en {
		if code, isServer := strings.CutPrefix(key, "server."); isServer && msgs[code] == "" {
			t.Errorf("en.json has %s but no Go code emits it", key)
		}
	}
}

func TestCatalogsMatch(t *testing.T) {
	en := loadCatalog(t, "en")
	for _, lang := range []string{"pt-BR", "es"} {
		other := loadCatalog(t, lang)
		for key, text := range en {
			tr, ok := other[key]
			if !ok {
				t.Errorf("%s: missing key %s", lang, key)
				continue
			}
			if strings.TrimSpace(tr) == "" {
				t.Errorf("%s: empty translation for %s", lang, key)
			}
			if !slices.Equal(placeholders(text), placeholders(tr)) {
				t.Errorf("%s: %s has placeholders %v, en has %v", lang, key, placeholders(tr), placeholders(text))
			}
		}
		for key := range other {
			if _, ok := en[key]; !ok {
				t.Errorf("%s: extra key %s not in en.json", lang, key)
			}
		}
	}
}
