-- +goose Up
-- Dokumen BAST perorangan dan closing sekarang menghitung lebar digit nomor
-- otomatis dari jumlah kuota/paket per kabupaten (lihat digitWidth di
-- internal/bast/service.go), jadi kolom override manual ini tidak lagi dipakai.
ALTER TABLE program_schedules DROP COLUMN distribution_number_padding;

-- +goose Down
ALTER TABLE program_schedules ADD COLUMN distribution_number_padding integer NOT NULL DEFAULT 4 CHECK (distribution_number_padding BETWEEN 1 AND 8);
