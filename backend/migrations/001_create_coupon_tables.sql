-- Coupon codes are derived from the source files and can always be rebuilt,
-- so the table is UNLOGGED (no WAL; emptied by Postgres after a crash).
CREATE UNLOGGED TABLE IF NOT EXISTS coupon_codes (
    code    TEXT     NOT NULL,
    file_id SMALLINT NOT NULL
);

CREATE TABLE IF NOT EXISTS import_status (
    name         TEXT PRIMARY KEY,
    row_count    BIGINT NOT NULL,
    completed_at TIMESTAMPTZ NOT NULL
);

-- The importer drops this before loading and recreates it afterwards, since
-- building an index once is much faster than maintaining it during COPY.
CREATE INDEX IF NOT EXISTS idx_coupon_code_file ON coupon_codes (code, file_id);
