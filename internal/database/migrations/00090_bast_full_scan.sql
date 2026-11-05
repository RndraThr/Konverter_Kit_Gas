-- +goose Up
-- Pindai lengkap: seluruh BA dicetak, ditandatangani/diparaf basah, lalu
-- semua halamannya dipindai (atau PDF hasil pindai diunggah). Selama ada
-- pindaian yang cocok dengan isi BA, pindaian itulah PDF final BA; tanpa
-- pindaian BA dibuat sistem (tanda tangan digital atau halaman tanda tangan
-- basah) seperti biasa.
CREATE TABLE bast_full_scans (
    schedule_id uuid NOT NULL REFERENCES program_schedules(id) ON DELETE CASCADE,
    document_key text NOT NULL,
    document_date date NOT NULL,
    -- content_code: sidik isi BA saat diunggah; isi berubah = perlu dipindai ulang.
    content_code text NOT NULL,
    page_count integer NOT NULL CHECK (page_count > 0),
    pdf bytea NOT NULL,
    byte_size integer NOT NULL,
    uploaded_by uuid REFERENCES users(id) ON DELETE SET NULL,
    uploaded_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (schedule_id, document_key, document_date)
);

-- +goose Down
DROP TABLE bast_full_scans;
