-- +goose Up
-- Live progress: one notification per kabupaten that is updated as
-- recipients are completed. push_tag names the notification a newer one
-- replaces (on the phone and in the inbox); push_silent sends it without a
-- sound. live_completed/live_on remember what the last update said.
ALTER TABLE notifications ADD COLUMN push_tag text NOT NULL DEFAULT '';
ALTER TABLE notifications ADD COLUMN push_silent boolean NOT NULL DEFAULT false;
CREATE INDEX notifications_user_tag_idx ON notifications (user_id, push_tag) WHERE push_tag <> '';
ALTER TABLE distribution_progress_notices ADD COLUMN live_completed integer;
ALTER TABLE distribution_progress_notices ADD COLUMN live_on date;

-- +goose Down
ALTER TABLE distribution_progress_notices DROP COLUMN live_on;
ALTER TABLE distribution_progress_notices DROP COLUMN live_completed;
DROP INDEX notifications_user_tag_idx;
ALTER TABLE notifications DROP COLUMN push_silent;
ALTER TABLE notifications DROP COLUMN push_tag;
