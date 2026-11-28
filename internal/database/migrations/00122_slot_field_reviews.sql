-- +goose Up
-- Data of a handover (recipient identity, contact, serial numbers) the central
-- monitoring sent back for revision. value_snapshot is the value that was
-- marked: the flag stays active until the field holds another value.
CREATE TABLE slot_field_reviews (
    distribution_slot_id uuid NOT NULL REFERENCES distribution_slots(id) ON DELETE CASCADE,
    field text NOT NULL,
    value_snapshot text NOT NULL,
    reason text NOT NULL DEFAULT '',
    reviewed_by uuid REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (distribution_slot_id, field)
);

-- +goose Down
DROP TABLE slot_field_reviews;
