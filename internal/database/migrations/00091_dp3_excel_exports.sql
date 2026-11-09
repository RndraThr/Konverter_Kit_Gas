-- +goose Up
-- Excel DP3: satu workbook data penerima per jadwal kabupaten, dibangun
-- ulang berkala oleh server dan disimpan di folder BA/1. DP3 di Drive.
-- content_checksum adalah sidik isi data (bukan bytes xlsx), sehingga file
-- hanya diunggah ulang bila data penerima berubah. synced_at kosong berarti
-- belum pernah berhasil diunggah; last_error menyimpan kegagalan terakhir.
CREATE TABLE dp3_excel_exports (
    schedule_id uuid PRIMARY KEY REFERENCES program_schedules(id) ON DELETE CASCADE,
    filename text NOT NULL DEFAULT '',
    storage_key text NOT NULL DEFAULT '',
    content_checksum text NOT NULL DEFAULT '',
    recipient_count integer NOT NULL DEFAULT 0 CHECK (recipient_count >= 0),
    byte_size bigint NOT NULL DEFAULT 0 CHECK (byte_size >= 0),
    synced_at timestamptz,
    last_error text NOT NULL DEFAULT '',
    last_attempt_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE dp3_excel_exports;
