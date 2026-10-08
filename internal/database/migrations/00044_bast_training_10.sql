-- +goose Up
-- BA Training 10% memakai alur daftar hadir RAKORDA seperti Sosialisasi.
ALTER TABLE bast_schedule_settings
    ADD COLUMN training_10_location text NOT NULL DEFAULT '',
    ADD COLUMN training_10_row_count integer NOT NULL DEFAULT 51
        CHECK (training_10_row_count BETWEEN 5 AND 200);

ALTER TABLE bast_rakorda_uploads DROP CONSTRAINT bast_rakorda_uploads_document_kind_check;
ALTER TABLE bast_rakorda_uploads ADD CONSTRAINT bast_rakorda_uploads_document_kind_check
    CHECK (document_kind IN ('rakorda', 'sosialisasi', 'training_10'));

-- +goose Down
DELETE FROM bast_rakorda_uploads WHERE document_kind = 'training_10';
ALTER TABLE bast_rakorda_uploads DROP CONSTRAINT bast_rakorda_uploads_document_kind_check;
ALTER TABLE bast_rakorda_uploads ADD CONSTRAINT bast_rakorda_uploads_document_kind_check
    CHECK (document_kind IN ('rakorda', 'sosialisasi'));

ALTER TABLE bast_schedule_settings
    DROP COLUMN training_10_row_count,
    DROP COLUMN training_10_location;
