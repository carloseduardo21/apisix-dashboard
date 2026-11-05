// Package i18n carries user-facing messages as stable codes plus parameters.
//
// The backend never translates: it sends the code and parameters, and the
// frontend renders them in the user's language (web/src/i18n). Every message
// also has an English rendering, used as the "error" text of API responses,
// in logs, audit details and the CLI, and as the UI fallback.
//
// Codes are part of the API contract: rename them only together with the
// frontend catalogs (checked by `npm run i18n:check`).
package i18n

import (
	"fmt"
	"strings"
)

// Message is a user-facing message.
type Message struct {
	Code   string            `json:"code"`
	Params map[string]string `json:"params,omitempty"`
	text   string
}

// M builds a message. template is English and may contain "{name}"
// placeholders, filled from kv given as name/value pairs:
//
//	i18n.M("auth.password_too_short", "password must be at least {min} characters", "min", 12)
func M(code, template string, kv ...any) Message {
	m := Message{Code: code, text: template}
	if len(kv) == 0 {
		return m
	}
	m.Params = make(map[string]string, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		name := fmt.Sprint(kv[i])
		value := fmt.Sprint(kv[i+1])
		m.Params[name] = value
		m.text = strings.ReplaceAll(m.text, "{"+name+"}", value)
	}
	return m
}

// String is the English rendering.
func (m Message) String() string { return m.text }

// IsZero reports whether the message is empty.
func (m Message) IsZero() bool { return m.Code == "" }

// Error is an error whose message is safe to show to users.
type Error struct {
	Msg Message
	Err error // optional cause, never shown to users
}

// E builds an *Error (see M for the arguments).
func E(code, template string, kv ...any) *Error {
	return &Error{Msg: M(code, template, kv...)}
}

// Wrap attaches a user-facing message to an internal cause.
func Wrap(err error, code, template string, kv ...any) *Error {
	return &Error{Msg: M(code, template, kv...), Err: err}
}

func (e *Error) Error() string        { return e.Msg.text }
func (e *Error) Unwrap() error        { return e.Err }
func (e *Error) UserMessage() Message { return e.Msg }

// Is matches another *Error with the same code, so sentinels still match
// after WithCause.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Msg.Code == e.Msg.Code
}
func (e *Error) WithCause(err error) *Error { return &Error{Msg: e.Msg, Err: err} }

// UserMessager is implemented by errors that carry a user-facing message.
type UserMessager interface{ UserMessage() Message }

// Messages returns the user-facing messages carried by err, including every
// branch of an errors.Join. ok is false when err carries none.
func Messages(err error) (msgs []Message, ok bool) {
	if err == nil {
		return nil, false
	}
	if um, isUM := err.(UserMessager); isUM {
		return []Message{um.UserMessage()}, true
	}
	switch u := err.(type) { //nolint:errorlint // walks the wrapper chain itself to collect every errors.Join branch
	case interface{ Unwrap() []error }:
		for _, e := range u.Unwrap() {
			if m, found := Messages(e); found {
				msgs = append(msgs, m...)
			}
		}
		return msgs, len(msgs) > 0
	case interface{ Unwrap() error }:
		return Messages(u.Unwrap())
	}
	return nil, false
}
