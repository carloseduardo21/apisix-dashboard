package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/carloseduardo21/apisix-dashboard/internal/auth"
	"github.com/carloseduardo21/apisix-dashboard/internal/i18n"
)

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) errorBody {
	t.Helper()
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON %q: %v", rec.Body.String(), err)
	}
	return body
}

func TestErrorResponses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		write      func(w http.ResponseWriter)
		status     int
		code       string
		text       string
		params     map[string]string
		numDetails int
	}{
		{
			name: "simple",
			write: func(w http.ResponseWriter) {
				writeError(w, http.StatusBadRequest, "x.y", "bad {what}", "what", "input")
			},
			status: http.StatusBadRequest, code: "x.y", text: "bad input", params: map[string]string{"what": "input"},
		},
		{
			name:   "not found",
			write:  notFound,
			status: http.StatusNotFound, code: "api.not_found", text: "not found",
		},
		{
			name: "auth sentinel",
			write: func(w http.ResponseWriter) {
				handleErr(w, auth.ErrInvalidCredentials)
			},
			status: http.StatusUnauthorized, code: "auth.invalid_credentials", text: "invalid username or password",
		},
		{
			name: "joined validation errors",
			write: func(w http.ResponseWriter) {
				writeUserError(w, http.StatusBadRequest, errors.Join(i18n.E("a.one", "one"), i18n.E("a.two", "two")))
			},
			status: http.StatusBadRequest, code: "api.multiple_errors", text: "one; two", numDetails: 2,
		},
		{
			name:   "internal error hides details",
			write:  func(w http.ResponseWriter) { internalError(w, errors.New("db password=secret")) },
			status: http.StatusInternalServerError, code: "api.internal_error", text: "internal error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			tt.write(rec)
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d", rec.Code, tt.status)
			}
			body := decodeError(t, rec)
			if body.Code != tt.code || body.Error != tt.text || len(body.Details) != tt.numDetails {
				t.Fatalf("body = %+v", body)
			}
			for k, v := range tt.params {
				if body.Params[k] != v {
					t.Fatalf("params = %v, want %v", body.Params, tt.params)
				}
			}
		})
	}
}

func TestWriteUserErrorWithoutMessage(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	if writeUserError(rec, http.StatusBadRequest, errors.New("internal")) {
		t.Fatal("a plain error must not be written as a user message")
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("nothing should be written, got %q", rec.Body.String())
	}
}
