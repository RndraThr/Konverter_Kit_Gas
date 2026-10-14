-- +goose Up
-- Dokumen Pendukung's naming slots move from being scoped to one Jadwal
-- Program (schedule_id) to being scoped directly to a kabupaten
-- (regency_id). A kabupaten's slot list is reused across every Jadwal it
-- ever runs, instead of needing to be re-entered each time a new Jadwal is
-- created. No production data depends on the old shape yet (this table
-- shipped in the same release), so the column swap is a clean drop/add
-- rather than a data migration.
TRUNCATE TABLE supporting_document_slots;
ALTER TABLE supporting_document_slots DROP CONSTRAINT supporting_document_slots_schedule_id_fkey;
DROP INDEX IF EXISTS idx_supporting_document_slots_schedule;
ALTER TABLE supporting_document_slots DROP COLUMN schedule_id;
ALTER TABLE supporting_document_slots ADD COLUMN regency_id uuid NOT NULL REFERENCES regencies(id) ON DELETE CASCADE;
CREATE INDEX idx_supporting_document_slots_regency ON supporting_document_slots(regency_id) WHERE is_active;

-- +goose Down
TRUNCATE TABLE supporting_document_slots;
ALTER TABLE supporting_document_slots DROP CONSTRAINT supporting_document_slots_regency_id_fkey;
DROP INDEX IF EXISTS idx_supporting_document_slots_regency;
ALTER TABLE supporting_document_slots DROP COLUMN regency_id;
ALTER TABLE supporting_document_slots ADD COLUMN schedule_id uuid NOT NULL REFERENCES program_schedules(id) ON DELETE CASCADE;
CREATE INDEX idx_supporting_document_slots_schedule ON supporting_document_slots(schedule_id) WHERE is_active;
