-- +goose Up
-- Dokumen agregat untuk DP3 (Daftar Penerima Paket Perdana) dan Rekapitulasi Harian.
-- storage_key adalah string opaque (bisa ID file Google Drive), bukan UUID.
CREATE TABLE bast_aggregate_documents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    schedule_id uuid NOT NULL REFERENCES program_schedules(id) ON DELETE RESTRICT,
    program_id uuid NOT NULL REFERENCES programs(id) ON DELETE RESTRICT,
    regency_id uuid NOT NULL REFERENCES regencies(id) ON DELETE RESTRICT,
    document_type text NOT NULL CHECK (document_type IN ('dp3', 'daily_recap')),
    document_date date NOT NULL,
    filename text NOT NULL CHECK (btrim(filename) <> ''),
    recipient_count integer NOT NULL CHECK (recipient_count > 0),
    page_count integer NOT NULL CHECK (page_count > 0),
    version integer NOT NULL CHECK (version > 0),
    status text NOT NULL CHECK (status IN ('active', 'superseded')),
    checksum char(64) NOT NULL CHECK (checksum ~ '^[0-9a-f]{64}$'),
    storage_key text,
    snapshot_json jsonb NOT NULL CHECK (jsonb_typeof(snapshot_json) = 'object'),
    last_error text,
    finalized_by uuid REFERENCES users(id) ON DELETE SET NULL,
    finalized_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (schedule_id, document_type, document_date, version)
);
CREATE UNIQUE INDEX bast_aggregate_documents_active_uq
    ON bast_aggregate_documents (schedule_id, document_type, document_date) WHERE status = 'active';
CREATE INDEX bast_aggregate_documents_date_idx
    ON bast_aggregate_documents (schedule_id, document_type, document_date DESC, version DESC);

-- +goose Down
DROP TABLE bast_aggregate_documents;
