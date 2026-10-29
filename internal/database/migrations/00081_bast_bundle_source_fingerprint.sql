-- +goose Up
-- BA Perorangan is synced to Drive on demand. source_fingerprint records the
-- content (recipient data, numbers, signatories, marks, logos, signatures) a
-- bundle was built from, so the panel can label a date "Belum sinkron" after
-- any change. Existing bundles start empty and show as not yet synced.
ALTER TABLE bast_daily_bundles ADD COLUMN source_fingerprint text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE bast_daily_bundles DROP COLUMN IF EXISTS source_fingerprint;
