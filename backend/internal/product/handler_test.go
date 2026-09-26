package product

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shop/internal/httpx"
)

func newTestMux(store Store) *http.ServeMux {
	mux := http.NewServeMux()
	NewHandler(NewService(store)).Register(mux)
	return mux
}

func serve(mux *http.ServeMux, method, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

var testCatalogue = []Product{
	{ID: "1", Name: "Waffle with Berries", Price: 650, Category: "Waffle"},
	{ID: "2", Name: "Classic Tiramisu", Price: 550, Category: "Tiramisu"},
	{ID: "10", Name: "Chicken Waffle", Price: 1299, Category: "Waffle"},
}

func TestHandlerList(t *testing.T) {
	tests := []struct {
		name       string
		store      Store
		wantStatus int
		wantBody   string // exact JSON for success cases
	}{
		{
			name:       "catalogue as JSON array",
			store:      NewMemoryStore(testCatalogue),
			wantStatus: http.StatusOK,
			wantBody: `[{"id":"1","name":"Waffle with Berries","price":6.5,"category":"Waffle"},` +
				`{"id":"2","name":"Classic Tiramisu","price":5.5,"category":"Tiramisu"},` +
				`{"id":"10","name":"Chicken Waffle","price":12.99,"category":"Waffle"}]`,
		},
		{"empty catalogue is [] not null", NewMemoryStore(nil), http.StatusOK, `[]`},
		{"nil slice from store is [] not null", &fakeStore{products: nil}, http.StatusOK, `[]`},
		{"store failure is 500", &fakeStore{err: errDB}, http.StatusInternalServerError, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := serve(newTestMux(tt.store), http.MethodGet, "/api/product")
			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body %s", w.Code, tt.wantStatus, w.Body)
			}
			if !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
				t.Errorf("Content-Type = %q", w.Header().Get("Content-Type"))
			}
			if tt.wantBody != "" {
				if got := strings.TrimSpace(w.Body.String()); got != tt.wantBody {
					t.Errorf("body = %s\nwant   %s", got, tt.wantBody)
				}
				return
			}
			assertAPIError(t, w, tt.wantStatus, httpx.TypeInternal)
			if strings.Contains(w.Body.String(), "db down") {
				t.Error("internal error details leaked to client")
			}
		})
	}
}

func TestHandlerGet(t *testing.T) {
	mux := newTestMux(NewMemoryStore(testCatalogue))
	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantID     string // for 200
		wantType   string // for errors
	}{
		{"found", "/api/product/1", http.StatusOK, "1", ""},
		{"multi-digit ID", "/api/product/10", http.StatusOK, "10", ""},
		{"leading zeros are canonicalized", "/api/product/0001", http.StatusOK, "1", ""},

		{"not found", "/api/product/999", http.StatusNotFound, "", httpx.TypeNotFound},
		{"max int64 not found", "/api/product/9223372036854775807", http.StatusNotFound, "", httpx.TypeNotFound},

		{"letters", "/api/product/abc", http.StatusBadRequest, "", httpx.TypeBadRequest},
		{"zero", "/api/product/0", http.StatusBadRequest, "", httpx.TypeBadRequest},
		{"negative", "/api/product/-1", http.StatusBadRequest, "", httpx.TypeBadRequest},
		{"plus sign", "/api/product/+1", http.StatusBadRequest, "", httpx.TypeBadRequest},
		{"decimal", "/api/product/1.5", http.StatusBadRequest, "", httpx.TypeBadRequest},
		{"overflows int64", "/api/product/9223372036854775808", http.StatusBadRequest, "", httpx.TypeBadRequest},
		{"space", "/api/product/1%202", http.StatusBadRequest, "", httpx.TypeBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := serve(mux, http.MethodGet, tt.path)
			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body %s", w.Code, tt.wantStatus, w.Body)
			}
			if tt.wantType != "" {
				assertAPIError(t, w, tt.wantStatus, tt.wantType)
				return
			}
			var got ProductResponse
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.ID != tt.wantID || got.Name == "" {
				t.Errorf("got %+v, want id %s", got, tt.wantID)
			}
		})
	}
}

func TestHandlerGetStoreFailure(t *testing.T) {
	w := serve(newTestMux(&fakeStore{err: errDB}), http.MethodGet, "/api/product/1")
	assertAPIError(t, w, http.StatusInternalServerError, httpx.TypeInternal)
}

func TestHandlerWrongMethod(t *testing.T) {
	mux := newTestMux(NewMemoryStore(testCatalogue))
	for _, path := range []string{"/api/product", "/api/product/1"} {
		w := serve(mux, http.MethodPost, path)
		assertAPIError(t, w, http.StatusMethodNotAllowed, httpx.TypeMethodNotAllowed)
		if w.Header().Get("Allow") != "GET, HEAD" {
			t.Errorf("%s: Allow = %q", path, w.Header().Get("Allow"))
		}
	}
}

func TestStatusFor(t *testing.T) {
	tests := []struct {
		err  error
		want int
	}{
		{ErrNotFound, http.StatusNotFound},
		{wrappedNotFound(), http.StatusNotFound},
		{errDB, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		if got := statusFor(tt.err); got != tt.want {
			t.Errorf("statusFor(%v) = %d, want %d", tt.err, got, tt.want)
		}
	}
}

// wrappedNotFound returns the wrapped not-found error a real lookup produces.
func wrappedNotFound() error {
	_, err := NewService(NewMemoryStore(nil)).Get(context.Background(), "1")
	return err
}

func TestToProductResponse(t *testing.T) {
	tests := []struct {
		cents int64
		want  float64
	}{{650, 6.5}, {1299, 12.99}, {1, 0.01}, {0, 0}, {100000, 1000}}
	for _, tt := range tests {
		if got := ToResponse(Product{Price: tt.cents}).Price; got != tt.want {
			t.Errorf("%d cents -> %v, want %v", tt.cents, got, tt.want)
		}
	}
}

func assertAPIError(t *testing.T, w *httptest.ResponseRecorder, status int, typ string) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status = %d, want %d", w.Code, status)
	}
	var e httpx.APIError
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatalf("body %q is not an APIError: %v", w.Body, err)
	}
	if e.Code != status || e.Type != typ || e.Message == "" {
		t.Errorf("APIError = %+v, want code %d type %s", e, status, typ)
	}
}
