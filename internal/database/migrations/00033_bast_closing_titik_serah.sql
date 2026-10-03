-- +goose Up
-- Izinkan dokumen agregat baru: Closing Titik Serah (rekapitulasi lintas
-- hari closing harian untuk satu lokasi/titik serah).
ALTER TABLE bast_aggregate_documents DROP CONSTRAINT bast_aggregate_documents_document_type_check;
ALTER TABLE bast_aggregate_documents ADD CONSTRAINT bast_aggregate_documents_document_type_check
    CHECK (document_type IN ('dp3', 'daily_recap', 'closing_titik_serah'));

-- +goose Down
ALTER TABLE bast_aggregate_documents DROP CONSTRAINT bast_aggregate_documents_document_type_check;
ALTER TABLE bast_aggregate_documents ADD CONSTRAINT bast_aggregate_documents_document_type_check
    CHECK (document_type IN ('dp3', 'daily_recap'));
