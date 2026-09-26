package coupon

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Exists reports whether code is in valid_codes, which the importer fills
// with codes that already passed the min-files rule.
func (s *Store) Exists(ctx context.Context, code string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM valid_codes WHERE code = $1)`, code).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("look up coupon: %w", err)
	}
	return ok, nil
}
