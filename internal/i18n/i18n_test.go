package i18n

import (
	"errors"
	"fmt"
	"testing"
)

func TestM(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		msg    Message
		text   string
		params map[string]string
	}{
		{"no params", M("a.b", "plain"), "plain", nil},
		{"params", M("a.b", "at least {min} of {max}", "min", 12, "max", 256), "at least 12 of 256", map[string]string{"min": "12", "max": "256"}},
		{"repeated placeholder", M("a.b", "{x}-{x}", "x", "y"), "y-y", map[string]string{"x": "y"}},
		{"odd kv ignored", M("a.b", "{x}", "x"), "{x}", map[string]string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.msg.String(); got != tt.text {
				t.Errorf("String() = %q, want %q", got, tt.text)
			}
			if fmt.Sprint(tt.msg.Params) != fmt.Sprint(tt.params) {
				t.Errorf("Params = %v, want %v", tt.msg.Params, tt.params)
			}
		})
	}
}

func TestErrorsIsAndMessages(t *testing.T) {
	t.Parallel()
	sentinel := E("x.sentinel", "sentinel")
	cause := errors.New("internal detail")
	wrapped := fmt.Errorf("context: %w", sentinel.WithCause(cause))

	if !errors.Is(wrapped, sentinel) {
		t.Fatal("errors.Is should match the sentinel through WithCause and %w")
	}
	if !errors.Is(wrapped, cause) {
		t.Fatal("errors.Is should reach the internal cause")
	}
	msgs, ok := Messages(wrapped)
	if !ok || len(msgs) != 1 || msgs[0].Code != "x.sentinel" {
		t.Fatalf("Messages = %v, %v", msgs, ok)
	}

	joined := errors.Join(E("x.a", "a"), errors.New("no message"), E("x.b", "b {n}", "n", 2))
	msgs, ok = Messages(joined)
	if !ok || len(msgs) != 2 || msgs[1].String() != "b 2" {
		t.Fatalf("Messages(join) = %v, %v", msgs, ok)
	}
	if _, ok := Messages(errors.New("plain")); ok {
		t.Fatal("a plain error carries no user message")
	}
}
