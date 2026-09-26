# Code Log

A running record of what was built and changed, and why, so that final
documentation can be written from here instead of from chat history.
Add a new entry at the bottom after every change.

Entry format: date, title, **What**, **Why**, **Files**, plus **Results** and
**Notes** when relevant.

---

## 2026-09-26: Coupon importer, first version (superseded)

**What:** First pass at loading the three coupon files into Postgres through
Docker Compose, written before a detailed spec existed.
- Streamed each `.gz` file into Postgres with `COPY`, loading the files
  concurrently.
- Used a `coupon_files` table (auto-generated IDs) and a logged
  `coupon_codes` table with an index on `code`.

**Why superseded:** A detailed spec arrived afterwards. It differed on the
module name, table names and types, idempotency rules, length filtering,
errgroup, and tests. See the next entry.

**Kept from this version:** Dockerfile, `.dockerignore` (keeps the ~2 GB of
data out of the build), and pgx pinned to v5.7.5 (the latest release needs
Go 1.25; local Go is 1.24).

---

## 2026-09-26: Coupon importer rewritten to the spec

**What**
- Module renamed to `shop`. Dependencies: pgx v5.7.5 (pgxpool) and
  `golang.org/x/sync` v0.13.0 (errgroup), both compatible with Go 1.24.
- **Data layout:** the real ~1 GB files moved to `data/` (git-ignored). Tiny
  sample files with the same names live in `testdata/` and are committed.
- **Schema** (also in `migrations/001_create_coupon_tables.sql`):
  - `coupon_codes (code TEXT, file_id SMALLINT)`, created **UNLOGGED**.
  - `import_status (name PK, row_count, completed_at)`.
  - `idx_coupon_code_file ON coupon_codes (code, file_id)`, created only after
    loading.
- **`internal/ingest/reader.go`:** a generic streaming `pgx.CopyFromSource`
  over gzip. It uses `bufio.Reader.ReadLine`, skips lines longer than the
  64 KB buffer, trims whitespace, and takes a caller-supplied `keep` filter.
  It has no coupon rules.
- **`internal/coupon`:**
  - `coupon.go`: `MinLength=8`, `MaxLength=10`, `MinFiles=2`, `IsWellFormed`.
  - `import.go`: `Importer.Run`. Steps: create schema → skip if done → clean
    up any partial run → load 3 files in parallel (file_id 1, 2, 3) → build
    the index with `maintenance_work_mem=512MB` → `ANALYZE` → write the status
    row.
  - `store.go`: `CountFiles` runs
    `SELECT COUNT(DISTINCT file_id) FROM coupon_codes WHERE code = $1`.
  - `service.go`: `Validate` rejects malformed codes without querying the
    database; a code is valid when `count >= MinFiles`.
- **`internal/database`:** a pgx pool with MaxConns 4 and a startup Ping.
- **`internal/config`:** reads `DATABASE_URL` (required) and `DATA_DIR`
  (default `./data`).
- **`cmd/importer`:** checks that all 3 files exist before connecting, handles
  SIGINT/SIGTERM, and exits with code 1 on error.
- **Docker Compose:** `db` (Postgres 16, `shm_size: 1g`, pg_isready
  healthcheck) → `importer` (mounts `./data:/data:ro`) → `api` (waits for the
  importer with `service_completed_successfully`).
- **Tests:** reader filtering and trimming, over-long lines, non-gzip input;
  `IsWellFormed` table; service tests with a fake store (counts 0–3,
  malformed codes, store errors).
- README section added covering the import design and why rule 2 is checked
  at query time.

**Why (key decisions)**
- **Rule 2 is checked at query time.** Precomputing codes shared across files
  would need a large aggregate over ~300M rows. Storing raw `(code, file_id)`
  rows keeps the load a plain `COPY`, lookups are index-only, and changing
  `MinFiles` needs no re-import.
- **UNLOGGED table.** The data can always be rebuilt from the files, so it
  skips the write-ahead log. Because Postgres empties unlogged tables after a
  crash, the "already imported" check needs both the status row *and* a
  non-empty table.
- **Index after load.** Building the index once is much faster than
  maintaining it during a bulk `COPY`.
- **Length filter applied during load.** Codes that can never be valid are
  never stored.

**Files:** `go.mod`, `go.sum`, `cmd/importer/main.go`, `internal/config/`,
`internal/database/`, `internal/ingest/`, `internal/coupon/`, `migrations/`,
`Dockerfile`, `.dockerignore`, `docker-compose.yml`, `.gitignore`,
`README.md`, `testdata/*.gz`.

**Results:** `go build`, `go vet` and `go test` all pass.

---

## 2026-09-26: Sample data import verified

**What:** Ran the importer locally against `testdata/`, using the compose
database.

```sh
docker compose up -d db
DATABASE_URL=postgres://shop:shop@localhost:5432/shop?sslmode=disable DATA_DIR=./testdata go run ./cmd/importer
```

**Results**
- 12 rows (4 per file). Short and over-long codes were dropped; `\r` and
  surrounding spaces were trimmed.
- Valid (in 2 or more files): `BIRTHDAY10`, `FIFTYOFF`, `HAPPYHRS`,
  `SUPER100`.
- Invalid (in 1 file): `ONLYFILE1`, `ONLYFILE2`, `ONLYFILE3`.
- The query plan showed a full table scan, which is expected for a 12-row
  table.

---

## 2026-09-26: Full data import verified

**What:** Ran the full import through Docker.

```sh
docker compose down -v
docker compose up --build importer
```

**Results**

| Step | Rows | Duration |
|---|---|---|
| couponbase1.gz | 107,260,777 | 2m18s |
| couponbase2.gz | 107,260,776 | 2m18s |
| couponbase3.gz | 98,566,152 | 2m16s |
| Index build | – | 12m57s |
| **Total** | **313,087,705** | **15m15s** |

- The three files load in parallel, so the load phase takes about 2m18s in
  total.
- Storage: table 13 GB, index 9.4 GB (about 22 GB of Docker volume).
- A validation lookup is an **index-only scan** on `idx_coupon_code_file`:
  0 table reads, about 1.5 ms with a cold cache.
- Sample lookups (number of files the code appears in):
  - `HAPPYHRS` 2, `FIFTYOFF` 3, `BIRTHDAY` 3, `OVER9000` 3: valid.
  - `SUPER100` 1: invalid.
  - `BUYGETONE` 0: not found.
- Re-running `docker compose up importer` skips the load. Only
  `docker compose down -v` forces a full re-import.

**Notes**
- File 3 has fewer rows because the file has fewer lines, not because of
  filtering: 8 of its codes are 8 characters and 98,566,144 are 10 characters,
  so nothing was dropped.
- All codes in file 1 are 8 characters long.

---

## 2026-09-26: Importer v1 committed and pushed

Commit `ff0514b feat(backend): coupon code importer v1` on `dev`, containing
everything above. `.DS_Store` was added to `backend/.gitignore`.

---

## 2026-09-26: Importer optimization

**What**
- **Byte-order collation:** the column is now `code TEXT COLLATE "C" NOT NULL`,
  in both the importer schema and `migrations/001_create_coupon_tables.sql`.
  - On startup the importer checks the collation of `coupon_codes.code`
    (`pg_attribute` joined with `pg_collation`). If the table exists without
    `"C"`, it logs a warning, drops the table, and deletes the `import_status`
    row, so an old v1 volume is rebuilt automatically.
- **Faster index build:** on the single index connection it runs
  `SET maintenance_work_mem` and `SET max_parallel_maintenance_workers`, then
  `RESET`s both afterwards.
  - Values come from env `IMPORT_MAINTENANCE_WORK_MEM` (default `1GB`) and
    `IMPORT_PARALLEL_WORKERS` (default `4`), also set in `docker-compose.yml`.
  - Both are interpolated into SQL, so `coupon.ParseIndexOptions` validates
    them first. Memory must match `^[0-9]+(kB|MB|GB)$`; workers must be an
    integer from 0 to 16. The importer fails before connecting otherwise.
- **Index-only scans:** `ANALYZE` was replaced with `VACUUM (ANALYZE)` after
  the index build, as a plain `Exec` since VACUUM can't run in a transaction.
- **Phase timings:** the importer logs load (per file and total), index build,
  vacuum/analyze, and total durations, then table and index sizes.
  - `import_status` gained `load_ms`, `index_ms` and `vacuum_ms` columns via
    `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`.
- `NewImporter` now takes an `IndexOptions` argument.

**Why**
- **"C" collation:** codes are ASCII uppercase letters and digits, so byte
  order sorts them correctly. The default linguistic collation makes every
  comparison in the index sort, and every lookup, more expensive.
- **More memory and parallel workers:** the index build was 85% of import
  time (12m57s of 15m15s).
- **VACUUM:** it sets the visibility map. Without it, index-only scans may
  still fetch table rows until autovacuum happens to run.

**Files:** `internal/coupon/import.go`, `internal/coupon/import_test.go` (new),
`cmd/importer/main.go`, `migrations/001_create_coupon_tables.sql`,
`docker-compose.yml`.

**Results (throwaway Postgres on :5433, sample data)**
- `go build`, `go vet` and `go test` pass. The new table test covers valid
  values, out-of-range workers, missing or lowercase units, and SQL-injection
  strings.
- **Old schema detected:** with a v1 table (default collation) and status row
  already present, the importer logged `collation=default`, dropped the table,
  and re-imported 12 rows. The old row was gone.
- The rebuilt column reports `collation_name = C`. The status row has
  `load_ms`, `index_ms` and `vacuum_ms` filled in.
- A second run skipped the import.
- A bad `IMPORT_MAINTENANCE_WORK_MEM` or `IMPORT_PARALLEL_WORKERS=99` exits 1
  with a clear message.
- A lookup is an index-only scan with `Heap Fetches: 0`.
- Full-data results are in the next entry.

**Notes:** Migration 001 was edited in place rather than adding a new
migration file. There is no migration runner; the importer creates the schema
itself.

---

## 2026-09-26: Optimized full import verified

**What:** Ran `docker compose down -v`, then `docker compose up --build`.
- The first attempt was cut off when Docker Desktop was quit from its app
  during the index build (the Docker log shows `POST /app/quit`, a clean
  shutdown, not a crash). The importer exited with code 1.
- It was resumed with `docker compose up importer` on the same volume. There
  was no status row, so the importer truncated the partial table and reloaded
  it. The final count is exactly 313,087,705, so partial-run cleanup works on
  real data.

**Results (v1 → optimized)**

| Phase | v1 | Optimized |
|---|---|---|
| Load (3 files in parallel) | 2m18s | 59s |
| Index build | 12m57s | 6m06s |
| VACUUM (ANALYZE) | – (plain ANALYZE, not timed) | 1m04s |
| **Total** | **15m15s** | **8m10s** |
| Table size | 13 GB | 13 GB |
| Index size | 9.4 GB | 9.4 GB (9418 MB) |
| Cold lookup (index-only scan) | 1.45 ms | 2.9 ms, `Heap Fetches: 0` |

- `import_status`: `load_ms=58885`, `index_ms=366465`, `vacuum_ms=64148`.
  The column's `collation_name` is `C`.
- Lookups: `HAPPYHRS` 2, `FIFTYOFF` 3, `SUPER100` 1, the same as v1.

**Notes**
- **Index build (2.1× faster)** is the direct result of the "C" collation,
  1 GB `maintenance_work_mem`, and 4 parallel workers.
- **Load time** varied from run to run (2m18s in v1, 1m33s on the interrupted
  run, 59s here) even though the load code barely changed. The likely cause is
  the OS file cache and less competition for the machine, so it isn't claimed
  as an optimization win.
- **Cold lookup timings** of 1–3 ms are single cold reads of 4–6 buffers and
  are noise at this scale. What matters is that the plan is an index-only scan
  with 0 heap fetches.
- **Disk and laptop:** Docker's disk file is about 31 GB and the Mac had 24 GB
  free. Loading and indexing make the laptop sluggish, so after measuring we
  stop the containers (`docker compose stop`) and quit Docker Desktop. The
  data stays in the `pgdata` volume.

---

## 2026-09-26: Optimization committed and pushed

Commit `5e3c737 perf(backend): speed up coupon import index build` on `dev`.

---

## 2026-09-26: Experimental "valid" import mode

**What**
- **Mode switch:** new env `IMPORT_MODE` accepts `all` (default, unchanged
  behavior) or `valid`. It is passed through `docker-compose.yml` as
  `${IMPORT_MODE:-all}`, and `coupon.ParseMode` rejects any other value.
- **`import_status.mode`:** new `TEXT` column, added with `ADD COLUMN IF NOT
  EXISTS`.
  - Each mode has its own status row: `coupon_codes`/`all` and
    `valid_codes`/`valid`. The skip check matches both name and mode.
  - A `NULL` mode (rows written before this column existed) is treated as
    `all`, so the existing full import is still skipped.
- **"valid" mode** (`internal/coupon/import_valid.go`):
  1. Reads the 3 files concurrently (errgroup) with the ingest reader.
  2. Encodes each code of valid length to a `uint64`, failing on any
     character outside `[A-Z0-9]` with the file, line number and line.
  3. Runs `slices.Sort` and `slices.Compact` on each file's slice.
  4. Merges the three slices, keeping values found in at least `MinFiles`
     files, then frees the per-file slices.
  5. In one transaction: `TRUNCATE valid_codes`, `COPY` the decoded codes,
     and upsert the status row.
- **Encoding** (`internal/coupon/codec.go`): base 37, with '0'-'9' → 1-10
  and 'A'-'Z' → 11-36. No digit is 0, so codes of different lengths never
  collide, and 37^10 < 2^64. Within one length, numeric order equals byte
  order.
  - `MergeValid` also skips repeated values within a file, so duplicates
    count once even if a caller skips deduplication.
- **Slice sizing:** each per-file slice is sized up front from the gzip
  trailer (uncompressed size ÷ 9 is an upper bound on codes). This avoids
  append temporarily doubling a ~1 GB array while it grows.
- **Ingest reader:** added `Scan`, `Bytes` and `LineNumber`, which read
  without allocating. `Next` is now built on `Scan`, with unchanged behavior.
- **Logged measurements:**
  - Per file: lines, codes kept, unique codes, read+encode time, sort+dedupe
    time.
  - Overall: merge time, valid count, COPY time, total time.
  - Peak `HeapAlloc` and `Sys` from sampling `runtime.MemStats` every 200 ms.
  - Sizes of `valid_codes` and `valid_codes_pkey`.
  - `load_ms` stores the Go preprocessing time. `index_ms` and `vacuum_ms` are
    NULL in this mode.
- **Other:** `logSizes` became `logRelationSizes(table, index)`, and migration
  001 gained the `mode` column and the `valid_codes` table.

**Why:** An experiment to compare against "all" mode. It stores a small table
of valid codes instead of 313M rows plus a 9.4 GB index, at the cost of RAM
at import time and of fixing `MinFiles` at import time.

**Files:** `internal/coupon/codec.go`, `codec_test.go`, `import_valid.go`,
`import_valid_test.go` (all new); `internal/coupon/import.go`,
`internal/ingest/reader.go`, `reader_test.go`, `cmd/importer/main.go`,
`docker-compose.yml`, `migrations/001_create_coupon_tables.sql`.

**Results**
- `go build`, `go vet` and `go test -race` pass.
- New tests cover:
  - Encode/decode round trips for lengths 8, 9 and 10, with no collisions
    across lengths and order matching byte order.
  - Rejection of wrong lengths and bad characters.
  - Merge with codes in 1, 2 and 3 files, and duplicates within a file
    counting once.
  - Reading a file with trimming and dedupe, and failing on a bad character
    with its line number.
  - `ParseMode`, and line numbers from `Scan`.
- **Throwaway Postgres (:5433), sample data:**
  - `valid_codes` = `BIRTHDAY10`, `FIFTYOFF`, `HAPPYHRS`, `SUPER100`, matching
    "all" mode.
  - A second run of each mode skipped, including an "all" status row with
    `mode = NULL`.
  - `coupon_codes` was untouched, and `IMPORT_MODE=both` exits 1.
- **Real files scanned with awk:** 0 codes of valid length contain
  characters outside `[A-Z0-9]`.
- **Environment:** Docker Desktop's VM has 3.8 GiB of memory; the Mac has
  8 GiB.

**Notes:** "valid" mode needs roughly 3-5 GB of RAM. That is more than the
3.8 GiB Docker VM, which Postgres also shares, so on this laptop run the
importer natively (`go run`) against the compose database, or raise Docker's
memory limit to 6 GB or more.

---

## 2026-09-26: "valid" mode on full data, compared with "all"

**What:** Ran the importer natively on the Mac, since Docker's VM has only
3.8 GiB, against the compose database that already held the "all" import.

```sh
docker compose up -d db
IMPORT_MODE=valid DATABASE_URL='postgres://shop:shop@localhost:5432/shop?sslmode=disable' DATA_DIR=./data go run ./cmd/importer
```

(The first attempt failed because Docker Desktop was not running, and
because zsh treated a pasted `# comment` as arguments. Don't put inline
comments in commands meant to be pasted.)

**Per file (read and sort run concurrently across files)**

| File | Lines | Unique | Duplicates in file | Read+encode | Sort+dedupe |
|---|---|---|---|---|---|
| couponbase1.gz | 107,260,777 | 107,258,700 | 2,077 | 15.3s | 12.5s |
| couponbase2.gz | 107,260,776 | 107,260,726 | 50 | 16.5s | 12.1s |
| couponbase3.gz | 98,566,152 | 98,566,151 | 1 | 16.4s | 11.4s |

**"all" vs "valid"**

| | all (optimized) | valid |
|---|---|---|
| Total import time | 8m10s | **32s** |
| Stages | load 59s, index 6m06s, vacuum 1m04s | preprocess 32.0s (incl. merge 3.2s), COPY 29 ms |
| Rows stored | 313,087,705 | **8** |
| Table + index | 13 GB + 9.4 GB | 8 kB + 16 kB |
| Peak importer memory | small (streams rows) | 2,653 MiB heap; 2.52 GB peak footprint (`/usr/bin/time -l`) |
| Lookup | index-only scan, ~1.5-3 ms cold | index-only scan, 0.25 ms (warm) |
| Changing `MinFiles` | no re-import | needs re-import |

**Correctness check:** the full-table query in "all" mode
(`GROUP BY code HAVING COUNT(DISTINCT file_id) >= 2`, 1m27s) returns exactly
the same 8 codes as `valid_codes`: `BIRTHDAY`, `BUYGETON`, `FIFTYOFF`,
`FREEZAAA`, `GNULINUX`, `HAPPYHRS`, `OVER9000`, `SIXTYOFF`.
- `HAPPYHRS` appears in 2 files; the other 7 appear in all 3.
- All 8 are 8 characters long. `SUPER100` is in only 1 file, so it's invalid.

**Notes**
- `coupon_codes` was untouched (still 313,087,705 rows). Its status row
  (`mode` NULL, treated as "all") survived, so the two modes coexist.
- The real data has very little overlap between files. File 1 is all
  8-character codes and file 3 is almost all 10-character codes, so only 8
  codes pass rule 2.
- Peak memory came in under the 3-5 GB estimate, because the slices are
  sized up front from the gzip trailer and never grow.
- The `valid_codes` lookup showed `Heap Fetches: 1` because the table hasn't
  been vacuumed yet, which doesn't matter for 8 rows.
- The design decision is still open. "valid" is far cheaper in time and
  disk; "all" keeps `MinFiles` changeable without a re-import.

---

## 2026-09-26: Experiment committed

Commit `f1a2d1f feat(backend): experimental "valid" import mode` on `dev`
(local; not pushed at the time of writing).

---

## 2026-09-26: "valid" mode becomes the design; "all" mode removed

**What**
- **Importer** (`internal/coupon/import.go`, rewritten):
  - Always runs the encode → sort → merge pipeline into `valid_codes`.
  - Removed: `IMPORT_MODE`, the per-row `COPY` into `coupon_codes`, the index
    build, VACUUM, `IMPORT_MAINTENANCE_WORK_MEM`, `IMPORT_PARALLEL_WORKERS`,
    `IndexOptions`, `Mode`, and the collation-migration code.
    `import_valid.go` was merged into `import.go`.
- **`IMPORT_MIN_FILES`** (default 2, must be 1-3): parsed by
  `coupon.ParseMinFiles` and stored in `import_status.min_files`.
  - A run is skipped only if a completed import used the same `min_files`.
  - A different value, or an older row with `min_files` NULL, re-imports
    automatically.
  - `MinFiles` became `DefaultMinFiles`; `SourceFiles = 3` is the upper bound.
- **Legacy cleanup:** on startup, if `coupon_codes` exists, the importer drops
  it (`DROP TABLE IF EXISTS`), deletes its status row, and logs
  "dropped legacy coupon_codes table to reclaim disk space".
  - `import_status` loses `index_ms`, `vacuum_ms` and `mode` (via
    `DROP COLUMN IF EXISTS`) and gains `min_files`.
- **Migrations:** added `002_valid_codes_only.sql` (drops `coupon_codes` and
  its status row, drops the unused status columns, adds `min_files`).
  - `001` was left unchanged: it's already pushed and there's no migration
    runner, so a new migration is cleaner than rewriting history.
- **Lookup:** `Store.CountFiles` became `Store.Exists`, running
  `SELECT EXISTS (SELECT 1 FROM valid_codes WHERE code = $1)`.
  - `Service.Validate` still rejects malformed codes before querying.
  - The `FileCounter` interface became `CodeStore`.
- **Compose:** removed `shm_size` (only parallel index builds needed it) and
  the three unused importer env vars; added `IMPORT_MIN_FILES`.
- **README:** rewritten with requirements (Docker memory >= 4 GB), env vars,
  the final design, the measured comparison table, and why this design was
  chosen.

**Why:** On the full data it took 32s instead of 8m10s and uses 8 kB + 16 kB
instead of 13 GB + 9.4 GB, with identical results. The only advantage of the
old design (changing the rule without a re-import) is now covered by
`IMPORT_MIN_FILES`, since a re-import takes about 32s.

**Files:** `internal/coupon/{coupon,import,store,service}.go`,
`internal/coupon/{coupon,codec,import}_test.go`, `cmd/importer/main.go`,
`docker-compose.yml`, `migrations/002_valid_codes_only.sql` (new),
`README.md`. Removed: `internal/coupon/import_valid.go`,
`import_valid_test.go`.

**Results**
- `go build`, `go vet` and `go test -race` pass.
- New tests: `ParseMinFiles` (accepts 1-3 and empty; rejects 0, 4, -1,
  "two", " 2", "2.0"), `MergeValid` with `minFiles` 1, 2 and 3, and service
  tests using a fake `Exists` store.
- **End to end on a throwaway Postgres (:5433), sample data**, seeded with the
  legacy layout (`coupon_codes` table, a status row with
  `mode`/`index_ms`/`vacuum_ms`, and a stale `valid_codes` row):
  1. Dropped `coupon_codes` and logged it; imported 4 valid codes and replaced
     the stale one.
  2. Same settings again: skipped.
  3. `IMPORT_MIN_FILES=3`: 1 code. `=1`: 7 codes. Back to `2`: 4 codes.
     Each change re-imported.
  4. `IMPORT_MIN_FILES=4`: exits 1 with a clear error.
  5. Final state: no `coupon_codes`, one status row with `min_files = 2`, and
     `valid_codes` = `BIRTHDAY10`, `FIFTYOFF`, `HAPPYHRS`, `SUPER100`.
- Not yet run on the real Docker volume, which still holds the ~22 GB
  `coupon_codes` table.

---

## Open items

- Verify end to end on the real volume: the importer should drop
  `coupon_codes`, the Docker disk usage should shrink, and `valid_codes`
  should hold 8 codes.
- Docker Desktop's VM has 3.8 GiB; the README requires 4 GB or more. Raise it
  before running the importer in Docker, or run it natively with `go run`.
- `cmd/api` is still an empty stub; the `api` container exits immediately.
  The next step is to wire up `coupon.NewService(coupon.NewStore(pool)).Validate`.
- Still planned: `scripts/stats.sql` + `make stats`, `make verify`, and an
  integration test.
