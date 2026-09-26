package product

import (
	"errors"
	"net/http"
	"strconv"

	"shop/internal/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// List serves GET /api/product. It always returns a JSON array, never null.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	products, err := h.svc.List(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	out := make([]ProductResponse, 0, len(products))
	for _, p := range products {
		out = append(out, ToResponse(p))
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// Get serves GET /api/product/{productId}. The spec types productId as an
// int64, so anything other than a positive integer is a 400.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseProductID(r.PathValue("productId"))
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, httpx.TypeBadRequest, "productId must be a positive integer")
		return
	}
	p, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ToResponse(p))
}

// parseProductID accepts only digits (no sign) forming a positive int64 and
// returns its canonical form, so "007" looks up product "7".
func parseProductID(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < '0' || raw[i] > '9' {
			return "", false
		}
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		return "", false
	}
	return strconv.FormatInt(n, 10), true
}

func writeServiceError(w http.ResponseWriter, err error) {
	status := statusFor(err)
	switch status {
	case http.StatusNotFound:
		httpx.WriteError(w, status, httpx.TypeNotFound, "product not found")
	default:
		// Don't leak internal details; the request log records the 500.
		httpx.WriteError(w, status, httpx.TypeInternal, "internal server error")
	}
}

// statusFor maps service errors to HTTP status codes.
func statusFor(err error) int {
	switch {
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}
