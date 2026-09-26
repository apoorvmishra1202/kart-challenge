package httpapi

import (
	"log/slog"
	"net/http"

	"shop/internal/httpx"
)

// NewRouter returns the API handler with the standard middleware applied.
// Unknown routes get a JSON 404 and known paths with the wrong method a JSON
// 405, both in the APIError shape.
func NewRouter(logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	// Method-less patterns are less specific than "GET /healthz", so they
	// only catch the other methods.
	mux.Handle("/healthz", methodNotAllowed("GET, HEAD"))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(w, http.StatusNotFound, httpx.TypeNotFound, "route not found")
	})

	return chain(mux, RequestID, Logger(logger), Recover(logger))
}

func methodNotAllowed(allow string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", allow)
		httpx.WriteError(w, http.StatusMethodNotAllowed, httpx.TypeMethodNotAllowed, "method not allowed")
	})
}
