package product

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"sync"
)

// MemoryStore is an in-memory, concurrency-safe Store.
type MemoryStore struct {
	mu       sync.RWMutex
	products map[string]Product
}

// NewMemoryStore returns a store holding a copy of seed. Later entries win
// if IDs repeat.
func NewMemoryStore(seed []Product) *MemoryStore {
	products := make(map[string]Product, len(seed))
	for _, p := range seed {
		products[p.ID] = p
	}
	return &MemoryStore{products: products}
}

// List returns all products sorted by ID. Numeric IDs sort numerically
// ("2" before "10"); other IDs sort after them, as strings.
func (s *MemoryStore) List(_ context.Context) ([]Product, error) {
	s.mu.RLock()
	out := make([]Product, 0, len(s.products))
	for _, p := range s.products {
		out = append(out, p)
	}
	s.mu.RUnlock()

	slices.SortFunc(out, func(a, b Product) int { return compareIDs(a.ID, b.ID) })
	return out, nil
}

func (s *MemoryStore) Get(_ context.Context, id string) (Product, error) {
	s.mu.RLock()
	p, ok := s.products[id]
	s.mu.RUnlock()
	if !ok {
		return Product{}, fmt.Errorf("%w: id %q", ErrNotFound, id)
	}
	return p, nil
}

func compareIDs(a, b string) int {
	na, errA := strconv.ParseInt(a, 10, 64)
	nb, errB := strconv.ParseInt(b, 10, 64)
	switch {
	case errA == nil && errB == nil:
		return cmp.Compare(na, nb)
	case errA == nil:
		return -1
	case errB == nil:
		return 1
	}
	return cmp.Compare(a, b)
}
