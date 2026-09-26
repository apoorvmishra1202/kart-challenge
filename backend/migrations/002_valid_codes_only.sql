-- The importer now applies the min-files rule itself and stores only valid
-- codes in valid_codes. Drop the per-row table from 001 (~13 GB table +
-- ~9.4 GB index on the full data) and the status columns it used.
DROP TABLE IF EXISTS coupon_codes;
DELETE FROM import_status WHERE name = 'coupon_codes';

ALTER TABLE import_status DROP COLUMN IF EXISTS index_ms;
ALTER TABLE import_status DROP COLUMN IF EXISTS vacuum_ms;
ALTER TABLE import_status DROP COLUMN IF EXISTS mode;

-- min_files the stored valid_codes were computed with; the importer
-- re-imports when IMPORT_MIN_FILES differs.
ALTER TABLE import_status ADD COLUMN IF NOT EXISTS min_files INT;
