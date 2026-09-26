package coupon

import "context"

// CodeStore is the storage dependency of Service; *Store implements it.
type CodeStore interface {
	Exists(ctx context.Context, code string) (bool, error)
}

type Service struct {
	store CodeStore
}

func NewService(store CodeStore) *Service {
	return &Service{store: store}
}

// Validate reports whether code is a valid promo code. Malformed codes are
// rejected without touching the database. A non-nil error is a store
// failure, not a verdict on the code.
func (s *Service) Validate(ctx context.Context, code string) (bool, error) {
	if !IsWellFormed(code) {
		return false, nil
	}
	return s.store.Exists(ctx, code)
}
