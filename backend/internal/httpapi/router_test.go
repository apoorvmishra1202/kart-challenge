package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"shop/internal/httpx"
)

func TestRouter(t *testing.T) {
	h := NewRouter(discardLogger(), []string{"*"})
	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantType   string // "" for success
		wantBody   string
		wantAllow  string
	}{
		{name: "healthz", method: http.MethodGet, path: "/healthz",
			wantStatus: http.StatusOK, wantBody: "{\"status\":\"ok\"}\n"},
		{name: "unknown route", method: http.MethodGet, path: "/nope",
			wantStatus: http.StatusNotFound, wantType: httpx.TypeNotFound},
		{name: "root", method: http.MethodGet, path: "/",
			wantStatus: http.StatusNotFound, wantType: httpx.TypeNotFound},
		{name: "healthz subpath", method: http.MethodGet, path: "/healthz/x",
			wantStatus: http.StatusNotFound, wantType: httpx.TypeNotFound},
		{name: "wrong method", method: http.MethodPost, path: "/healthz",
			wantStatus: http.StatusMethodNotAllowed, wantType: httpx.TypeMethodNotAllowed, wantAllow: "GET, HEAD"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(tt.method, tt.path, nil))

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if w.Header().Get("X-Request-ID") == "" {
				t.Error("missing X-Request-ID")
			}
			if tt.wantAllow != "" && w.Header().Get("Allow") != tt.wantAllow {
				t.Errorf("Allow = %q, want %q", w.Header().Get("Allow"), tt.wantAllow)
			}
			if tt.wantType == "" {
				if w.Body.String() != tt.wantBody {
					t.Errorf("body = %q, want %q", w.Body.String(), tt.wantBody)
				}
				return
			}
			if e := decodeAPIError(t, w.Body.Bytes()); e.Code != tt.wantStatus || e.Type != tt.wantType {
				t.Errorf("body = %+v", e)
			}
		})
	}
}
