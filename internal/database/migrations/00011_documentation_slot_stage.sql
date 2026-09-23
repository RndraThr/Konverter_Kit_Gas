-- +goose Up
ALTER TABLE documentation_slots ADD COLUMN stage text;

-- PostgreSQL disallows referencing the UPDATE target's own alias from inside a nested FROM-list JOIN's ON clause; only WHERE/SET can see it.
UPDATE documentation_slots ds SET stage = dts.stage
FROM distribution_slots dsl
JOIN program_schedules ps ON ps.id = dsl.schedule_id
JOIN documentation_template_slots dts ON dts.template_version_id = ps.documentation_template_version_id
WHERE ds.distribution_slot_id = dsl.id AND dts.slot_code = ds.slot_code AND ds.stage IS NULL;

UPDATE documentation_slots SET stage = 'penyerahan' WHERE stage IS NULL;

ALTER TABLE documentation_slots ALTER COLUMN stage SET NOT NULL;
ALTER TABLE documentation_slots ADD CONSTRAINT documentation_slots_stage_check CHECK (stage IN ('mesin', 'dokumen', 'penyerahan'));

-- +goose Down
ALTER TABLE documentation_slots DROP CONSTRAINT documentation_slots_stage_check;
ALTER TABLE documentation_slots DROP COLUMN stage;
