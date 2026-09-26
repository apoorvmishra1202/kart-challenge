package coupon

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"shop/internal/ingest"
)

const statusName = "coupon_codes"

// Kept in sync with migrations/001_create_coupon_tables.sql.
var schema = []string{
	`CREATE UNLOGGED TABLE IF NOT EXISTS coupon_codes (
		code    TEXT     NOT NULL,
		file_id SMALLINT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS import_status (
		name         TEXT PRIMARY KEY,
		row_count    BIGINT NOT NULL,
		completed_at TIMESTAMPTZ NOT NULL
	)`,
}

// Importer loads the coupon source files into coupon_codes.
type Importer struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

func NewImporter(pool *pgxpool.Pool, log *slog.Logger) *Importer {
	return &Importer{pool: pool, log: log}
}

// Run imports files concurrently, assigning file_id 1..n in order. It is a
// no-op when a previous import completed and its data is still present.
func (im *Importer) Run(ctx context.Context, files []string) error {
	start := time.Now()

	for _, stmt := range schema {
		if _, err := im.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("create schema: %w", err)
		}
	}

	done, err := im.alreadyImported(ctx)
	if err != nil {
		return err
	}
	if done {
		im.log.Info("coupon codes already imported, skipping")
		return nil
	}

	if err := im.reset(ctx); err != nil {
		return err
	}

	counts := make([]int64, len(files))
	g, gctx := errgroup.WithContext(ctx)
	for i, path := range files {
		g.Go(func() error {
			n, err := im.loadFile(gctx, path, int16(i+1))
			counts[i] = n
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}

	var total int64
	for _, n := range counts {
		total += n
	}

	if err := im.buildIndex(ctx); err != nil {
		return err
	}

	_, err = im.pool.Exec(ctx,
		`INSERT INTO import_status (name, row_count, completed_at) VALUES ($1, $2, now())
		 ON CONFLICT (name) DO UPDATE SET row_count = EXCLUDED.row_count, completed_at = EXCLUDED.completed_at`,
		statusName, total)
	if err != nil {
		return fmt.Errorf("record import status: %w", err)
	}

	im.log.Info("import complete", "rows", total, "duration", time.Since(start).Round(time.Millisecond))
	return nil
}

// alreadyImported requires both the status row and data: coupon_codes is
// UNLOGGED, so Postgres empties it after a crash while the status row survives.
func (im *Importer) alreadyImported(ctx context.Context) (bool, error) {
	var done bool
	err := im.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM import_status WHERE name = $1)
		    AND EXISTS (SELECT 1 FROM coupon_codes)`,
		statusName).Scan(&done)
	if err != nil {
		return false, fmt.Errorf("check import status: %w", err)
	}
	return done, nil
}

// reset clears anything left by a partial or stale run.
func (im *Importer) reset(ctx context.Context) error {
	for _, stmt := range []string{
		`DROP INDEX IF EXISTS idx_coupon_code_file`,
		`TRUNCATE coupon_codes`,
	} {
		if _, err := im.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("reset: %w", err)
		}
	}
	if _, err := im.pool.Exec(ctx, `DELETE FROM import_status WHERE name = $1`, statusName); err != nil {
		return fmt.Errorf("reset: %w", err)
	}
	return nil
}

func (im *Importer) loadFile(ctx context.Context, path string, fileID int16) (int64, error) {
	name := filepath.Base(path)
	start := time.Now()
	im.log.Info("loading file", "file", name, "file_id", fileID)

	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", name, err)
	}
	defer f.Close()

	src, err := ingest.NewSource(f, fileID, func(line []byte) bool { return validLength(len(line)) })
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	defer src.Close()

	n, err := im.pool.CopyFrom(ctx, pgx.Identifier{"coupon_codes"}, []string{"code", "file_id"}, src)
	if err != nil {
		return n, fmt.Errorf("copy %s: %w", name, err)
	}

	im.log.Info("loaded file", "file", name, "rows", n, "duration", time.Since(start).Round(time.Millisecond))
	return n, nil
}

// buildIndex runs on a single connection so the SET applies to CREATE INDEX.
func (im *Importer) buildIndex(ctx context.Context) error {
	start := time.Now()
	im.log.Info("building index", "index", "idx_coupon_code_file")

	conn, err := im.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()

	for _, stmt := range []string{
		`SET maintenance_work_mem = '512MB'`,
		`CREATE INDEX idx_coupon_code_file ON coupon_codes (code, file_id)`,
		`ANALYZE coupon_codes`,
		`RESET maintenance_work_mem`,
	} {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("build index: %w", err)
		}
	}

	im.log.Info("index built", "duration", time.Since(start).Round(time.Millisecond))
	return nil
}
