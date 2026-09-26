// Package httpapi holds the HTTP router and middleware.
package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"shop/internal/httpx"
)

// Middleware wraps a handler.
type Middleware func(http.Handler) http.Handler

// chain applies mws so that the first one is outermost.
func chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

type ctxKey int

const requestIDKey ctxKey = iota

const (
	requestIDHeader = "X-Request-ID"
	maxRequestIDLen = 128
)

// RequestID reuses a well-formed X-Request-ID from the client or generates a
// new one, stores it in the request context, and echoes it in the response.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if !validRequestID(id) {
			id = newRequestID()
		}
		w.Header().Set(requestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

// RequestIDFrom returns the request ID set by RequestID, or "".
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

func newRequestID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never fails on supported platforms
	return hex.EncodeToString(b[:])
}

// validRequestID accepts printable ASCII without spaces, so a client-supplied
// ID can't inject anything into logs or headers.
func validRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		if id[i] <= ' ' || id[i] > '~' {
			return false
		}
	}
	return true
}

// statusRecorder captures the status code and body size for logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// Logger logs one line per request with its status, size and duration.
func Logger(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)
			if rec.status == 0 {
				rec.status = http.StatusOK
			}
			level := slog.LevelInfo
			if rec.status >= http.StatusInternalServerError {
				level = slog.LevelError
			}
			logger.LogAttrs(r.Context(), level, "request",
				slog.String("request_id", RequestIDFrom(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Int("bytes", rec.bytes),
				slog.Duration("duration", time.Since(start)),
			)
		})
	}
}

// Recover turns a panic into a 500 APIError and logs it with its stack
// trace. http.ErrAbortHandler is re-panicked so net/http can abort the
// connection as intended.
func Recover(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &statusRecorder{ResponseWriter: w}
			defer func() {
				p := recover()
				if p == nil {
					return
				}
				if p == http.ErrAbortHandler {
					panic(p)
				}
				logger.LogAttrs(r.Context(), slog.LevelError, "panic",
					slog.String("request_id", RequestIDFrom(r.Context())),
					slog.Any("panic", p),
					slog.String("stack", string(debug.Stack())),
				)
				if rec.status == 0 { // headers not sent yet
					httpx.WriteError(rec, http.StatusInternalServerError, httpx.TypeInternal, "internal server error")
				}
			}()
			next.ServeHTTP(rec, r)
		})
	}
}

// APIKey requires the api_key header: 401 when it is missing or empty, 403
// when it doesn't match. The comparison is constant-time.
func APIKey(key string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := r.Header.Get("api_key")
			if got == "" {
				httpx.WriteError(w, http.StatusUnauthorized, httpx.TypeUnauthorized, "missing api_key header")
				return
			}
			if subtle.ConstantTimeCompare([]byte(got), []byte(key)) != 1 {
				httpx.WriteError(w, http.StatusForbidden, httpx.TypeForbidden, "invalid api_key")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
