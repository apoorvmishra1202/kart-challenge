package order

import (
	"net/http"

	"shop/internal/httpx"
)

// Register adds POST /api/order, wrapped with protect (the API key check).
// Other methods get a JSON 405 without requiring the key.
func (h *Handler) Register(mux *http.ServeMux, protect func(http.Handler) http.Handler) {
	mux.Handle("POST /api/order", protect(http.HandlerFunc(h.Place)))
	mux.Handle("/api/order", httpx.MethodNotAllowed("POST"))
}
