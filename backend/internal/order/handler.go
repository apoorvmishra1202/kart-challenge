package order

import (
	"errors"
	"fmt"
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
		msg := "invalid request body"
		var re *httpx.RequestError
		if errors.As(err, &re) {
			msg = re.Message
		}
		httpx.WriteError(w, http.StatusBadRequest, httpx.TypeBadRequest, msg)
		return
	}
	// Validate builds its messages from the request fields alone.
	if err := req.Validate(); err != nil {
		httpx.WriteError(w, http.StatusUnprocessableEntity, httpx.TypeUnprocessable, err.Error())
		return
	}

	o, err := h.svc.Place(r.Context(), req.toItems(), req.CouponCode)
	if err != nil {
		status := statusFor(err)
		if status == http.StatusUnprocessableEntity {
			httpx.WriteError(w, status, httpx.TypeUnprocessable, clientMessage(err))
			return
		}
		h.log.ErrorContext(r.Context(), "place order failed", "err", err)
		httpx.WriteError(w, status, httpx.TypeInternal, "internal server error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toOrderResponse(o))
}

// clientErrors are the sentinels that map to 422.
var clientErrors = []error{ErrUnknownProduct, ErrInvalidCoupon, ErrQuantityLimit}

// statusFor maps service errors to HTTP status codes.
func statusFor(err error) int {
	for _, sentinel := range clientErrors {
		if errors.Is(err, sentinel) {
			return http.StatusUnprocessableEntity
		}
	}
	return http.StatusInternalServerError
}

// clientMessage builds a 422 message from the matched sentinel and, when
// available, the offending value; never from the whole error chain, so
// context added by wrapping can't leak to the client.
func clientMessage(err error) string {
	for _, sentinel := range clientErrors {
		if !errors.Is(err, sentinel) {
			continue
		}
		var ie *InputError
		if errors.As(err, &ie) && errors.Is(ie.Err, sentinel) {
			return fmt.Sprintf("%s: %q", sentinel.Error(), ie.Value)
		}
		return sentinel.Error()
	}
	return "unprocessable request"
}
