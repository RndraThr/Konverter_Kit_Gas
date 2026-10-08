-- +goose Up
-- BA Pemeriksaan Barang: satu template, banyak form barang. Daftar form per
-- program; nomor PO per jadwal kabupaten; tiap form diterbitkan sebagai
-- dokumen agregat sendiri dengan document_type "pemeriksaan:<kode form>".
CREATE TABLE bast_pemeriksaan_profiles (
    program_id uuid PRIMARY KEY REFERENCES programs(id) ON DELETE CASCADE,
    forms_json jsonb NOT NULL CHECK (jsonb_typeof(forms_json) = 'array'),
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE bast_schedule_settings ADD COLUMN pemeriksaan_po_number text NOT NULL DEFAULT '';

ALTER TABLE bast_aggregate_documents DROP CONSTRAINT bast_aggregate_documents_document_type_check;
ALTER TABLE bast_aggregate_documents ADD CONSTRAINT bast_aggregate_documents_document_type_check
    CHECK (document_type IN ('dp3', 'daily_recap', 'closing_titik_serah', 'closing_kabupaten', 'servis_berkala', 'tkdn')
        OR document_type ~ '^pemeriksaan:[a-z0-9_]{1,40}$');

-- +goose Down
DELETE FROM bast_aggregate_documents WHERE document_type LIKE 'pemeriksaan:%';
ALTER TABLE bast_aggregate_documents DROP CONSTRAINT bast_aggregate_documents_document_type_check;
ALTER TABLE bast_aggregate_documents ADD CONSTRAINT bast_aggregate_documents_document_type_check
    CHECK (document_type IN ('dp3', 'daily_recap', 'closing_titik_serah', 'closing_kabupaten', 'servis_berkala', 'tkdn'));

ALTER TABLE bast_schedule_settings DROP COLUMN pemeriksaan_po_number;
DROP TABLE bast_pemeriksaan_profiles;
