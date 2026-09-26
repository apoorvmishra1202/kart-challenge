package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shop/internal/httpx"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
})

func decodeAPIError(t *testing.T, body []byte) httpx.APIError {
	t.Helper()
	var e httpx.APIError
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatalf("body %q is not an APIError: %v", body, err)
	}
	return e
}

func TestAPIKey(t *testing.T) {
	tests := []struct {
		name       string
		header     map[string]string
		wantStatus int
		wantType   string
	}{
		{"correct key", map[string]string{"api_key": "apitest"}, http.StatusNoContent, ""},
		{"header name is case-insensitive", map[string]string{"API_KEY": "apitest"}, http.StatusNoContent, ""},
		{"missing header", nil, http.StatusUnauthorized, httpx.TypeUnauthorized},
		{"empty header", map[string]string{"api_key": ""}, http.StatusUnauthorized, httpx.TypeUnauthorized},
		{"wrong key", map[string]string{"api_key": "wrong"}, http.StatusForbidden, httpx.TypeForbidden},
		{"key prefix", map[string]string{"api_key": "apites"}, http.StatusForbidden, httpx.TypeForbidden},
		{"key with suffix", map[string]string{"api_key": "apitest2"}, http.StatusForbidden, httpx.TypeForbidden},
		{"key wrong case", map[string]string{"api_key": "APITEST"}, http.StatusForbidden, httpx.TypeForbidden},
		{"other auth header only", map[string]string{"Authorization": "apitest"}, http.StatusUnauthorized, httpx.TypeUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				okHandler(w, r)
			})
			r := httptest.NewRequest(http.MethodPost, "/order", nil)
			for k, v := range tt.header {
				r.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			APIKey("apitest")(next).ServeHTTP(w, r)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if tt.wantType == "" {
				if !called {
					t.Error("next handler not called")
				}
				return
			}
			if called {
				t.Error("next handler called for rejected request")
			}
			e := decodeAPIError(t, w.Body.Bytes())
			if e.Code != tt.wantStatus || e.Type != tt.wantType || e.Message == "" {
				t.Errorf("body = %+v", e)
			}
		})
	}
}

func TestRecover(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	t.Run("panic becomes 500 APIError", func(t *testing.T) {
		h := Recover(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

		if w.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", w.Code)
		}
		if e := decodeAPIError(t, w.Body.Bytes()); e.Type != httpx.TypeInternal {
			t.Errorf("type = %q", e.Type)
		}
		if !strings.Contains(logs.String(), "boom") || !strings.Contains(logs.String(), "stack=") {
			t.Errorf("panic not logged with stack: %s", logs.String())
		}
	})

	t.Run("panic after headers keeps original status", func(t *testing.T) {
		h := Recover(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			panic("late")
		}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		if w.Code != http.StatusAccepted || w.Body.Len() != 0 {
			t.Errorf("got %d %q, want untouched 202", w.Code, w.Body.String())
		}
	})

	t.Run("ErrAbortHandler is re-panicked", func(t *testing.T) {
		defer func() {
			if p := recover(); p != http.ErrAbortHandler {
				t.Errorf("recovered %v, want http.ErrAbortHandler", p)
			}
		}()
		h := Recover(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) }))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	})
}

func TestRequestID(t *testing.T) {
	tests := []struct {
		name     string
		incoming string
		keep     bool
	}{
		{"generated when missing", "", false},
		{"client ID reused", "abc-123", true},
		{"ID with spaces replaced", "abc 123", false},
		{"control characters replaced", "abc\x01", false},
		{"too long replaced", strings.Repeat("a", maxRequestIDLen+1), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seen string
			h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen = RequestIDFrom(r.Context())
			}))
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.incoming != "" {
				r.Header.Set(requestIDHeader, tt.incoming)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)

			if seen == "" || w.Header().Get(requestIDHeader) != seen {
				t.Fatalf("context ID %q, header %q", seen, w.Header().Get(requestIDHeader))
			}
			if tt.keep != (seen == tt.incoming) {
				t.Errorf("ID = %q, incoming %q, keep = %v", seen, tt.incoming, tt.keep)
			}
			if !tt.keep && len(seen) != 32 {
				t.Errorf("generated ID %q is not 32 hex chars", seen)
			}
		})
	}
}

func TestLoggerRecordsStatus(t *testing.T) {
	var logs bytes.Buffer
	h := chain(okHandler, RequestID, Logger(slog.New(slog.NewTextHandler(&logs, nil))))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))
	for _, want := range []string{"status=204", "method=GET", "path=/x", "request_id="} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("log %q missing %s", logs.String(), want)
		}
	}
}
