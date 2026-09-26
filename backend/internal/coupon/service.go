package coupon

import "context"

// FileCounter is the storage dependency of Service; *Store implements it.
type FileCounter interface {
	CountFiles(ctx context.Context, code string) (int, error)
}

type Service struct {
	store FileCounter
}

func NewService(store FileCounter) *Service {
	return &Service{store: store}
}

// Validate reports whether code is a valid promo code. Malformed codes are
// rejected without touching the database.
func (s *Service) Validate(ctx context.Context, code string) (bool, error) {
	if !IsWellFormed(code) {
		return false, nil
	}
	n, err := s.store.CountFiles(ctx, code)
	if err != nil {
		return false, err
	}
	return n >= MinFiles, nil
}
