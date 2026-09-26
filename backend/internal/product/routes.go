package product

import (
	"net/http"

	"shop/internal/httpx"
)

// Register adds the product routes to mux. Other methods on these paths get a
// JSON 405.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/product", h.List)
	mux.HandleFunc("GET /api/product/{productId}", h.Get)

	mux.Handle("/api/product", httpx.MethodNotAllowed("GET, HEAD"))
	mux.Handle("/api/product/{productId}", httpx.MethodNotAllowed("GET, HEAD"))
}
