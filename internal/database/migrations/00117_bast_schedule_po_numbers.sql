-- +goose Up
-- No. PO yang berbeda per kabupaten (mis. tabung gas: nomor LO Pertamina per
-- kabupaten). Bila ada, menimpa No. PO zona untuk barang+merk yang sama.
CREATE TABLE bast_schedule_po_numbers (
    schedule_id uuid NOT NULL REFERENCES program_schedules(id) ON DELETE CASCADE,
    item_kind text NOT NULL CHECK (item_kind IN ('machine', 'converter', 'hose_suction', 'hose_discharge', 'component')),
    item_code text NOT NULL CHECK (btrim(item_code) <> ''),
    po_number text NOT NULL CHECK (btrim(po_number) <> ''),
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (schedule_id, item_kind, item_code)
);

-- +goose Down
DROP TABLE bast_schedule_po_numbers;
