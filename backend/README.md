Backend for the kart challenge. Work in progress.

## Coupon import

A promo code is valid when it is 8–10 characters long **and** appears in at
least 2 of the 3 source files (`couponbase1.gz`, `couponbase2.gz`,
`couponbase3.gz`, ~1 GB and ~100M codes each).

### Requirements

- **Docker memory of at least 4 GB** (Docker Desktop → Settings → Resources).
  The importer holds every encoded code in RAM while it works; peak heap on the
  full data is about 2.65 GB. With less memory the importer is killed
  (exit code 137).
- The real files in `data/` (git-ignored). Small sample files with the same
  names are in `testdata/`.

### Running

```sh
docker compose up --build
```

The `importer` service computes the valid codes, writes them to `valid_codes`,
and exits (about 30 seconds on the full data). `api` starts only after the
import completes successfully. Later starts skip the import.

To try the sample files, run the importer against the compose database:

```sh
docker compose up -d db
DATABASE_URL='postgres://shop:shop@localhost:5432/shop?sslmode=disable' DATA_DIR=./testdata go run ./cmd/importer
```

| Env var | Default | Meaning |
|---|---|---|
| `DATABASE_URL` | (required) | Postgres connection string |
| `DATA_DIR` | `./data` | Folder containing the three `.gz` files |
| `IMPORT_MIN_FILES` | `2` | A code is valid if it appears in at least this many files (1–3) |

### Design

The importer applies both rules once, in Go, and stores only the valid codes:

1. **Read.** The three files are streamed concurrently (`internal/ingest`).
   Lines are trimmed, and lines outside 8–10 characters are skipped. A line of
   valid length with a character outside `A-Z0-9` stops the import, with the
   file and line number in the error.
2. **Encode.** Each code is packed into a `uint64`: base 37, with `0-9` → 1–10
   and `A-Z` → 11–36. Zero is never a digit, so codes of different lengths
   can't collide, and 37¹⁰ < 2⁶⁴. Each file's slice is sized up front from
   the gzip trailer, so it never has to grow.
3. **Sort and dedupe** each file's slice.
4. **Merge.** Walk the three sorted slices together and keep every value found
   in at least `IMPORT_MIN_FILES` of them. A code repeated within one file
   counts once.
5. **Write.** In one transaction: truncate `valid_codes`, `COPY` the decoded
   codes into it, and record the run in `import_status`.

Lookups are a primary-key check, after the length rule is checked in Go:

```sql
SELECT EXISTS (SELECT 1 FROM valid_codes WHERE code = $1)
```

- **Idempotent.** `import_status` records `min_files`. A run is skipped only
  when a completed import used the same value, so changing `IMPORT_MIN_FILES`
  re-imports automatically. The table swap is atomic, so the API never sees a
  half-written `valid_codes`.
- **Upgrading.** Earlier versions stored every row in `coupon_codes`. The
  importer drops that table on startup, reclaiming about 22 GB, and logs that
  it did so. `migrations/002_valid_codes_only.sql` does the same.
- **Byte-order collation.** `valid_codes.code` is `COLLATE "C"`, which is
  correct for ASCII codes and cheaper than a linguistic collation.

### Why the rule is applied at import time

The first version stored every `(code, file_id)` row (313M rows) and checked
"at least 2 files" at query time with `COUNT(DISTINCT file_id)`. Both
approaches were measured on the full data, on the same machine:

| | Store all rows, check at query time | **Store valid codes only (current)** |
|---|---|---|
| Import time | 15m15s (first version), 8m10s (optimized) | **32s** |
| Stages | load 59s, index build 6m06s, VACUUM 1m04s | read + sort + merge 32.0s, COPY 29 ms |
| Rows stored | 313,087,705 | **8** |
| Disk (table + index) | 13 GB + 9.4 GB | **8 kB + 16 kB** |
| Importer peak memory | small (rows streamed to Postgres) | 2.65 GB |
| Lookup | index-only scan, ~1.5–3 ms cold | primary-key lookup, 0.25 ms |
| Changing the min-files rule | no re-import | re-import (~32s) |

Both approaches return the same 8 valid codes: `BIRTHDAY`, `BUYGETON`,
`FIFTYOFF`, `FREEZAAA`, `GNULINUX`, `HAPPYHRS`, `OVER9000` and `SIXTYOFF`.

Storing only valid codes wins on almost every measure. It is about 15× faster
to import than the optimized per-row version, and uses kilobytes of disk
instead of 22 GB. The one thing the per-row design offered was changing the
rule without re-importing. But a re-import now takes about 32 seconds, much
less than the old import itself, so rule changes are handled with
`IMPORT_MIN_FILES` instead. The cost is RAM at import time, hence the 4 GB
Docker requirement.
