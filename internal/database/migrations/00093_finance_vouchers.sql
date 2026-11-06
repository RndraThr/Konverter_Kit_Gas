-- +goose Up
-- Voucher bundle: setelah pengajuan selesai, finance menerbitkan Payment
-- Voucher berjurnal debit-kredit, disusul bukti transaksi dan bukti
-- pembayaran tiap baris, satu berkas PDF.
CREATE TABLE finance_voucher_settings (
    id boolean PRIMARY KEY DEFAULT true CHECK (id),
    company_name text NOT NULL DEFAULT '',
    address_line1 text NOT NULL DEFAULT '',
    address_line2 text NOT NULL DEFAULT '',
    logo_key text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO finance_voucher_settings (id) VALUES (true);

-- voucher_prefix membentuk nomor voucher, contoh BSM + 26 + 0418.
CREATE TABLE finance_banks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 80),
    account_number text NOT NULL DEFAULT '',
    voucher_prefix text NOT NULL CHECK (voucher_prefix ~ '^[A-Z0-9]{1,8}$'),
    is_active boolean NOT NULL DEFAULT true,
    position integer NOT NULL DEFAULT 0
);

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

-- entry: debit menambah biaya, kredit menguranginya (misal PPh dipotong).
-- item_key tetap sama saat baris disimpan ulang, supaya buktinya tidak lepas.
ALTER TABLE finance_request_items
    ADD COLUMN item_key uuid NOT NULL DEFAULT gen_random_uuid(),
    ADD COLUMN entry text NOT NULL DEFAULT 'debit' CHECK (entry IN ('debit', 'kredit')),
    ADD COLUMN note text NOT NULL DEFAULT '',
    ADD COLUMN project_code text NOT NULL DEFAULT '';

ALTER TABLE finance_request_attachments
    DROP CONSTRAINT finance_request_attachments_kind_check,
    ADD CONSTRAINT finance_request_attachments_kind_check CHECK (kind IN ('evidence', 'payment', 'settlement', 'item_transaction', 'item_payment')),
    ADD COLUMN item_key uuid,
    ADD COLUMN original_name text NOT NULL DEFAULT '';
CREATE INDEX finance_request_attachments_item_idx ON finance_request_attachments (request_id, item_key);

-- +goose Down
DROP INDEX finance_request_attachments_item_idx;
DELETE FROM finance_request_attachments WHERE kind IN ('item_transaction', 'item_payment');
ALTER TABLE finance_request_attachments
    DROP COLUMN original_name,
    DROP COLUMN item_key,
    DROP CONSTRAINT finance_request_attachments_kind_check,
    ADD CONSTRAINT finance_request_attachments_kind_check CHECK (kind IN ('evidence', 'payment', 'settlement'));
ALTER TABLE finance_request_items DROP COLUMN project_code, DROP COLUMN note, DROP COLUMN entry, DROP COLUMN item_key;
ALTER TABLE finance_requests
    DROP COLUMN voucher_issued_at, DROP COLUMN voucher_page_count, DROP COLUMN voucher_storage_key, DROP COLUMN voucher_received_by,
    DROP COLUMN voucher_bank_id, DROP COLUMN voucher_date, DROP COLUMN voucher_number;
DROP TABLE finance_voucher_counters;
DROP TABLE finance_banks;
DROP TABLE finance_voucher_settings;
