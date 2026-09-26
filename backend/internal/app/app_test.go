package app

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shop/internal/config"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name     string
		cfg      config.Config
		wantErr  bool
		wantJSON bool
	}{
		{"development uses text logs", config.Config{Env: "development", HTTPAddr: ":8080", LogLevel: "info", APIKey: "k", DatabaseURL: "postgres://x", DataDir: "./data", ImportMinFiles: 2, AllowedOrigins: []string{"*"}}, false, false},
		{"production uses JSON logs", config.Config{Env: "production", HTTPAddr: ":8080", LogLevel: "info", APIKey: "k", DatabaseURL: "postgres://x", DataDir: "./data", ImportMinFiles: 2, AllowedOrigins: []string{"*"}}, false, true},
		{"invalid config is rejected", config.Config{Env: "nope", HTTPAddr: ":8080", LogLevel: "info", APIKey: "k", DatabaseURL: "postgres://x", DataDir: "./data", ImportMinFiles: 2, AllowedOrigins: []string{"*"}}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			a, err := newApp(tt.cfg, &out, fakeCodes{"HAPPYHRS": true})
			if tt.wantErr {
				if err == nil {
					t.Fatal("want error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			w := httptest.NewRecorder()
			a.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
			if w.Code != http.StatusOK {
				t.Errorf("healthz status = %d", w.Code)
			}
			w = httptest.NewRecorder()
			a.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/product/1", nil))
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"id":"1"`) {
				t.Errorf("product route not wired: %d %s", w.Code, w.Body)
			}
			for _, c := range []struct {
				key, body string
				want      int
			}{
				{tt.cfg.APIKey, `{"items":[{"productId":"1","quantity":1}]}`, http.StatusOK},
				{"", `{"items":[{"productId":"1","quantity":1}]}`, http.StatusUnauthorized},
				{"wrong", `{"items":[{"productId":"1","quantity":1}]}`, http.StatusForbidden},
				// Coupons come from the injected store (Postgres in production).
				{tt.cfg.APIKey, `{"couponCode":"HAPPYHRS","items":[{"productId":"1","quantity":1}]}`, http.StatusOK},
				{tt.cfg.APIKey, `{"couponCode":"NOTACODE","items":[{"productId":"1","quantity":1}]}`, http.StatusUnprocessableEntity},
			} {
				r := httptest.NewRequest(http.MethodPost, "/api/order", strings.NewReader(c.body))
				r.Header.Set("Content-Type", "application/json")
				if c.key != "" {
					r.Header.Set("api_key", c.key)
				}
				w = httptest.NewRecorder()
				a.Handler.ServeHTTP(w, r)
				if w.Code != c.want {
					t.Errorf("POST /api/order key=%q: status %d, want %d; %s", c.key, w.Code, c.want, w.Body)
				}
			}
			if isJSON := strings.HasPrefix(out.String(), "{"); isJSON != tt.wantJSON {
				t.Errorf("log output %q, want JSON = %v", out.String(), tt.wantJSON)
			}
		})
	}
}

// fakeCodes is a coupon.CodeStore over a fixed set of codes.
type fakeCodes map[string]bool

func (f fakeCodes) Exists(_ context.Context, code string) (bool, error) { return f[code], nil }

type failingCodes struct{}

func (failingCodes) Exists(context.Context, string) (bool, error) {
	return false, errors.New("connection refused")
}

func TestCouponStoreFailureIs500(t *testing.T) {
	cfg := config.Config{Env: "test", HTTPAddr: ":0", LogLevel: "error", APIKey: "k", DatabaseURL: "postgres://x", DataDir: "./data", ImportMinFiles: 2, AllowedOrigins: []string{"*"}}
	var out bytes.Buffer
	a, err := newApp(cfg, &out, failingCodes{})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/order",
		strings.NewReader(`{"couponCode":"HAPPYHRS","items":[{"productId":"1","quantity":1}]}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("api_key", "k")
	w := httptest.NewRecorder()
	a.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "connection refused") {
		t.Errorf("got %d %s; want 500 without internal details", w.Code, w.Body)
	}
	if !strings.Contains(out.String(), "connection refused") {
		t.Errorf("store error not logged: %s", out.String())
	}
}

func TestNewRejectsInvalidConfigBeforeConnecting(t *testing.T) {
	if _, err := New(context.Background(), config.Config{}); err == nil {
		t.Fatal("want config error")
	}
}

func TestNewFailsWhenDatabaseUnreachable(t *testing.T) {
	cfg := config.Config{Env: "test", HTTPAddr: ":0", LogLevel: "error", APIKey: "k",
		DatabaseURL: "postgres://shop:shop@127.0.0.1:1/shop?sslmode=disable&connect_timeout=1", DataDir: "./data", ImportMinFiles: 2, AllowedOrigins: []string{"*"}}
	if _, err := New(context.Background(), cfg); err == nil {
		t.Fatal("want connection error")
	}
}

// TestBrowserCORSFlow is what a front end on another origin does: a
// preflight (no api_key yet), then the real request. Error responses must
// carry the CORS headers too, or the browser hides them from the page.
func TestBrowserCORSFlow(t *testing.T) {
	const shop = "https://shop.example.com"
	cfg := config.Config{Env: "test", HTTPAddr: ":0", LogLevel: "error", APIKey: "k",
		AllowedOrigins: []string{shop}, DatabaseURL: "postgres://x", DataDir: "./data", ImportMinFiles: 2}
	var out bytes.Buffer
	a, err := newApp(cfg, &out, fakeCodes{"HAPPYHRS": true})
	if err != nil {
		t.Fatal(err)
	}
	send := func(method, origin, key, body string, preflight bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/order", strings.NewReader(body))
		r.Header.Set("Origin", origin)
		if preflight {
			r.Header.Set("Access-Control-Request-Method", http.MethodPost)
			r.Header.Set("Access-Control-Request-Headers", "content-type,api_key")
		} else {
			r.Header.Set("Content-Type", "application/json")
		}
		if key != "" {
			r.Header.Set("api_key", key)
		}
		w := httptest.NewRecorder()
		a.Handler.ServeHTTP(w, r)
		return w
	}
	order := `{"couponCode":"HAPPYHRS","items":[{"productId":"1","quantity":1}]}`

	tests := []struct {
		name       string
		w          *httptest.ResponseRecorder
		wantStatus int
		wantOrigin string
	}{
		{"preflight needs no api_key", send(http.MethodOptions, shop, "", "", true), http.StatusNoContent, shop},
		{"order succeeds", send(http.MethodPost, shop, "k", order, false), http.StatusOK, shop},
		{"401 is readable by the page", send(http.MethodPost, shop, "", order, false), http.StatusUnauthorized, shop},
		{"422 is readable by the page", send(http.MethodPost, shop, "k", `{"items":[]}`, false), http.StatusUnprocessableEntity, shop},
		{"preflight from another site", send(http.MethodOptions, "https://evil.example", "", "", true), http.StatusForbidden, ""},
		{"request from another site gets no CORS headers", send(http.MethodPost, "https://evil.example", "k", order, false), http.StatusOK, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; %s", tt.w.Code, tt.wantStatus, tt.w.Body)
			}
			if got := tt.w.Header().Get("Access-Control-Allow-Origin"); got != tt.wantOrigin {
				t.Errorf("Allow-Origin = %q, want %q", got, tt.wantOrigin)
			}
			if tt.w.Header().Get("X-Request-ID") == "" {
				t.Error("missing X-Request-ID")
			}
		})
	}
}
