-- Coupon codes are derived from the source files and can always be rebuilt,
-- so the table is UNLOGGED (no WAL; emptied by Postgres after a crash).
-- Codes are ASCII uppercase letters and digits, so byte-order collation ("C")
-- sorts them correctly and makes comparisons and index builds much faster.
CREATE UNLOGGED TABLE IF NOT EXISTS coupon_codes (
    code    TEXT COLLATE "C" NOT NULL,
    file_id SMALLINT NOT NULL
);

CREATE TABLE IF NOT EXISTS import_status (
    name         TEXT PRIMARY KEY,
    row_count    BIGINT NOT NULL,
    completed_at TIMESTAMPTZ NOT NULL
);

-- Phase timings, added after the first version; ADD COLUMN IF NOT EXISTS
-- migrates existing databases.
ALTER TABLE import_status ADD COLUMN IF NOT EXISTS load_ms   BIGINT;
ALTER TABLE import_status ADD COLUMN IF NOT EXISTS index_ms  BIGINT;
ALTER TABLE import_status ADD COLUMN IF NOT EXISTS vacuum_ms BIGINT;
-- Import mode that wrote the row ('all' or 'valid'); NULL means 'all'.
ALTER TABLE import_status ADD COLUMN IF NOT EXISTS mode      TEXT;

-- Experimental IMPORT_MODE=valid: only codes found in >= 2 files, computed by
-- the importer in Go.
CREATE TABLE IF NOT EXISTS valid_codes (
    code TEXT COLLATE "C" PRIMARY KEY
);

-- The importer drops this before loading and recreates it afterwards, since
-- building an index once is much faster than maintaining it during COPY.
CREATE INDEX IF NOT EXISTS idx_coupon_code_file ON coupon_codes (code, file_id);
