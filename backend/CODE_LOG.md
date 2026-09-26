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

## Open items

- `cmd/api` is still an empty stub; the `api` container exits immediately.
  The next step is to wire up `coupon.NewService(coupon.NewStore(pool)).Validate`.
- Importer v1 (everything above) committed and pushed to `dev` on
  2026-09-26. Next: optimization pass (C collation, parallel index build,
  VACUUM for index-only scans, phase timings, stats and verify tooling).
