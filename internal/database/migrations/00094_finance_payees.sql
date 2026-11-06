-- +goose Up
-- Rekening penerima yang pernah dipakai tim, agar leader tinggal memilih.
CREATE TABLE finance_payees (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id uuid NOT NULL REFERENCES finance_teams(id) ON DELETE CASCADE,
    account_name text NOT NULL CHECK (length(btrim(account_name)) BETWEEN 1 AND 200),
    bank_name text NOT NULL CHECK (length(btrim(bank_name)) BETWEEN 1 AND 200),
    account_number text NOT NULL CHECK (account_number ~ '^[0-9]{5,34}$'),
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (team_id, bank_name, account_number)
);

-- +goose Down
DROP TABLE finance_payees;
