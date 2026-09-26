package product

import (
	"context"
	"fmt"
)

// Store is the storage the Service depends on. MemoryStore implements it.
type Store interface {
	List(ctx context.Context) ([]Product, error)
	Get(ctx context.Context, id string) (Product, error)
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

// List returns every product, sorted by ID.
func (s *Service) List(ctx context.Context) ([]Product, error) {
	products, err := s.store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	return products, nil
}

// Get returns the product with the given ID, or an error wrapping
// ErrNotFound.
func (s *Service) Get(ctx context.Context, id string) (Product, error) {
	p, err := s.store.Get(ctx, id)
	if err != nil {
		return Product{}, fmt.Errorf("get product %s: %w", id, err)
	}
	return p, nil
}
