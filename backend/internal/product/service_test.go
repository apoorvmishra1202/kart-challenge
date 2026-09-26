package product

import (
	"context"
	"errors"
	"slices"
	"testing"
)

// fakeStore returns canned results and records the IDs it was asked for.
type fakeStore struct {
	products []Product
	product  Product
	err      error
	gotIDs   []string
}

func (f *fakeStore) List(context.Context) ([]Product, error) { return f.products, f.err }

func (f *fakeStore) Get(_ context.Context, id string) (Product, error) {
	f.gotIDs = append(f.gotIDs, id)
	return f.product, f.err
}

var errDB = errors.New("db down")

func TestServiceList(t *testing.T) {
	seed := []Product{{ID: "1", Name: "A", Price: 100}, {ID: "2", Name: "B", Price: 250}}
	tests := []struct {
		name    string
		store   *fakeStore
		want    []Product
		wantErr error
	}{
		{"returns products", &fakeStore{products: seed}, seed, nil},
		{"empty catalogue", &fakeStore{}, nil, nil},
		{"store error is wrapped", &fakeStore{err: errDB}, nil, errDB},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewService(tt.store).List(context.Background())
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestServiceGet(t *testing.T) {
	waffle := Product{ID: "1", Name: "Waffle", Price: 650, Category: "Waffle"}
	tests := []struct {
		name    string
		store   *fakeStore
		want    Product
		wantErr error
	}{
		{"found", &fakeStore{product: waffle}, waffle, nil},
		{"not found keeps ErrNotFound", &fakeStore{err: ErrNotFound}, Product{}, ErrNotFound},
		{"store error is wrapped", &fakeStore{err: errDB}, Product{}, errDB},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewService(tt.store).Get(context.Background(), "1")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
			if !slices.Equal(tt.store.gotIDs, []string{"1"}) {
				t.Errorf("store asked for %v, want [1]", tt.store.gotIDs)
			}
		})
	}
}

func TestMemoryStore(t *testing.T) {
	s := NewMemoryStore([]Product{
		{ID: "10", Name: "Ten"}, {ID: "2", Name: "Two"}, {ID: "abc", Name: "Letters"}, {ID: "1", Name: "One"},
	})

	list, err := s.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, p := range list {
		ids = append(ids, p.ID)
	}
	if want := []string{"1", "2", "10", "abc"}; !slices.Equal(ids, want) {
		t.Errorf("List order = %v, want %v (numeric IDs first, numerically)", ids, want)
	}

	if p, err := s.Get(context.Background(), "2"); err != nil || p.Name != "Two" {
		t.Errorf("Get(2) = %+v, %v", p, err)
	}
	if _, err := s.Get(context.Background(), "99"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(99) err = %v, want ErrNotFound", err)
	}

	empty, err := NewMemoryStore(nil).List(context.Background())
	if err != nil || empty == nil || len(empty) != 0 {
		t.Errorf("empty store List = %#v, %v; want non-nil empty slice", empty, err)
	}
}

func TestSeedProducts(t *testing.T) {
	seed := SeedProducts()
	if n := len(seed); n < 5 || n > 10 {
		t.Errorf("seed has %d products, want 5-10", n)
	}
	seen := map[string]bool{}
	for _, p := range seed {
		if _, ok := parseProductID(p.ID); !ok {
			t.Errorf("seed ID %q is not a positive integer, so GET /api/product/{id} could never reach it", p.ID)
		}
		if seen[p.ID] {
			t.Errorf("duplicate seed ID %q", p.ID)
		}
		seen[p.ID] = true
		if p.Name == "" || p.Category == "" || p.Price <= 0 {
			t.Errorf("incomplete seed product %+v", p)
		}
	}
}
