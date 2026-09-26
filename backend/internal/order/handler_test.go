package order

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shop/internal/httpapi"
	"shop/internal/httpx"
)

const testKey = "apitest"

type testServer struct {
	mux   *http.ServeMux
	store *fakeStore
	logs  *bytes.Buffer
}

func newTestServer(products *fakeProducts, coupons *fakeCoupons, store *fakeStore) testServer {
	var logs bytes.Buffer
	h := NewHandler(NewService(store, products, coupons), slog.New(slog.NewTextHandler(&logs, nil)))
	mux := http.NewServeMux()
	h.Register(mux, httpapi.APIKey(testKey))
	return testServer{mux: mux, store: store, logs: &logs}
}

func defaultServer() testServer {
	return newTestServer(
		&fakeProducts{byID: catalog},
		&fakeCoupons{valid: map[string]bool{"HAPPYHRS": true}},
		&fakeStore{},
	)
}

func (s testServer) post(body string, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/api/order", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("api_key", testKey)
	for k, v := range headers {
		if v == "" {
			r.Header.Del(k)
		} else {
			r.Header.Set(k, v)
		}
	}
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, r)
	return w
}

func TestHandlerPlaceSuccess(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantItems string
		wantProds []string
	}{
		{"one item", `{"items":[{"productId":"1","quantity":2}]}`,
			`[{"productId":"1","quantity":2}]`, []string{"1"}},
		{"with valid coupon", `{"couponCode":"HAPPYHRS","items":[{"productId":"2","quantity":1}]}`,
			`[{"productId":"2","quantity":1}]`, []string{"2"}},
		{"duplicates merged", `{"items":[{"productId":"1","quantity":1},{"productId":"2","quantity":1},{"productId":"1","quantity":3}]}`,
			`[{"productId":"1","quantity":4},{"productId":"2","quantity":1}]`, []string{"1", "2"}},
		{"empty coupon means no coupon", `{"couponCode":"","items":[{"productId":"1","quantity":1}]}`,
			`[{"productId":"1","quantity":1}]`, []string{"1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := defaultServer()
			w := s.post(tt.body, nil)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, body %s", w.Code, w.Body)
			}

			var raw map[string]json.RawMessage
			if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
				t.Fatal(err)
			}
			if len(raw) != 3 || raw["id"] == nil || raw["items"] == nil || raw["products"] == nil {
				t.Errorf("response fields = %v, want exactly id, items, products", keys(raw))
			}
			if string(raw["items"]) != tt.wantItems {
				t.Errorf("items = %s, want %s", raw["items"], tt.wantItems)
			}

			var resp OrderResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			if !uuidV4.MatchString(resp.ID) {
				t.Errorf("id %q is not a UUIDv4", resp.ID)
			}
			var ids []string
			for _, p := range resp.Products {
				ids = append(ids, p.ID)
				if p.Name == "" || p.Price == 0 || p.Category == "" {
					t.Errorf("incomplete product %+v", p)
				}
			}
			if strings.Join(ids, ",") != strings.Join(tt.wantProds, ",") {
				t.Errorf("product ids = %v, want %v", ids, tt.wantProds)
			}
			if len(s.store.saved) != 1 {
				t.Errorf("saved %d orders, want 1", len(s.store.saved))
			}
		})
	}
}

func TestHandlerPlaceErrors(t *testing.T) {
	validBody := `{"items":[{"productId":"1","quantity":1}]}`
	tests := []struct {
		name       string
		body       string
		headers    map[string]string
		wantStatus int
		wantType   string
		wantMsg    string
	}{
		// 401 / 403: api_key checked before the body is read.
		{"missing api_key", validBody, map[string]string{"api_key": ""}, http.StatusUnauthorized, httpx.TypeUnauthorized, ""},
		{"wrong api_key", validBody, map[string]string{"api_key": "nope"}, http.StatusForbidden, httpx.TypeForbidden, ""},
		{"auth checked before body", `{bad`, map[string]string{"api_key": ""}, http.StatusUnauthorized, httpx.TypeUnauthorized, ""},

		// 400: the body can't be decoded.
		{"malformed JSON", `{"items":[`, nil, http.StatusBadRequest, httpx.TypeBadRequest, "malformed JSON"},
		{"empty body", ``, nil, http.StatusBadRequest, httpx.TypeBadRequest, "empty"},
		{"unknown field", `{"items":[],"total":5}`, nil, http.StatusBadRequest, httpx.TypeBadRequest, `unknown field "total"`},
		{"quantity wrong type", `{"items":[{"productId":"1","quantity":"2"}]}`, nil, http.StatusBadRequest, httpx.TypeBadRequest, "quantity"},
		{"productId wrong type", `{"items":[{"productId":1,"quantity":1}]}`, nil, http.StatusBadRequest, httpx.TypeBadRequest, "productId"},
		{"wrong content type", validBody, map[string]string{"Content-Type": "text/plain"}, http.StatusBadRequest, httpx.TypeBadRequest, "application/json"},
		{"body too large", `{"items":[` + strings.Repeat(`{"productId":"1","quantity":1},`, 4000) + `]}`, nil,
			http.StatusBadRequest, httpx.TypeBadRequest, "must not exceed"},

		// 422: well-formed JSON that fails validation.
		{"items missing", `{}`, nil, http.StatusUnprocessableEntity, httpx.TypeUnprocessable, "items must not be empty"},
		{"items empty", `{"items":[]}`, nil, http.StatusUnprocessableEntity, httpx.TypeUnprocessable, "items must not be empty"},
		{"productId missing", `{"items":[{"quantity":1}]}`, nil, http.StatusUnprocessableEntity, httpx.TypeUnprocessable, "items[0].productId is required"},
		{"quantity missing", `{"items":[{"productId":"1"}]}`, nil, http.StatusUnprocessableEntity, httpx.TypeUnprocessable, "items[0].quantity"},
		{"quantity zero", `{"items":[{"productId":"1","quantity":0}]}`, nil, http.StatusUnprocessableEntity, httpx.TypeUnprocessable, "between 1 and 100"},
		{"quantity negative", `{"items":[{"productId":"1","quantity":-1}]}`, nil, http.StatusUnprocessableEntity, httpx.TypeUnprocessable, "between 1 and 100"},
		{"quantity 101", `{"items":[{"productId":"1","quantity":101}]}`, nil, http.StatusUnprocessableEntity, httpx.TypeUnprocessable, "between 1 and 100"},
		{"all item errors reported", `{"items":[{"quantity":0},{"productId":"2","quantity":500}]}`, nil,
			http.StatusUnprocessableEntity, httpx.TypeUnprocessable, "items[1].quantity"},
		{"unknown product", `{"items":[{"productId":"99","quantity":1}]}`, nil, http.StatusUnprocessableEntity, httpx.TypeUnprocessable, "unknown product"},
		{"invalid coupon", `{"couponCode":"NOPE1234","items":[{"productId":"1","quantity":1}]}`, nil,
			http.StatusUnprocessableEntity, httpx.TypeUnprocessable, "invalid coupon"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := defaultServer()
			w := s.post(tt.body, tt.headers)
			assertAPIError(t, w, tt.wantStatus, tt.wantType, tt.wantMsg)
			if len(s.store.saved) != 0 {
				t.Error("order saved despite error")
			}
		})
	}
}

func TestHandlerPlaceInternalError(t *testing.T) {
	s := newTestServer(&fakeProducts{byID: catalog}, &fakeCoupons{}, &fakeStore{err: errInfra})
	w := s.post(`{"items":[{"productId":"1","quantity":1}]}`, nil)

	assertAPIError(t, w, http.StatusInternalServerError, httpx.TypeInternal, "internal server error")
	if strings.Contains(w.Body.String(), errInfra.Error()) {
		t.Error("internal error details leaked to client")
	}
	if !strings.Contains(s.logs.String(), errInfra.Error()) {
		t.Errorf("real error not logged: %q", s.logs.String())
	}
}

func TestHandlerWrongMethod(t *testing.T) {
	s := defaultServer()
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/order", nil)) // no api_key needed
	assertAPIError(t, w, http.StatusMethodNotAllowed, httpx.TypeMethodNotAllowed, "")
	if w.Header().Get("Allow") != "POST" {
		t.Errorf("Allow = %q", w.Header().Get("Allow"))
	}
}

func TestPlaceOrderRequestValidate(t *testing.T) {
	tests := []struct {
		name string
		req  PlaceOrderRequest
		ok   bool
	}{
		{"minimal", PlaceOrderRequest{Items: []ItemRequest{{"1", 1}}}, true},
		{"quantity 100", PlaceOrderRequest{Items: []ItemRequest{{"1", 100}}}, true},
		{"coupon is optional", PlaceOrderRequest{CouponCode: "X", Items: []ItemRequest{{"1", 1}}}, true},
		{"nil items", PlaceOrderRequest{}, false},
		{"blank productId", PlaceOrderRequest{Items: []ItemRequest{{"", 1}}}, false},
		{"quantity 0", PlaceOrderRequest{Items: []ItemRequest{{"1", 0}}}, false},
		{"quantity 101", PlaceOrderRequest{Items: []ItemRequest{{"1", 101}}}, false},
	}
	for _, tt := range tests {
		if err := tt.req.Validate(); (err == nil) != tt.ok {
			t.Errorf("%s: Validate() = %v, want ok=%v", tt.name, err, tt.ok)
		}
	}
}

func assertAPIError(t *testing.T, w *httptest.ResponseRecorder, status int, typ, msg string) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status = %d, want %d; body %s", w.Code, status, w.Body)
	}
	var e httpx.APIError
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatalf("body %q is not an APIError: %v", w.Body, err)
	}
	if e.Code != status || e.Type != typ || !strings.Contains(e.Message, msg) {
		t.Errorf("APIError = %+v, want code %d type %s message containing %q", e, status, typ, msg)
	}
}

func keys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
