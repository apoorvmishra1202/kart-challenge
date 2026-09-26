package httpx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type item struct {
	ProductID string `json:"productId"`
	Quantity  int    `json:"quantity"`
}

func TestDecodeJSON(t *testing.T) {
	const limit = 64
	tests := []struct {
		name        string
		contentType string
		body        string
		want        item
		wantStatus  int    // 0 means success
		wantMsg     string // substring of RequestError.Message
	}{
		{name: "valid", contentType: "application/json", body: `{"productId":"10","quantity":2}`,
			want: item{"10", 2}},
		{name: "charset utf-8", contentType: "application/json; charset=utf-8", body: `{"productId":"1"}`,
			want: item{ProductID: "1"}},
		{name: "charset uppercase", contentType: "application/json; charset=UTF-8", body: `{}`},
		{name: "media type case-insensitive", contentType: "Application/JSON", body: `{}`},
		{name: "trailing whitespace ok", contentType: "application/json", body: "{}\n\n"},

		{name: "missing content type", contentType: "", body: `{}`,
			wantStatus: http.StatusUnsupportedMediaType, wantMsg: "application/json"},
		{name: "wrong content type", contentType: "text/plain", body: `{}`,
			wantStatus: http.StatusUnsupportedMediaType},
		{name: "json-like content type", contentType: "application/json-patch+json", body: `{}`,
			wantStatus: http.StatusUnsupportedMediaType},
		{name: "malformed content type", contentType: "application/json; charset", body: `{}`,
			wantStatus: http.StatusUnsupportedMediaType},
		{name: "empty body", contentType: "application/json", body: "",
			wantStatus: http.StatusBadRequest, wantMsg: "must not be empty"},
		{name: "syntax error", contentType: "application/json", body: `{"productId":}`,
			wantStatus: http.StatusBadRequest, wantMsg: "malformed JSON at position"},
		{name: "truncated", contentType: "application/json", body: `{"productId":"1"`,
			wantStatus: http.StatusBadRequest, wantMsg: "malformed JSON"},
		{name: "wrong field type", contentType: "application/json", body: `{"quantity":"two"}`,
			wantStatus: http.StatusBadRequest, wantMsg: `field "quantity" must be of type int`},
		{name: "array instead of object", contentType: "application/json", body: `[1,2]`,
			wantStatus: http.StatusBadRequest, wantMsg: "must be a JSON"},
		{name: "unknown field", contentType: "application/json", body: `{"productId":"1","price":5}`,
			wantStatus: http.StatusBadRequest, wantMsg: `unknown field "price"`},
		{name: "two objects", contentType: "application/json", body: `{}{}`,
			wantStatus: http.StatusBadRequest, wantMsg: "single JSON object"},
		{name: "trailing garbage", contentType: "application/json", body: `{} x`,
			wantStatus: http.StatusBadRequest, wantMsg: "single JSON object"},
		{name: "too large", contentType: "application/json",
			body:       `{"productId":"` + strings.Repeat("x", limit) + `"}`,
			wantStatus: http.StatusRequestEntityTooLarge, wantMsg: "64 bytes"},
		{name: "too large after first object", contentType: "application/json",
			body:       `{}` + strings.Repeat(" ", limit),
			wantStatus: http.StatusRequestEntityTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/order", strings.NewReader(tt.body))
			if tt.contentType != "" {
				r.Header.Set("Content-Type", tt.contentType)
			}
			got, err := DecodeJSON[item](httptest.NewRecorder(), r, limit)

			if tt.wantStatus == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != tt.want {
					t.Errorf("got %+v, want %+v", got, tt.want)
				}
				return
			}
			var re *RequestError
			if !errors.As(err, &re) {
				t.Fatalf("err = %v (%T), want *RequestError", err, err)
			}
			if re.Status != tt.wantStatus {
				t.Errorf("status = %d, want %d (%s)", re.Status, tt.wantStatus, re.Message)
			}
			if re.Type == "" {
				t.Error("empty error type")
			}
			if !strings.Contains(re.Message, tt.wantMsg) {
				t.Errorf("message %q does not contain %q", re.Message, tt.wantMsg)
			}
		})
	}
}
