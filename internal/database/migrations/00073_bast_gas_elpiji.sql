-- +goose Up
-- BA Gas Elpiji: satu dokumen per penerimaan LPG (gas_elpiji, per tanggal) dan
-- satu BA gabungan semua penerimaan jadwal (gas_elpiji_gabungan).
ALTER TABLE bast_aggregate_documents DROP CONSTRAINT bast_aggregate_documents_document_type_check;
ALTER TABLE bast_aggregate_documents ADD CONSTRAINT bast_aggregate_documents_document_type_check
    CHECK (document_type IN ('dp3', 'daily_recap', 'closing_titik_serah', 'closing_kabupaten', 'servis_berkala', 'tkdn', 'gas_elpiji', 'gas_elpiji_gabungan')
        OR document_type ~ '^pemeriksaan:[a-z0-9_]{1,40}$');

-- +goose Down
DELETE FROM bast_aggregate_documents WHERE document_type IN ('gas_elpiji', 'gas_elpiji_gabungan');
ALTER TABLE bast_aggregate_documents DROP CONSTRAINT bast_aggregate_documents_document_type_check;
ALTER TABLE bast_aggregate_documents ADD CONSTRAINT bast_aggregate_documents_document_type_check
    CHECK (document_type IN ('dp3', 'daily_recap', 'closing_titik_serah', 'closing_kabupaten', 'servis_berkala', 'tkdn')
        OR document_type ~ '^pemeriksaan:[a-z0-9_]{1,40}$');
