package coupon

import (
	"context"
	"fmt"
)

// Store reports whether a code is a known valid coupon. MemoryStore and
// PostgresStore implement it.
type Store interface {
	Has(ctx context.Context, code string) (bool, error)
}

// TODO: real coupon rules; codes will be loaded from .gz files later.
// (cmd/importer already applies the rules and loads valid_codes from the .gz
// files; the API still needs to be wired to PostgresStore instead of the
// empty MemoryStore.)
type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

// Validate returns nil for a valid code and an error matching ErrInvalid for
// a rejected one. Malformed codes are rejected without querying the store.
// Any other error is a store failure, not a verdict on the code.
func (s *Service) Validate(ctx context.Context, code string) error {
	if !IsWellFormed(code) {
		return invalidError{code}
	}
	ok, err := s.store.Has(ctx, code)
	if err != nil {
		return fmt.Errorf("validate coupon: %w", err)
	}
	if !ok {
		return invalidError{code}
	}
	return nil
}

// invalidError is a rejected code. Besides matching ErrInvalid, it reports
// InvalidCoupon() so callers that must not import this package (order) can
// tell a rejected code from a store failure by behavior alone.
type invalidError struct{ code string }

func (e invalidError) Error() string        { return fmt.Sprintf("%s: %q", ErrInvalid, e.code) }
func (e invalidError) Is(target error) bool { return target == ErrInvalid }
func (e invalidError) InvalidCoupon() bool  { return true }
