package coupon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"shop/internal/ingest"
)

const statusName = "coupon_codes"

// Kept in sync with migrations/001_create_coupon_tables.sql. coupon_codes is
// created separately, after ensureByteOrderCollation has run.
var statusSchema = []string{
	`CREATE TABLE IF NOT EXISTS import_status (
		name         TEXT PRIMARY KEY,
		row_count    BIGINT NOT NULL,
		completed_at TIMESTAMPTZ NOT NULL
	)`,
	`ALTER TABLE import_status ADD COLUMN IF NOT EXISTS load_ms   BIGINT`,
	`ALTER TABLE import_status ADD COLUMN IF NOT EXISTS index_ms  BIGINT`,
	`ALTER TABLE import_status ADD COLUMN IF NOT EXISTS vacuum_ms BIGINT`,
	`ALTER TABLE import_status ADD COLUMN IF NOT EXISTS mode      TEXT`,
}

// Codes are ASCII uppercase letters and digits, so byte order ("C") sorts
// them correctly and is much cheaper than a linguistic collation.
const codesSchema = `CREATE UNLOGGED TABLE IF NOT EXISTS coupon_codes (
	code    TEXT COLLATE "C" NOT NULL,
	file_id SMALLINT NOT NULL
)`

const (
	DefaultMaintenanceWorkMem = "1GB"
	DefaultParallelWorkers    = 4
	maxParallelWorkers        = 16
)

var workMemPattern = regexp.MustCompile(`^[0-9]+(kB|MB|GB)$`)

// IndexOptions tune the index build. The values are interpolated into SET
// statements (which cannot take bind parameters), so they must be validated.
type IndexOptions struct {
	MaintenanceWorkMem string
	ParallelWorkers    int
}

// ParseIndexOptions builds IndexOptions from raw env values; empty strings
// select the defaults.
func ParseIndexOptions(workMem, workers string) (IndexOptions, error) {
	opts := IndexOptions{MaintenanceWorkMem: DefaultMaintenanceWorkMem, ParallelWorkers: DefaultParallelWorkers}
	if workMem != "" {
		opts.MaintenanceWorkMem = workMem
	}
	if workers != "" {
		n, err := strconv.Atoi(workers)
		if err != nil {
			return IndexOptions{}, fmt.Errorf("IMPORT_PARALLEL_WORKERS must be an integer from 0 to %d, got %q", maxParallelWorkers, workers)
		}
		opts.ParallelWorkers = n
	}
	return opts, opts.Validate()
}

func (o IndexOptions) Validate() error {
	if !workMemPattern.MatchString(o.MaintenanceWorkMem) {
		return fmt.Errorf("IMPORT_MAINTENANCE_WORK_MEM must look like 512MB or 1GB (units kB, MB, GB), got %q", o.MaintenanceWorkMem)
	}
	if o.ParallelWorkers < 0 || o.ParallelWorkers > maxParallelWorkers {
		return fmt.Errorf("IMPORT_PARALLEL_WORKERS must be an integer from 0 to %d, got %d", maxParallelWorkers, o.ParallelWorkers)
	}
	return nil
}

// Mode selects what the importer stores.
type Mode string

const (
	// ModeAll stores every well-formed (code, file_id) row in coupon_codes;
	// the "at least MinFiles" rule is checked at query time.
	ModeAll Mode = "all"
	// ModeValid (experimental) applies the rule in Go and stores only the
	// valid codes in valid_codes. See runValid.
	ModeValid Mode = "valid"
)

// ParseMode reads IMPORT_MODE; empty selects ModeAll.
func ParseMode(s string) (Mode, error) {
	switch Mode(s) {
	case "", ModeAll:
		return ModeAll, nil
	case ModeValid:
		return ModeValid, nil
	}
	return "", fmt.Errorf("IMPORT_MODE must be %q or %q, got %q", ModeAll, ModeValid, s)
}

// Importer loads the coupon source files into Postgres.
type Importer struct {
	pool *pgxpool.Pool
	log  *slog.Logger
	mode Mode
	opts IndexOptions
}

func NewImporter(pool *pgxpool.Pool, log *slog.Logger, mode Mode, opts IndexOptions) *Importer {
	return &Importer{pool: pool, log: log, mode: mode, opts: opts}
}

type phaseTimings struct {
	load, index, vacuum time.Duration
}

// Run imports files concurrently, assigning file_id 1..n in order. It is a
// no-op when a previous import in the same mode completed and its data is
// still present. Each mode has its own status row and table, so running one
// mode never invalidates the other's data.
func (im *Importer) Run(ctx context.Context, files []string) error {
	if im.mode == ModeValid {
		return im.runValid(ctx, files)
	}
	if err := im.opts.Validate(); err != nil {
		return err
	}
	start := time.Now()

	if err := im.ensureSchema(ctx); err != nil {
		return err
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

	var t phaseTimings
	loadStart := time.Now()
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
	t.load = time.Since(loadStart)

	var total int64
	for _, n := range counts {
		total += n
	}
	im.log.Info("load complete", "rows", total, "duration", t.load.Round(time.Millisecond))

	if t.index, t.vacuum, err = im.buildIndex(ctx); err != nil {
		return err
	}

	_, err = im.pool.Exec(ctx,
		upsertStatus,
		statusName, total, t.load.Milliseconds(), t.index.Milliseconds(), t.vacuum.Milliseconds(), string(ModeAll))
	if err != nil {
		return fmt.Errorf("record import status: %w", err)
	}

	im.log.Info("import complete",
		"rows", total,
		"load", t.load.Round(time.Millisecond),
		"index", t.index.Round(time.Millisecond),
		"vacuum_analyze", t.vacuum.Round(time.Millisecond),
		"total", time.Since(start).Round(time.Millisecond))
	im.logRelationSizes(ctx, "coupon_codes", "idx_coupon_code_file")
	return nil
}

const upsertStatus = `INSERT INTO import_status (name, row_count, completed_at, load_ms, index_ms, vacuum_ms, mode)
	 VALUES ($1, $2, now(), $3, $4, $5, $6)
	 ON CONFLICT (name) DO UPDATE SET
	   row_count = EXCLUDED.row_count, completed_at = EXCLUDED.completed_at,
	   load_ms = EXCLUDED.load_ms, index_ms = EXCLUDED.index_ms, vacuum_ms = EXCLUDED.vacuum_ms,
	   mode = EXCLUDED.mode`

func (im *Importer) ensureStatusSchema(ctx context.Context) error {
	for _, stmt := range statusSchema {
		if _, err := im.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("create schema: %w", err)
		}
	}
	return nil
}

func (im *Importer) ensureSchema(ctx context.Context) error {
	if err := im.ensureStatusSchema(ctx); err != nil {
		return err
	}
	if err := im.ensureByteOrderCollation(ctx); err != nil {
		return err
	}
	if _, err := im.pool.Exec(ctx, codesSchema); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	return nil
}

// ensureByteOrderCollation drops a coupon_codes table left by an older schema
// whose code column is not COLLATE "C", so an existing volume is rebuilt
// instead of silently keeping the slower schema.
func (im *Importer) ensureByteOrderCollation(ctx context.Context) error {
	var collation string
	err := im.pool.QueryRow(ctx,
		`SELECT coalesce(c.collname, '')
		   FROM pg_attribute a
		   LEFT JOIN pg_collation c ON c.oid = a.attcollation
		  WHERE a.attrelid = to_regclass('coupon_codes')
		    AND a.attname = 'code' AND NOT a.attisdropped`).Scan(&collation)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // table does not exist yet
	}
	if err != nil {
		return fmt.Errorf("check coupon_codes collation: %w", err)
	}
	if collation == "C" {
		return nil
	}

	im.log.Warn("coupon_codes uses a non-C collation, dropping it for a rebuild", "collation", collation)
	if _, err := im.pool.Exec(ctx, `DROP TABLE coupon_codes`); err != nil {
		return fmt.Errorf("drop old coupon_codes: %w", err)
	}
	if _, err := im.pool.Exec(ctx, `DELETE FROM import_status WHERE name = $1`, statusName); err != nil {
		return fmt.Errorf("clear import status: %w", err)
	}
	return nil
}

// alreadyImported requires a status row for this mode and data: coupon_codes
// is UNLOGGED, so Postgres empties it after a crash while the status row
// survives. Rows written before the mode column existed (mode NULL) came from
// ModeAll.
func (im *Importer) alreadyImported(ctx context.Context) (bool, error) {
	var done bool
	err := im.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM import_status WHERE name = $1 AND coalesce(mode, 'all') = $2)
		    AND EXISTS (SELECT 1 FROM coupon_codes)`,
		statusName, string(ModeAll)).Scan(&done)
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

// buildIndex runs on a single connection so the SET values apply to CREATE
// INDEX and VACUUM. VACUUM sets the visibility map, which is what lets lookups
// use index-only scans without heap fetches; it cannot run in a transaction,
// so it is a plain Exec.
func (im *Importer) buildIndex(ctx context.Context) (indexDur, vacuumDur time.Duration, err error) {
	conn, err := im.pool.Acquire(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()

	// Values were validated by IndexOptions.Validate.
	settings := []string{
		fmt.Sprintf(`SET maintenance_work_mem = '%s'`, im.opts.MaintenanceWorkMem),
		fmt.Sprintf(`SET max_parallel_maintenance_workers = %d`, im.opts.ParallelWorkers),
	}
	for _, stmt := range settings {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			return 0, 0, fmt.Errorf("configure index build: %w", err)
		}
	}

	im.log.Info("building index", "index", "idx_coupon_code_file",
		"maintenance_work_mem", im.opts.MaintenanceWorkMem, "parallel_workers", im.opts.ParallelWorkers)
	start := time.Now()
	if _, err := conn.Exec(ctx, `CREATE INDEX idx_coupon_code_file ON coupon_codes (code, file_id)`); err != nil {
		return 0, 0, fmt.Errorf("create index: %w", err)
	}
	indexDur = time.Since(start)
	im.log.Info("index built", "duration", indexDur.Round(time.Millisecond))

	im.log.Info("vacuum analyze", "table", "coupon_codes")
	start = time.Now()
	if _, err := conn.Exec(ctx, `VACUUM (ANALYZE) coupon_codes`); err != nil {
		return indexDur, 0, fmt.Errorf("vacuum analyze: %w", err)
	}
	vacuumDur = time.Since(start)
	im.log.Info("vacuum analyze done", "duration", vacuumDur.Round(time.Millisecond))

	for _, stmt := range []string{`RESET maintenance_work_mem`, `RESET max_parallel_maintenance_workers`} {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			return indexDur, vacuumDur, fmt.Errorf("reset settings: %w", err)
		}
	}
	return indexDur, vacuumDur, nil
}

// logRelationSizes is informational only, so a failure is logged, not returned.
func (im *Importer) logRelationSizes(ctx context.Context, table, index string) {
	var tableSize, indexSize string
	err := im.pool.QueryRow(ctx,
		`SELECT pg_size_pretty(pg_relation_size($1::regclass)), pg_size_pretty(pg_relation_size($2::regclass))`,
		table, index).Scan(&tableSize, &indexSize)
	if err != nil {
		im.log.Warn("could not read relation sizes", "err", err)
		return
	}
	im.log.Info("storage", "table", table, "table_size", tableSize, "index", index, "index_size", indexSize)
}
