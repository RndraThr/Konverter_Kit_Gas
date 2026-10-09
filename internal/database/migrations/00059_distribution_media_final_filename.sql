-- +goose Up
ALTER TABLE distribution_media_move_jobs
    ADD COLUMN target_filename text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE distribution_media_move_jobs
    DROP COLUMN target_filename;
