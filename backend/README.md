Backend for the kart challenge. Work in progress.

## Coupon import

A promo code is valid when it is 8–10 characters long **and** appears in at
least 2 of the 3 source files (`couponbase1.gz`, `couponbase2.gz`,
`couponbase3.gz`, ~1 GB each).

### Running

Put the real files in `data/` (git-ignored), then:

```sh
docker compose up --build
```

The `importer` service loads the files and exits. `api` starts only after the
import completes successfully. To load the small sample files in `testdata/`
instead, run the importer against the compose database:

```sh
docker compose up -d db
DATABASE_URL=postgres://shop:shop@localhost:5432/shop?sslmode=disable DATA_DIR=./testdata go run ./cmd/importer
```

### Design

- **Streaming.** Each file is decompressed and read line by line
  (`internal/ingest`) and sent straight to Postgres with `COPY`. A whole file is
  never held in memory.
- **Parallel.** The three files load concurrently, one connection each, with
  `file_id` 1, 2 and 3.
- **Filtered at load.** Lines are trimmed. Lines that fail the length rule are
  dropped before reaching the database.
- **Index after load.** `idx_coupon_code_file (code, file_id)` is built once
  after all rows are in. That is much faster than maintaining it during `COPY`.
- **UNLOGGED table.** `coupon_codes` is derived data that can always be rebuilt,
  so it skips the write-ahead log. Postgres empties unlogged tables after a
  crash, so the importer re-imports when `import_status` says "done" but the
  table is empty.
- **Idempotent.** A completed import is skipped on later runs. A partial run is
  cleaned up (index dropped, table truncated) before loading again.

### Why rule 2 is checked at query time

The importer stores a `(code, file_id)` row for every well-formed code instead
of precomputing the set of valid codes. The API then checks validity with:

```sql
SELECT COUNT(DISTINCT file_id) FROM coupon_codes WHERE code = $1
```

- **Simpler, faster load.** Finding codes shared across files at import time
  needs a large join or aggregate over ~300M rows. Plain `COPY` needs neither.
- **Cheap lookups.** With the `(code, file_id)` index, the query reads only the
  few index entries for one code (an index-only scan).
- **Flexible rules.** Changing `MinFiles`, or adding a new source file, needs no
  re-import logic. The raw facts stay in the table and the rule lives in one
  place (`internal/coupon`).
