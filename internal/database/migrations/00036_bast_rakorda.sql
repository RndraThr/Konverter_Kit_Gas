-- +goose Up
ALTER TABLE bast_schedule_settings
    ADD COLUMN rakorda_location text NOT NULL DEFAULT '',
    ADD COLUMN rakorda_row_count integer NOT NULL DEFAULT 45
        CHECK (rakorda_row_count BETWEEN 5 AND 200);

CREATE TABLE bast_rakorda_uploads (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    schedule_id uuid NOT NULL REFERENCES program_schedules(id) ON DELETE CASCADE,
    event_date date NOT NULL,
    original_name text NOT NULL,
    mime_type text NOT NULL,
    byte_size bigint NOT NULL CHECK (byte_size > 0),
    storage_key text NOT NULL,
    storage_backend text NOT NULL DEFAULT 'local',
    uploaded_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX bast_rakorda_uploads_schedule_date_idx
    ON bast_rakorda_uploads(schedule_id, event_date DESC, created_at DESC);

-- +goose Down
DROP TABLE bast_rakorda_uploads;
ALTER TABLE bast_schedule_settings
    DROP COLUMN rakorda_row_count,
    DROP COLUMN rakorda_location;
