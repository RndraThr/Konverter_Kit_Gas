-- +goose Up
-- Limit per kabupaten yang diatur finance. Kasbon 0 berarti tanpa batas;
-- petty cash 0 berarti tidak tersedia. Pemakaian dihitung dari pengajuan yang
-- masih berjalan, jadi saldo kembali sendiri saat pengajuan selesai.
CREATE TABLE finance_limits (
    regency_id uuid PRIMARY KEY REFERENCES regencies(id) ON DELETE CASCADE,
    kasbon_limit bigint NOT NULL DEFAULT 0 CHECK (kasbon_limit >= 0),
    petty_cash_limit bigint NOT NULL DEFAULT 0 CHECK (petty_cash_limit >= 0),
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE finance_requests ADD COLUMN use_petty_cash boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE finance_requests DROP COLUMN use_petty_cash;
DROP TABLE finance_limits;
