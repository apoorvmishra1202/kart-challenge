package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteError(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		wantStatus int
	}{
		{"client error", http.StatusNotFound, http.StatusNotFound},
		{"server error", http.StatusServiceUnavailable, http.StatusServiceUnavailable},
		{"success code is never used for errors", http.StatusOK, http.StatusInternalServerError},
		{"redirect code is never used for errors", http.StatusFound, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			WriteError(w, tt.status, TypeNotFound, "nope")

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if ct := w.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
				t.Errorf("Content-Type = %q", ct)
			}
			var got APIError
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			want := APIError{Code: tt.wantStatus, Type: TypeNotFound, Message: "nope"}
			if got != want {
				t.Errorf("body = %+v, want %+v", got, want)
			}
		})
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSON(w, http.StatusCreated, map[string]int{"n": 1})
	if w.Code != http.StatusCreated || w.Body.String() != "{\"n\":1}\n" {
		t.Errorf("got %d %q", w.Code, w.Body.String())
	}
}
