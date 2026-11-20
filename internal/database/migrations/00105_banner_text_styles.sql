-- +goose Up
-- Ukuran dan warna teks yang diisi pada banner, per teks:
-- {"plakat_name": {"size_percent": 120, "color": "#8B0000"}}. Kosong: bawaan.
ALTER TABLE banner_settings ADD COLUMN styles jsonb NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down
ALTER TABLE banner_settings DROP COLUMN styles;
