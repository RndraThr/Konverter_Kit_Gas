-- +goose Up
ALTER TABLE media_files
    ALTER COLUMN storage_key TYPE text
    USING storage_key::text;

-- +goose Down
-- Storage key Google Drive bukan UUID, sehingga konversi balik akan merusak data.
SELECT 1;
