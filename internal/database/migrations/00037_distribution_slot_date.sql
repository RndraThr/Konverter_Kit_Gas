-- +goose Up
ALTER TABLE distribution_slots
    ADD COLUMN distribution_date date;

UPDATE distribution_slots
SET distribution_date = COALESCE(
    (distributed_at AT TIME ZONE 'Asia/Jakarta')::date,
    (created_at AT TIME ZONE 'Asia/Jakarta')::date
);

ALTER TABLE distribution_slots
    ALTER COLUMN distribution_date SET DEFAULT ((CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Jakarta')::date),
    ALTER COLUMN distribution_date SET NOT NULL;

-- +goose Down
ALTER TABLE distribution_slots DROP COLUMN distribution_date;
