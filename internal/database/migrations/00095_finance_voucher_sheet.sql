-- +goose Up
-- Voucher berdiri sendiri: finance menyusunnya di tabel, bisa dari beberapa
-- pengajuan atau manual. Nomornya diisi manual, dan satu pengajuan boleh masuk
-- lebih dari satu voucher.
ALTER TABLE finance_requests
    DROP COLUMN voucher_issued_at, DROP COLUMN voucher_page_count, DROP COLUMN voucher_storage_key, DROP COLUMN voucher_received_by,
    DROP COLUMN voucher_bank_id, DROP COLUMN voucher_date, DROP COLUMN voucher_number;
DROP TABLE finance_voucher_counters;

-- Rekening yang tercetak di voucher: bank, kas, dan lainnya.
ALTER TABLE finance_banks RENAME TO finance_accounts;
DELETE FROM finance_accounts;
ALTER TABLE finance_accounts
    DROP COLUMN voucher_prefix,
    ADD COLUMN kind text NOT NULL DEFAULT 'bank' CHECK (kind IN ('bank', 'kas', 'lainnya'));
INSERT INTO finance_accounts (kind, name, account_number, position) VALUES
    ('bank', 'BNI', '8131000638', 0),
    ('bank', 'Mandiri', '1670004576988', 1),
    ('bank', 'BRI', '210801000370308', 2),
    ('bank', 'BSI Operasional', '7185595751', 3),
    ('bank', 'BSI Escrow', '7274437388', 4),
    ('bank', 'BSI IPO', '5577889904', 5),
    ('bank', 'BSI Waran', '1235789104', 6),
    ('bank', 'BNI USD', '8131000661', 7),
    ('kas', 'GoPay', '', 8),
    ('kas', 'Hasanah Card', '', 9),
    ('kas', 'Rupiah', '', 10),
    ('kas', 'ShopeePay', '', 11),
    ('kas', 'Tokopedia', '', 12),
    ('lainnya', 'Hutang Reimburse KSO', '', 13);

ALTER TABLE finance_voucher_settings ADD COLUMN address_line3 text NOT NULL DEFAULT '';
UPDATE finance_voucher_settings SET company_name = 'PT KIAN SANTANG MULIATAMA TBK',
    address_line1 = 'Jalan Wibawa Mukti II, Perum Telkom Satwika Permai',
    address_line2 = 'Blok A5, No. 8, RT. 009, Kelurahan Jatiluhur, Kecamatan Jatiasih',
    address_line3 = 'Bekasi 17425 - INDONESIA';

CREATE TABLE finance_vouchers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    number text NOT NULL UNIQUE,
    voucher_date date NOT NULL,
    account_id uuid REFERENCES finance_accounts(id) ON DELETE SET NULL,
    pay_to text NOT NULL,
    description text NOT NULL DEFAULT '',
    total_amount bigint NOT NULL,
    page_count integer NOT NULL DEFAULT 0,
    storage_key text NOT NULL,
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX finance_vouchers_date_idx ON finance_vouchers (voucher_date DESC, created_at DESC);

CREATE TABLE finance_voucher_lines (
    voucher_id uuid NOT NULL REFERENCES finance_vouchers(id) ON DELETE CASCADE,
    position integer NOT NULL,
    description text NOT NULL,
    debit bigint NOT NULL DEFAULT 0 CHECK (debit >= 0),
    kredit bigint NOT NULL DEFAULT 0 CHECK (kredit >= 0),
    note text NOT NULL DEFAULT '',
    project_code text NOT NULL DEFAULT '',
    is_bank boolean NOT NULL DEFAULT false,
    item_key uuid,
    PRIMARY KEY (voucher_id, position)
);

CREATE TABLE finance_voucher_requests (
    voucher_id uuid NOT NULL REFERENCES finance_vouchers(id) ON DELETE CASCADE,
    request_id uuid NOT NULL REFERENCES finance_requests(id) ON DELETE CASCADE,
    PRIMARY KEY (voucher_id, request_id)
);
CREATE INDEX finance_voucher_requests_request_idx ON finance_voucher_requests (request_id);

-- +goose Down
DROP TABLE finance_voucher_requests;
DROP TABLE finance_voucher_lines;
DROP TABLE finance_vouchers;
ALTER TABLE finance_voucher_settings DROP COLUMN address_line3;
DELETE FROM finance_accounts;
ALTER TABLE finance_accounts
    DROP COLUMN kind,
    ADD COLUMN voucher_prefix text NOT NULL CHECK (voucher_prefix ~ '^[A-Z0-9]{1,8}$');
ALTER TABLE finance_accounts RENAME TO finance_banks;
CREATE TABLE finance_voucher_counters (
    prefix text NOT NULL,
    year integer NOT NULL,
    last_value integer NOT NULL,
    PRIMARY KEY (prefix, year)
);
ALTER TABLE finance_requests
    ADD COLUMN voucher_number text UNIQUE,
    ADD COLUMN voucher_date date,
    ADD COLUMN voucher_bank_id uuid REFERENCES finance_banks(id) ON DELETE SET NULL,
    ADD COLUMN voucher_received_by text NOT NULL DEFAULT '',
    ADD COLUMN voucher_storage_key text NOT NULL DEFAULT '',
    ADD COLUMN voucher_page_count integer NOT NULL DEFAULT 0,
    ADD COLUMN voucher_issued_at timestamptz;
