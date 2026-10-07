-- +goose Up
ALTER TABLE distribution_slots
    ALTER COLUMN distribution_date DROP NOT NULL,
    ALTER COLUMN distribution_date DROP DEFAULT;

ALTER TABLE media_files
    ADD COLUMN storage_state text NOT NULL DEFAULT 'final'
        CHECK (storage_state IN ('staging', 'moving', 'final', 'move_failed')),
    ADD COLUMN storage_last_error text NOT NULL DEFAULT '',
    ADD COLUMN storage_target_generation bigint NOT NULL DEFAULT 0
        CHECK (storage_target_generation >= 0);

CREATE TABLE distribution_media_move_jobs (
    media_file_id uuid PRIMARY KEY REFERENCES media_files(id) ON DELETE CASCADE,
    target_path text[] NOT NULL CHECK (cardinality(target_path) > 0),
    target_generation bigint NOT NULL CHECK (target_generation > 0),
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'processing', 'retry')),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    locked_at timestamptz,
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX distribution_media_move_jobs_ready_idx
    ON distribution_media_move_jobs (next_attempt_at, updated_at)
    WHERE status IN ('queued', 'retry', 'processing');

CREATE INDEX media_files_storage_state_idx
    ON media_files (storage_state, documentation_slot_id)
    WHERE status = 'accepted';

-- +goose Down
DROP TABLE distribution_media_move_jobs;

ALTER TABLE media_files
    DROP COLUMN storage_target_generation,
    DROP COLUMN storage_last_error,
    DROP COLUMN storage_state;

UPDATE distribution_slots
SET distribution_date = (CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Jakarta')::date
WHERE distribution_date IS NULL;

ALTER TABLE distribution_slots
    ALTER COLUMN distribution_date SET DEFAULT ((CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Jakarta')::date),
    ALTER COLUMN distribution_date SET NOT NULL;
