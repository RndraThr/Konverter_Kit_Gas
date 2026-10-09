-- +goose Up
CREATE TABLE recipient_replacements (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    distribution_slot_id uuid NOT NULL REFERENCES distribution_slots(id) ON DELETE CASCADE,
    old_allocation_id uuid NOT NULL REFERENCES package_allocations(id),
    old_person_id uuid NOT NULL REFERENCES people(id),
    new_allocation_id uuid NOT NULL REFERENCES package_allocations(id),
    new_person_id uuid NOT NULL REFERENCES people(id),
    origin text NOT NULL CHECK (origin IN ('existing_allocation', 'new_allocation')),
    reason text NOT NULL CHECK (btrim(reason) <> ''),
    replaced_by uuid REFERENCES users(id) ON DELETE SET NULL,
    replaced_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX recipient_replacements_slot_idx ON recipient_replacements (distribution_slot_id, replaced_at DESC);
CREATE INDEX recipient_replacements_old_person_idx ON recipient_replacements (old_person_id);
CREATE INDEX recipient_replacements_new_person_idx ON recipient_replacements (new_person_id);

-- +goose Down
DROP TABLE recipient_replacements;
