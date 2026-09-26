package httpapi

import (
	"log/slog"
	"net/http"

	"shop/internal/httpx"
)

// Registrar adds a domain's routes to the mux; each domain handler (such as
// *product.Handler) implements it.
type Registrar interface {
	Register(mux *http.ServeMux)
}

// RegistrarFunc adapts a function to a Registrar, like http.HandlerFunc. Use it
// for handlers whose Register needs extra arguments, such as middleware.
type RegistrarFunc func(mux *http.ServeMux)

func (f RegistrarFunc) Register(mux *http.ServeMux) { f(mux) }

// NewRouter returns the API handler with the standard middleware applied and
// every registrar's routes mounted. Unknown routes get a JSON 404 and known
// paths with the wrong method a JSON 405, both in the APIError shape.
func NewRouter(logger *slog.Logger, registrars ...Registrar) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	// Method-less patterns are less specific than "GET /healthz", so they
	// only catch the other methods.
	mux.Handle("/healthz", httpx.MethodNotAllowed("GET, HEAD"))

	for _, r := range registrars {
		r.Register(mux)
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(w, http.StatusNotFound, httpx.TypeNotFound, "route not found")
	})

	return chain(mux, RequestID, Logger(logger), Recover(logger))
}
