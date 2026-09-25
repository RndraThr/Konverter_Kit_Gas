-- +goose Up
ALTER TABLE program_schedules ADD COLUMN slot_quota integer;
ALTER TABLE program_schedules ADD CONSTRAINT program_schedules_slot_quota_check CHECK (slot_quota IS NULL OR slot_quota > 0);

-- +goose Down
ALTER TABLE program_schedules DROP CONSTRAINT program_schedules_slot_quota_check;
ALTER TABLE program_schedules DROP COLUMN slot_quota;
