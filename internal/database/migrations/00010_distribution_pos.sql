-- +goose Up
CREATE TABLE distribution_slots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    schedule_id uuid NOT NULL REFERENCES program_schedules(id),
    slot_number integer NOT NULL CHECK (slot_number > 0),
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'linked', 'completed', 'cancelled')),
    allocation_id uuid REFERENCES package_allocations(id),
    recipient_person_id uuid REFERENCES people(id),
    machine_option_code text,
    machine_serial_number text,
    hose_option_code text,
    hose_serial_number text,
    converter_serial_number text,
    verification_snapshot_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    distributed_at timestamptz,
    distributed_by uuid REFERENCES users(id) ON DELETE SET NULL,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (schedule_id, slot_number),
    UNIQUE (allocation_id)
);
CREATE INDEX distribution_slots_recipient_idx ON distribution_slots (recipient_person_id, status, completed_at DESC);

INSERT INTO distribution_slots (id, schedule_id, slot_number, status, allocation_id, recipient_person_id, machine_option_code, machine_serial_number, hose_option_code, hose_serial_number, converter_serial_number, verification_snapshot_json, distributed_at, distributed_by, completed_at, created_at, updated_at)
SELECT dr.id, pa.schedule_id, pa.distribution_number,
    CASE dr.status WHEN 'completed' THEN 'completed' WHEN 'cancelled' THEN 'cancelled' ELSE 'linked' END,
    pa.id, dr.recipient_person_id, dr.machine_option_code, dr.machine_serial_number, dr.hose_option_code, dr.hose_serial_number, dr.converter_serial_number,
    dr.verification_snapshot_json, dr.distributed_at, dr.distributed_by, dr.completed_at, dr.created_at, dr.updated_at
FROM distribution_records dr
JOIN package_allocations pa ON pa.id = dr.allocation_id;

ALTER TABLE documentation_slots DROP CONSTRAINT documentation_slots_distribution_id_fkey;
ALTER TABLE documentation_slots RENAME COLUMN distribution_id TO distribution_slot_id;
ALTER TABLE documentation_slots ADD CONSTRAINT documentation_slots_distribution_slot_id_fkey FOREIGN KEY (distribution_slot_id) REFERENCES distribution_slots(id) ON DELETE CASCADE;

DROP TABLE distribution_records;

ALTER TABLE package_allocations ALTER COLUMN distribution_number DROP NOT NULL;

UPDATE documentation_template_slots SET stage = 'penyerahan' WHERE stage NOT IN ('mesin', 'dokumen', 'penyerahan');
ALTER TABLE documentation_template_slots ADD CONSTRAINT documentation_template_slots_stage_check CHECK (stage IN ('mesin', 'dokumen', 'penyerahan'));
ALTER TABLE documentation_template_slots ALTER COLUMN stage DROP DEFAULT;

INSERT INTO permissions(code, name, description) VALUES
    ('distribution.pos_mesin', 'POS Mesin', 'Menandai unit mesin dengan nomor bagi dan mengunggah bukti mesin'),
    ('distribution.pos_dokumen', 'POS Dokumen', 'Mengaitkan NIK penerima ke nomor bagi dan melengkapi data identitas'),
    ('distribution.pos_penyerahan', 'POS Penyerahan', 'Menyelesaikan serah terima dan mengunggah bukti penyerahan')
ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id FROM roles CROSS JOIN permissions
WHERE roles.code = 'super_admin' AND permissions.code IN ('distribution.pos_mesin', 'distribution.pos_dokumen', 'distribution.pos_penyerahan')
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM permissions WHERE code IN ('distribution.pos_mesin', 'distribution.pos_dokumen', 'distribution.pos_penyerahan');

ALTER TABLE documentation_template_slots ALTER COLUMN stage SET DEFAULT 'distribution';
ALTER TABLE documentation_template_slots DROP CONSTRAINT documentation_template_slots_stage_check;

ALTER TABLE package_allocations ALTER COLUMN distribution_number SET NOT NULL;

CREATE TABLE distribution_records (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    allocation_id uuid NOT NULL UNIQUE REFERENCES package_allocations(id),
    recipient_person_id uuid REFERENCES people(id),
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'completed', 'cancelled')),
    machine_option_code text,
    machine_serial_number text,
    hose_option_code text,
    hose_serial_number text,
    converter_serial_number text,
    verification_snapshot_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    distributed_at timestamptz,
    distributed_by uuid REFERENCES users(id) ON DELETE SET NULL,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX distribution_records_recipient_idx ON distribution_records (recipient_person_id, status, completed_at DESC);

INSERT INTO distribution_records (id, allocation_id, recipient_person_id, status, machine_option_code, machine_serial_number, hose_option_code, hose_serial_number, converter_serial_number, verification_snapshot_json, distributed_at, distributed_by, completed_at, created_at, updated_at)
SELECT id, allocation_id, recipient_person_id,
    CASE status WHEN 'completed' THEN 'completed' WHEN 'cancelled' THEN 'cancelled' ELSE 'draft' END,
    machine_option_code, machine_serial_number, hose_option_code, hose_serial_number, converter_serial_number,
    verification_snapshot_json, distributed_at, distributed_by, completed_at, created_at, updated_at
FROM distribution_slots WHERE allocation_id IS NOT NULL;

ALTER TABLE documentation_slots DROP CONSTRAINT documentation_slots_distribution_slot_id_fkey;
ALTER TABLE documentation_slots RENAME COLUMN distribution_slot_id TO distribution_id;
ALTER TABLE documentation_slots ADD CONSTRAINT documentation_slots_distribution_id_fkey FOREIGN KEY (distribution_id) REFERENCES distribution_records(id) ON DELETE CASCADE;

DROP TABLE distribution_slots;
