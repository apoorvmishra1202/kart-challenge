package coupon

// Experimental "valid" import mode (IMPORT_MODE=valid).
//
// Instead of storing every (code, file_id) row and checking the MinFiles rule
// at query time, this mode applies the rule in Go and stores only the valid
// codes:
//
//  1. Read the files concurrently, encoding each code into a uint64 (codec.go).
//  2. Sort and dedupe each file's slice.
//  3. Merge the sorted slices; keep values present in >= MinFiles of them.
//  4. COPY the decoded valid codes into valid_codes.
//
// MEMORY: every encoded code is held in RAM at once. The full data set is
// ~313M codes at 8 bytes each, about 2.5 GB, plus Go runtime overhead, so
// this mode needs roughly 3-5 GB of RAM in the container. Docker Desktop's
// memory limit should be at least 6 GB, or the importer is OOM-killed
// (exit code 137).

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/errgroup"

	"shop/internal/ingest"
)

const validStatusName = "valid_codes"

const validSchema = `CREATE TABLE IF NOT EXISTS valid_codes (
	code TEXT COLLATE "C" PRIMARY KEY
)`

func (im *Importer) runValid(ctx context.Context, files []string) error {
	start := time.Now()
	peak := startMemSampler(200 * time.Millisecond)

	if err := im.ensureStatusSchema(ctx); err != nil {
		return err
	}
	if _, err := im.pool.Exec(ctx, validSchema); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}

	var done bool
	err := im.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM import_status WHERE name = $1 AND mode = $2)`,
		validStatusName, string(ModeValid)).Scan(&done)
	if err != nil {
		return fmt.Errorf("check import status: %w", err)
	}
	if done {
		im.log.Info("valid codes already imported, skipping")
		return nil
	}

	// 1-2. Read, encode, sort and dedupe each file concurrently.
	prepStart := time.Now()
	sets := make([][]uint64, len(files))
	g, gctx := errgroup.WithContext(ctx)
	for i, path := range files {
		g.Go(func() error {
			codes, err := im.readSortedFile(gctx, path)
			sets[i] = codes
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}

	// 3. Merge, then drop the per-file slices before the COPY.
	mergeStart := time.Now()
	valid := MergeValid(sets, MinFiles)
	mergeDur := time.Since(mergeStart)
	clear(sets)
	sets = nil
	debug.FreeOSMemory()
	prepDur := time.Since(prepStart)
	im.log.Info("merge complete", "valid_codes", len(valid), "duration", mergeDur.Round(time.Millisecond))

	// 4. Replace valid_codes and record status atomically.
	copyStart := time.Now()
	n, err := im.writeValid(ctx, valid, prepDur)
	if err != nil {
		return err
	}
	copyDur := time.Since(copyStart)

	heap, sys := peak()
	im.log.Info("import complete",
		"mode", ModeValid,
		"valid_codes", n,
		"preprocess", prepDur.Round(time.Millisecond),
		"merge", mergeDur.Round(time.Millisecond),
		"copy", copyDur.Round(time.Millisecond),
		"total", time.Since(start).Round(time.Millisecond),
		"peak_heap_alloc", mib(heap),
		"peak_sys", mib(sys))
	im.logRelationSizes(ctx, "valid_codes", "valid_codes_pkey")
	return nil
}

// readSortedFile streams one file, encodes every code of valid length, and
// returns the sorted, deduplicated values. A code of valid length containing a
// character outside [A-Z0-9] fails the import rather than being skipped.
func (im *Importer) readSortedFile(ctx context.Context, path string) ([]uint64, error) {
	name := filepath.Base(path)
	start := time.Now()

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", name, err)
	}
	defer f.Close()

	src, err := ingest.NewSource(f, 0, func(line []byte) bool { return validLength(len(line)) })
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	defer src.Close()

	codes := make([]uint64, 0, capacityHint(f))
	for src.Scan() {
		v, ok := encode(src.Bytes())
		if !ok {
			return nil, fmt.Errorf("%s line %d: invalid code %q: only A-Z and 0-9 are allowed",
				name, src.LineNumber(), src.Bytes())
		}
		codes = append(codes, v)
		if len(codes)%(1<<20) == 0 && ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	if err := src.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	readDur := time.Since(start)
	kept := len(codes)

	sortStart := time.Now()
	codes = SortUnique(codes)
	im.log.Info("file preprocessed", "file", name,
		"lines", src.LineNumber(), "codes_kept", kept, "unique", len(codes),
		"read_encode", readDur.Round(time.Millisecond),
		"sort_dedupe", time.Since(sortStart).Round(time.Millisecond))
	return codes, nil
}

// capacityHint sizes the per-file slice up front so append never has to grow
// (and briefly double) a ~1 GB backing array. The gzip trailer's ISIZE field
// is the uncompressed size mod 2^32; each kept line takes at least
// MinLength+1 bytes, so ISIZE/(MinLength+1) is an upper bound on kept codes.
// It is only a hint: a wrong value costs memory or a regrow, never correctness.
func capacityHint(f *os.File) int {
	var trailer [4]byte
	info, err := f.Stat()
	if err != nil || info.Size() < int64(len(trailer)) {
		return 0
	}
	if _, err := f.ReadAt(trailer[:], info.Size()-int64(len(trailer))); err != nil {
		return 0
	}
	return int(binary.LittleEndian.Uint32(trailer[:]) / (MinLength + 1))
}

func (im *Importer) writeValid(ctx context.Context, valid []uint64, prepDur time.Duration) (int64, error) {
	tx, err := im.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `TRUNCATE valid_codes`); err != nil {
		return 0, fmt.Errorf("truncate valid_codes: %w", err)
	}
	n, err := tx.CopyFrom(ctx, pgx.Identifier{"valid_codes"}, []string{"code"},
		pgx.CopyFromSlice(len(valid), func(i int) ([]any, error) {
			return []any{Decode(valid[i])}, nil
		}))
	if err != nil {
		return 0, fmt.Errorf("copy valid_codes: %w", err)
	}

	// load_ms holds the Go preprocessing time; index_ms and vacuum_ms don't
	// apply (the primary key is maintained during COPY) and stay NULL.
	if _, err := tx.Exec(ctx, upsertStatus,
		validStatusName, n, prepDur.Milliseconds(), nil, nil, string(ModeValid)); err != nil {
		return 0, fmt.Errorf("record import status: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return n, nil
}

// startMemSampler polls runtime.MemStats until the returned function is
// called, which stops sampling and reports the peak HeapAlloc and Sys.
func startMemSampler(every time.Duration) func() (heap, sys uint64) {
	var (
		mu              sync.Mutex
		maxHeap, maxSys uint64
		stop            = make(chan struct{})
		done            = make(chan struct{})
	)
	sample := func() {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		mu.Lock()
		maxHeap = max(maxHeap, m.HeapAlloc)
		maxSys = max(maxSys, m.Sys)
		mu.Unlock()
	}
	go func() {
		defer close(done)
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				sample()
			}
		}
	}()
	return func() (uint64, uint64) {
		close(stop)
		<-done
		sample()
		mu.Lock()
		defer mu.Unlock()
		return maxHeap, maxSys
	}
}

func mib(b uint64) string { return fmt.Sprintf("%.0f MiB", float64(b)/(1<<20)) }
