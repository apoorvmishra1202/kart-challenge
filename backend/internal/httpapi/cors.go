package httpapi

import (
	"net/http"
	"strings"

	"shop/internal/httpx"
)

const anyOrigin = "*"

// CORS values sent to allowed origins. Allowed headers cover what the API
// reads (JSON bodies, the api_key header, a client request ID); exposed
// headers let the front end read the request ID for support/debugging.
var (
	corsAllowMethods  = strings.Join([]string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodOptions}, ", ")
	corsAllowHeaders  = strings.Join([]string{"Content-Type", "api_key", requestIDHeader}, ", ")
	corsExposeHeaders = requestIDHeader
)

const corsMaxAge = "600" // seconds browsers may cache a preflight result

// CORS lets browsers on the allowed origins call the API. allowed is either
// ["*"] (any origin) or exact origins such as "https://shop.example.com".
//
//   - Requests without an Origin header (curl, servers) pass through untouched.
//   - Preflights (OPTIONS with Access-Control-Request-Method) are answered here
//     with 204 for allowed origins, or 403 otherwise, and never reach the
//     routes, so they don't need the api_key.
//   - Other requests from an allowed origin get Access-Control-Allow-Origin on
//     every response, errors included, so the browser can read error bodies.
//     A disallowed origin gets no CORS headers and the browser blocks it.
//
// No credentials are allowed: the API uses a header key, not cookies.
func CORS(allowed []string) Middleware {
	wildcard := len(allowed) == 1 && allowed[0] == anyOrigin
	set := make(map[string]bool, len(allowed))
	for _, o := range allowed {
		set[o] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			h := w.Header()
			if !wildcard {
				// The response differs per origin, so caches must key on it.
				h.Add("Vary", "Origin")
			}
			ok := wildcard || set[origin]
			preflight := r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != ""

			if ok {
				if wildcard {
					h.Set("Access-Control-Allow-Origin", anyOrigin)
				} else {
					h.Set("Access-Control-Allow-Origin", origin)
				}
			}

			if preflight {
				h.Add("Vary", "Access-Control-Request-Method")
				h.Add("Vary", "Access-Control-Request-Headers")
				if !ok {
					httpx.WriteError(w, http.StatusForbidden, httpx.TypeForbidden, "origin not allowed")
					return
				}
				h.Set("Access-Control-Allow-Methods", corsAllowMethods)
				h.Set("Access-Control-Allow-Headers", corsAllowHeaders)
				h.Set("Access-Control-Max-Age", corsMaxAge)
				w.WriteHeader(http.StatusNoContent)
				return
			}

			if ok {
				h.Set("Access-Control-Expose-Headers", corsExposeHeaders)
			}
			next.ServeHTTP(w, r)
		})
	}
}
