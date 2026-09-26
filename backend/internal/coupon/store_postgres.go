package coupon

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore looks codes up in valid_codes, which cmd/importer fills with
// codes that already passed the min-files rule.
type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) Has(ctx context.Context, code string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM valid_codes WHERE code = $1)`, code).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("look up coupon: %w", err)
	}
	return ok, nil
}
