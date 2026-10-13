-- +goose Up
-- Picklist of document names an officer chooses from when uploading or
-- scanning a Dokumen Pendukung PDF, scoped to one Jadwal Program so the
-- schedule is the single place that configures it. The name is never
-- stored as a foreign key on activity_media — only used once to build
-- that row's display_name, so renaming/deactivating a slot never changes
-- the name already printed on a past upload.
CREATE TABLE supporting_document_slots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    schedule_id uuid NOT NULL REFERENCES program_schedules(id) ON DELETE CASCADE,
    name text NOT NULL,
    is_active boolean NOT NULL DEFAULT true,
    sort_order integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_supporting_document_slots_schedule ON supporting_document_slots(schedule_id) WHERE is_active;

-- +goose Down
DROP TABLE IF EXISTS supporting_document_slots;
