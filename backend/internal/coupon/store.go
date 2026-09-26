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

// CountFiles returns how many distinct source files contain code.
func (s *Store) CountFiles(ctx context.Context, code string) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT file_id) FROM coupon_codes WHERE code = $1`, code).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count files for coupon: %w", err)
	}
	return n, nil
}
