-- +goose Up
-- Daftar hadir Rakorda, Sosialisasi, dan Training boleh sampai 500 baris.
ALTER TABLE bast_schedule_settings
    DROP CONSTRAINT IF EXISTS bast_schedule_settings_rakorda_row_count_check,
    DROP CONSTRAINT IF EXISTS bast_schedule_settings_sosialisasi_row_count_check,
    DROP CONSTRAINT IF EXISTS bast_schedule_settings_training_10_row_count_check,
    DROP CONSTRAINT IF EXISTS bast_schedule_settings_training_100_row_count_check;
ALTER TABLE bast_schedule_settings
    ADD CONSTRAINT bast_schedule_settings_rakorda_row_count_check CHECK (rakorda_row_count BETWEEN 5 AND 500),
    ADD CONSTRAINT bast_schedule_settings_sosialisasi_row_count_check CHECK (sosialisasi_row_count BETWEEN 5 AND 500),
    ADD CONSTRAINT bast_schedule_settings_training_10_row_count_check CHECK (training_10_row_count BETWEEN 5 AND 500),
    ADD CONSTRAINT bast_schedule_settings_training_100_row_count_check CHECK (training_100_row_count BETWEEN 5 AND 500);

-- +goose Down
UPDATE bast_schedule_settings SET
    rakorda_row_count = LEAST(rakorda_row_count, 200),
    sosialisasi_row_count = LEAST(sosialisasi_row_count, 200),
    training_10_row_count = LEAST(training_10_row_count, 200),
    training_100_row_count = LEAST(training_100_row_count, 200);
ALTER TABLE bast_schedule_settings
    DROP CONSTRAINT IF EXISTS bast_schedule_settings_rakorda_row_count_check,
    DROP CONSTRAINT IF EXISTS bast_schedule_settings_sosialisasi_row_count_check,
    DROP CONSTRAINT IF EXISTS bast_schedule_settings_training_10_row_count_check,
    DROP CONSTRAINT IF EXISTS bast_schedule_settings_training_100_row_count_check;
ALTER TABLE bast_schedule_settings
    ADD CONSTRAINT bast_schedule_settings_rakorda_row_count_check CHECK (rakorda_row_count BETWEEN 5 AND 200),
    ADD CONSTRAINT bast_schedule_settings_sosialisasi_row_count_check CHECK (sosialisasi_row_count BETWEEN 5 AND 200),
    ADD CONSTRAINT bast_schedule_settings_training_10_row_count_check CHECK (training_10_row_count BETWEEN 5 AND 200),
    ADD CONSTRAINT bast_schedule_settings_training_100_row_count_check CHECK (training_100_row_count BETWEEN 5 AND 200);
