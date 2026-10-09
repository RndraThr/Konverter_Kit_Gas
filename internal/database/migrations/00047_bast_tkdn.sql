-- +goose Up
-- Realisasi TKDN: daftar barang, merek, jumlah per paket, dan persentase TKDN
-- ditetapkan per program (tender); dokumen per kabupaten diterbitkan sebagai
-- dokumen agregat berversi.
CREATE TABLE bast_tkdn_profiles (
    program_id uuid PRIMARY KEY REFERENCES programs(id) ON DELETE CASCADE,
    items_json jsonb NOT NULL,
    total_tkdn numeric(5, 2) NOT NULL CHECK (total_tkdn BETWEEN 0 AND 100),
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE bast_aggregate_documents DROP CONSTRAINT bast_aggregate_documents_document_type_check;
ALTER TABLE bast_aggregate_documents ADD CONSTRAINT bast_aggregate_documents_document_type_check
    CHECK (document_type IN ('dp3', 'daily_recap', 'closing_titik_serah', 'closing_kabupaten', 'servis_berkala', 'tkdn'));

-- +goose Down
DELETE FROM bast_aggregate_documents WHERE document_type = 'tkdn';
ALTER TABLE bast_aggregate_documents DROP CONSTRAINT bast_aggregate_documents_document_type_check;
ALTER TABLE bast_aggregate_documents ADD CONSTRAINT bast_aggregate_documents_document_type_check
    CHECK (document_type IN ('dp3', 'daily_recap', 'closing_titik_serah', 'closing_kabupaten', 'servis_berkala'));

DROP TABLE bast_tkdn_profiles;
