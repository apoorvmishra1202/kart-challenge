package app

import (
	"bytes"
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
		{"development uses text logs", config.Config{Env: "development", HTTPAddr: ":8080", LogLevel: "info", APIKey: "k", DatabaseURL: "postgres://x", DataDir: "./data", ImportMinFiles: 2}, false, false},
		{"production uses JSON logs", config.Config{Env: "production", HTTPAddr: ":8080", LogLevel: "info", APIKey: "k", DatabaseURL: "postgres://x", DataDir: "./data", ImportMinFiles: 2}, false, true},
		{"invalid config is rejected", config.Config{Env: "nope", HTTPAddr: ":8080", LogLevel: "info", APIKey: "k", DatabaseURL: "postgres://x", DataDir: "./data", ImportMinFiles: 2}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			a, err := newApp(tt.cfg, &out)
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
				// No coupons are loaded yet, so any code is rejected.
				{tt.cfg.APIKey, `{"couponCode":"HAPPYHRS","items":[{"productId":"1","quantity":1}]}`, http.StatusUnprocessableEntity},
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
