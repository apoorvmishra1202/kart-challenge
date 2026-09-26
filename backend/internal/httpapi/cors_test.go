package httpapi

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"shop/internal/httpx"
)

func TestCORS(t *testing.T) {
	const shop = "https://shop.example.com"
	explicit := []string{shop, "http://localhost:3000"}
	wildcard := []string{"*"}

	tests := []struct {
		name          string
		allowed       []string
		method        string
		origin        string
		requestMethod string // Access-Control-Request-Method; set => preflight

		wantStatus     int
		wantNextCalled bool
		wantAllowOrig  string // "" = header must be absent
		wantPreflight  bool   // Allow-Methods/Headers/Max-Age present
		wantExpose     bool
		wantVaryOrigin bool
	}{
		{name: "no Origin passes through untouched", allowed: explicit, method: http.MethodGet,
			wantStatus: http.StatusNoContent, wantNextCalled: true},
		{name: "wildcard simple request", allowed: wildcard, method: http.MethodGet, origin: shop,
			wantStatus: http.StatusNoContent, wantNextCalled: true, wantAllowOrig: "*", wantExpose: true},
		{name: "allowed origin echoed", allowed: explicit, method: http.MethodPost, origin: shop,
			wantStatus: http.StatusNoContent, wantNextCalled: true, wantAllowOrig: shop, wantExpose: true, wantVaryOrigin: true},
		{name: "second allowed origin", allowed: explicit, method: http.MethodGet, origin: "http://localhost:3000",
			wantStatus: http.StatusNoContent, wantNextCalled: true, wantAllowOrig: "http://localhost:3000", wantExpose: true, wantVaryOrigin: true},
		{name: "disallowed origin gets no CORS headers", allowed: explicit, method: http.MethodGet, origin: "https://evil.example",
			wantStatus: http.StatusNoContent, wantNextCalled: true, wantVaryOrigin: true},
		{name: "origin match is exact", allowed: explicit, method: http.MethodGet, origin: shop + ":443",
			wantStatus: http.StatusNoContent, wantNextCalled: true, wantVaryOrigin: true},

		{name: "preflight wildcard", allowed: wildcard, method: http.MethodOptions, origin: shop, requestMethod: http.MethodPost,
			wantStatus: http.StatusNoContent, wantAllowOrig: "*", wantPreflight: true},
		{name: "preflight allowed origin", allowed: explicit, method: http.MethodOptions, origin: shop, requestMethod: http.MethodPost,
			wantStatus: http.StatusNoContent, wantAllowOrig: shop, wantPreflight: true, wantVaryOrigin: true},
		{name: "preflight disallowed origin is 403", allowed: explicit, method: http.MethodOptions, origin: "https://evil.example", requestMethod: http.MethodPost,
			wantStatus: http.StatusForbidden, wantVaryOrigin: true},
		{name: "OPTIONS without request method is not a preflight", allowed: explicit, method: http.MethodOptions, origin: shop,
			wantStatus: http.StatusNoContent, wantNextCalled: true, wantAllowOrig: shop, wantExpose: true, wantVaryOrigin: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			})
			r := httptest.NewRequest(tt.method, "/api/order", nil)
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			if tt.requestMethod != "" {
				r.Header.Set("Access-Control-Request-Method", tt.requestMethod)
				r.Header.Set("Access-Control-Request-Headers", "content-type, api_key")
			}
			w := httptest.NewRecorder()
			CORS(tt.allowed)(next).ServeHTTP(w, r)
			h := w.Header()

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if called != tt.wantNextCalled {
				t.Errorf("next called = %v, want %v", called, tt.wantNextCalled)
			}
			if got := h.Get("Access-Control-Allow-Origin"); got != tt.wantAllowOrig {
				t.Errorf("Allow-Origin = %q, want %q", got, tt.wantAllowOrig)
			}
			for _, k := range []string{"Access-Control-Allow-Methods", "Access-Control-Allow-Headers", "Access-Control-Max-Age"} {
				if (h.Get(k) != "") != tt.wantPreflight {
					t.Errorf("%s = %q, want present = %v", k, h.Get(k), tt.wantPreflight)
				}
			}
			if (h.Get("Access-Control-Expose-Headers") == requestIDHeader) != tt.wantExpose {
				t.Errorf("Expose-Headers = %q, want present = %v", h.Get("Access-Control-Expose-Headers"), tt.wantExpose)
			}
			if slices.Contains(h.Values("Vary"), "Origin") != tt.wantVaryOrigin {
				t.Errorf("Vary = %v, want Origin present = %v", h.Values("Vary"), tt.wantVaryOrigin)
			}
			if h.Get("Access-Control-Allow-Credentials") != "" {
				t.Error("credentials must never be allowed")
			}
			if tt.wantStatus == http.StatusForbidden {
				if e := decodeAPIError(t, w.Body.Bytes()); e.Type != httpx.TypeForbidden {
					t.Errorf("body = %+v", e)
				}
			}
		})
	}
}

func TestCORSPreflightAllowsTheHeadersTheAPIReads(t *testing.T) {
	r := httptest.NewRequest(http.MethodOptions, "/api/order", nil)
	r.Header.Set("Origin", "https://shop.example.com")
	r.Header.Set("Access-Control-Request-Method", http.MethodPost)
	w := httptest.NewRecorder()
	CORS([]string{"*"})(okHandler).ServeHTTP(w, r)

	for _, want := range []string{"POST", "GET"} {
		if !containsToken(w.Header().Get("Access-Control-Allow-Methods"), want) {
			t.Errorf("Allow-Methods %q missing %s", w.Header().Get("Access-Control-Allow-Methods"), want)
		}
	}
	for _, want := range []string{"Content-Type", "api_key", "X-Request-ID"} {
		if !containsToken(w.Header().Get("Access-Control-Allow-Headers"), want) {
			t.Errorf("Allow-Headers %q missing %s", w.Header().Get("Access-Control-Allow-Headers"), want)
		}
	}
}

func containsToken(list, token string) bool {
	for _, s := range strings.Split(list, ",") {
		if strings.TrimSpace(s) == token {
			return true
		}
	}
	return false
}
