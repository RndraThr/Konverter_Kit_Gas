-- +goose Up
-- BA Sosialisasi memakai alur daftar hadir RAKORDA: cetak lembar kosong
-- bernomor, isi di lokasi, lalu unggah hasil pindaian. Unggahan RAKORDA
-- dibedakan per jenis dokumen melalui document_kind.
ALTER TABLE bast_schedule_settings
    ADD COLUMN sosialisasi_location text NOT NULL DEFAULT '',
    ADD COLUMN sosialisasi_row_count integer NOT NULL DEFAULT 51
        CHECK (sosialisasi_row_count BETWEEN 5 AND 200);

ALTER TABLE bast_rakorda_uploads
    ADD COLUMN document_kind text NOT NULL DEFAULT 'rakorda'
        CHECK (document_kind IN ('rakorda', 'sosialisasi'));
ALTER TABLE bast_rakorda_uploads ALTER COLUMN document_kind DROP DEFAULT;

DROP INDEX bast_rakorda_uploads_schedule_date_idx;
CREATE INDEX bast_rakorda_uploads_schedule_kind_date_idx
    ON bast_rakorda_uploads(schedule_id, document_kind, event_date DESC, created_at DESC);

-- +goose Down
DELETE FROM bast_rakorda_uploads WHERE document_kind <> 'rakorda';
DROP INDEX bast_rakorda_uploads_schedule_kind_date_idx;
ALTER TABLE bast_rakorda_uploads DROP COLUMN document_kind;
CREATE INDEX bast_rakorda_uploads_schedule_date_idx
    ON bast_rakorda_uploads(schedule_id, event_date DESC, created_at DESC);

ALTER TABLE bast_schedule_settings
    DROP COLUMN sosialisasi_row_count,
    DROP COLUMN sosialisasi_location;
