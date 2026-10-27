-- +goose Up
-- Draf isian BA Gas Elpiji per tanggal penerimaan. Halaman tanda tangan basah
-- dan editor tanda tangan merender BA dari (jadwal, tanggal), jadi isian yang
-- belum difinalisasi disimpan di sini agar halaman cetak sama dengan PDF final.
CREATE TABLE bast_gas_elpiji_drafts (
    schedule_id uuid NOT NULL REFERENCES program_schedules(id) ON DELETE CASCADE,
    document_date date NOT NULL,
    input_json jsonb NOT NULL CHECK (jsonb_typeof(input_json) = 'object'),
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (schedule_id, document_date)
);

-- +goose Down
DROP TABLE bast_gas_elpiji_drafts;
