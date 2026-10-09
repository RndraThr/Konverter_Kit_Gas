-- +goose Up
-- BA Agenda Servis Berkala: jadwal servis ke-1 dan ke-2 per jadwal kabupaten,
-- dicetak sebagai dokumen agregat berversi seperti Closing Kabupaten.
ALTER TABLE bast_schedule_settings
    ADD COLUMN servis_1_start date,
    ADD COLUMN servis_1_end date,
    ADD COLUMN servis_2_start date,
    ADD COLUMN servis_2_end date,
    ADD CONSTRAINT bast_schedule_settings_servis_1_range_check CHECK (servis_1_start IS NULL OR servis_1_end IS NULL OR servis_1_start <= servis_1_end),
    ADD CONSTRAINT bast_schedule_settings_servis_2_range_check CHECK (servis_2_start IS NULL OR servis_2_end IS NULL OR servis_2_start <= servis_2_end);

ALTER TABLE bast_aggregate_documents DROP CONSTRAINT bast_aggregate_documents_document_type_check;
ALTER TABLE bast_aggregate_documents ADD CONSTRAINT bast_aggregate_documents_document_type_check
    CHECK (document_type IN ('dp3', 'daily_recap', 'closing_titik_serah', 'closing_kabupaten', 'servis_berkala'));

-- +goose Down
DELETE FROM bast_aggregate_documents WHERE document_type = 'servis_berkala';
ALTER TABLE bast_aggregate_documents DROP CONSTRAINT bast_aggregate_documents_document_type_check;
ALTER TABLE bast_aggregate_documents ADD CONSTRAINT bast_aggregate_documents_document_type_check
    CHECK (document_type IN ('dp3', 'daily_recap', 'closing_titik_serah', 'closing_kabupaten'));

ALTER TABLE bast_schedule_settings
    DROP CONSTRAINT bast_schedule_settings_servis_2_range_check,
    DROP CONSTRAINT bast_schedule_settings_servis_1_range_check,
    DROP COLUMN servis_2_end,
    DROP COLUMN servis_2_start,
    DROP COLUMN servis_1_end,
    DROP COLUMN servis_1_start;
