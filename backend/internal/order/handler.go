package order

import (
	"errors"
	"log/slog"
	"net/http"

	"shop/internal/httpx"
)

// maxBodyBytes caps the request body; 100 items fit in a few KB.
const maxBodyBytes = 64 << 10

type Handler struct {
	svc *Service
	log *slog.Logger
}

func NewHandler(svc *Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// Place serves POST /api/order. The spec allows 200, 400, 401, 403 and 422,
// so every body-decoding failure (including an oversized body or wrong
// Content-Type) is reported as 400.
func (h *Handler) Place(w http.ResponseWriter, r *http.Request) {
	req, err := httpx.DecodeJSON[PlaceOrderRequest](w, r, maxBodyBytes)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.TypeBadRequest, err.Error())
		return
	}
	if err := req.Validate(); err != nil {
		httpx.WriteError(w, http.StatusUnprocessableEntity, httpx.TypeUnprocessable, err.Error())
		return
	}

	o, err := h.svc.Place(r.Context(), req.toItems(), req.CouponCode)
	if err != nil {
		status := statusFor(err)
		if status == http.StatusUnprocessableEntity {
			httpx.WriteError(w, status, httpx.TypeUnprocessable, err.Error())
			return
		}
		h.log.ErrorContext(r.Context(), "place order failed", "err", err)
		httpx.WriteError(w, status, httpx.TypeInternal, "internal server error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toOrderResponse(o))
}

// statusFor maps service errors to HTTP status codes.
func statusFor(err error) int {
	switch {
	case errors.Is(err, ErrUnknownProduct), errors.Is(err, ErrInvalidCoupon):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}
