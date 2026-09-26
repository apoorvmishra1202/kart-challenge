// Package httpx holds JSON request and response helpers shared by handlers.
package httpx

import (
	"encoding/json"
	"net/http"
)

// APIError is the error body for every non-2xx response, matching
// ApiResponse in api/openapi.yaml.
type APIError struct {
	Code    int    `json:"code"`
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Error types used in APIError.Type.
const (
	TypeBadRequest           = "bad_request"
	TypeUnauthorized         = "unauthorized"
	TypeForbidden            = "forbidden"
	TypeNotFound             = "not_found"
	TypeMethodNotAllowed     = "method_not_allowed"
	TypePayloadTooLarge      = "payload_too_large"
	TypeUnsupportedMediaType = "unsupported_media_type"
	TypeUnprocessable        = "unprocessable_entity"
	TypeInternal             = "internal_error"
)

// WriteJSON writes v as a JSON response with the given status. Encoding
// errors can't be reported to the client once the header is sent, so they
// are ignored; v should be a plain data type that always encodes.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// MethodNotAllowed returns a handler that answers 405 with an APIError and
// the given Allow header. Register it on a method-less pattern next to the
// method-specific ones, which take precedence.
func MethodNotAllowed(allow string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", allow)
		WriteError(w, http.StatusMethodNotAllowed, TypeMethodNotAllowed, "method not allowed")
	})
}

// WriteError writes an APIError. A status below 400 is a programming error,
// so it is replaced with 500 rather than sending an error body with a success
// code.
func WriteError(w http.ResponseWriter, status int, typ, msg string) {
	if status < http.StatusBadRequest {
		status = http.StatusInternalServerError
	}
	WriteJSON(w, status, APIError{Code: status, Type: typ, Message: msg})
}
