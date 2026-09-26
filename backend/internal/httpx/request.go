package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
)

// RequestError is returned by DecodeJSON. Status and Type say how the handler
// should respond, e.g. WriteError(w, e.Status, e.Type, e.Message).
type RequestError struct {
	Status  int
	Type    string
	Message string
}

func (e *RequestError) Error() string { return e.Message }

func badRequest(format string, args ...any) *RequestError {
	return &RequestError{Status: http.StatusBadRequest, Type: TypeBadRequest, Message: fmt.Sprintf(format, args...)}
}

// DecodeJSON decodes a single JSON object from the request body into T. It
// requires Content-Type application/json (any parameters, such as charset,
// are allowed), limits the body to maxBytes, and rejects unknown fields and
// trailing data. Every error it returns is a *RequestError.
func DecodeJSON[T any](w http.ResponseWriter, r *http.Request, maxBytes int64) (T, error) {
	var v T

	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return v, &RequestError{
			Status:  http.StatusUnsupportedMediaType,
			Type:    TypeUnsupportedMediaType,
			Message: "Content-Type must be application/json",
		}
	}

	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes))
	dec.DisallowUnknownFields()

	if err := dec.Decode(&v); err != nil {
		return v, decodeError(err, maxBytes)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return v, decodeError(err, maxBytes)
		}
		return v, badRequest("request body must contain a single JSON object")
	}
	return v, nil
}

func decodeError(err error, maxBytes int64) *RequestError {
	var (
		syntaxErr *json.SyntaxError
		typeErr   *json.UnmarshalTypeError
		maxErr    *http.MaxBytesError
	)
	switch {
	case errors.As(err, &maxErr):
		return &RequestError{
			Status:  http.StatusRequestEntityTooLarge,
			Type:    TypePayloadTooLarge,
			Message: fmt.Sprintf("request body must not exceed %d bytes", maxBytes),
		}
	case errors.Is(err, io.EOF):
		return badRequest("request body must not be empty")
	case errors.As(err, &syntaxErr):
		return badRequest("malformed JSON at position %d", syntaxErr.Offset)
	case errors.Is(err, io.ErrUnexpectedEOF):
		return badRequest("malformed JSON")
	case errors.As(err, &typeErr):
		if typeErr.Field != "" {
			return badRequest("field %q must be of type %s", typeErr.Field, typeErr.Type)
		}
		return badRequest("request body must be a JSON %s", typeErr.Type)
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		// encoding/json has no typed error for unknown fields.
		return badRequest("unknown field %s", strings.TrimPrefix(err.Error(), "json: unknown field "))
	default:
		return badRequest("invalid request body")
	}
}
