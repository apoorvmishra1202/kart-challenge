package order

import (
	"context"
	"fmt"
	"slices"
	"sync"
)

// MemoryStore keeps orders in a map. It stores copies, so later changes to a
// saved Order's slices by the caller don't affect the stored one.
type MemoryStore struct {
	mu     sync.Mutex
	orders map[string]Order
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{orders: make(map[string]Order)}
}

func (s *MemoryStore) Save(_ context.Context, o Order) error {
	o.Items = slices.Clone(o.Items)
	o.Products = slices.Clone(o.Products)

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.orders[o.ID]; exists {
		return fmt.Errorf("order %s already exists", o.ID)
	}
	s.orders[o.ID] = o
	return nil
}
