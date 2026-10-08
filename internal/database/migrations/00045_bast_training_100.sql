-- +goose Up
-- BA Training 100% memakai alur daftar hadir RAKORDA seperti Training 10%.
ALTER TABLE bast_schedule_settings
    ADD COLUMN training_100_location text NOT NULL DEFAULT '',
    ADD COLUMN training_100_row_count integer NOT NULL DEFAULT 51
        CHECK (training_100_row_count BETWEEN 5 AND 200);

ALTER TABLE bast_rakorda_uploads DROP CONSTRAINT bast_rakorda_uploads_document_kind_check;
ALTER TABLE bast_rakorda_uploads ADD CONSTRAINT bast_rakorda_uploads_document_kind_check
    CHECK (document_kind IN ('rakorda', 'sosialisasi', 'training_10', 'training_100'));

-- +goose Down
DELETE FROM bast_rakorda_uploads WHERE document_kind = 'training_100';
ALTER TABLE bast_rakorda_uploads DROP CONSTRAINT bast_rakorda_uploads_document_kind_check;
ALTER TABLE bast_rakorda_uploads ADD CONSTRAINT bast_rakorda_uploads_document_kind_check
    CHECK (document_kind IN ('rakorda', 'sosialisasi', 'training_10'));

ALTER TABLE bast_schedule_settings
    DROP COLUMN training_100_row_count,
    DROP COLUMN training_100_location;
