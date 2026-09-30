-- +goose Up
INSERT INTO permissions (code, name, description) VALUES
    ('bast.manage', 'Kelola Berita Acara', 'Mengunci penomoran, membuat, dan menyinkronkan berita acara')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id
FROM roles CROSS JOIN permissions
WHERE roles.code = 'super_admin' AND permissions.code = 'bast.manage'
ON CONFLICT DO NOTHING;

CREATE TABLE program_regency_bast_settings (
    program_id uuid NOT NULL REFERENCES programs(id) ON DELETE RESTRICT,
    regency_id uuid NOT NULL REFERENCES regencies(id) ON DELETE RESTRICT,
    final_total integer NOT NULL CHECK (final_total > 0),
    locked_at timestamptz NOT NULL DEFAULT now(),
    locked_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (program_id, regency_id)
);

CREATE TABLE bast_individual_documents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    distribution_slot_id uuid NOT NULL REFERENCES distribution_slots(id) ON DELETE RESTRICT,
    program_id uuid NOT NULL REFERENCES programs(id) ON DELETE RESTRICT,
    regency_id uuid NOT NULL REFERENCES regencies(id) ON DELETE RESTRICT,
    document_type text NOT NULL DEFAULT 'individual' CHECK (document_type = 'individual'),
    local_date date NOT NULL,
    slot_number integer NOT NULL CHECK (slot_number > 0),
    final_total integer NOT NULL CHECK (final_total > 0 AND slot_number <= final_total),
    document_number text NOT NULL CHECK (btrim(document_number) <> ''),
    profile_version_id uuid NOT NULL REFERENCES program_document_profile_versions(id) ON DELETE RESTRICT,
    package_template_version_id uuid NOT NULL REFERENCES package_template_versions(id) ON DELETE RESTRICT,
    snapshot_json jsonb NOT NULL CHECK (jsonb_typeof(snapshot_json) = 'object'),
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0),
    status text NOT NULL DEFAULT 'final' CHECK (status IN ('final', 'superseded')),
    finalized_at timestamptz NOT NULL DEFAULT now(),
    finalized_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (distribution_slot_id, document_type, revision)
);
CREATE UNIQUE INDEX bast_individual_documents_current_uq ON bast_individual_documents (distribution_slot_id, document_type) WHERE status = 'final';
CREATE INDEX bast_individual_documents_date_idx ON bast_individual_documents (program_id, regency_id, local_date, slot_number) WHERE status = 'final';

CREATE TABLE bast_daily_bundles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    program_id uuid NOT NULL REFERENCES programs(id) ON DELETE RESTRICT,
    regency_id uuid NOT NULL REFERENCES regencies(id) ON DELETE RESTRICT,
    local_date date NOT NULL,
    document_type text NOT NULL DEFAULT 'individual' CHECK (document_type = 'individual'),
    profile_version_id uuid NOT NULL REFERENCES program_document_profile_versions(id) ON DELETE RESTRICT,
    filename text NOT NULL CHECK (btrim(filename) <> ''),
    recipient_count integer NOT NULL CHECK (recipient_count > 0),
    page_count integer NOT NULL CHECK (page_count > 0),
    checksum char(64) NOT NULL CHECK (checksum ~ '^[0-9a-f]{64}$'),
    storage_key text,
    version integer NOT NULL CHECK (version > 0),
    status text NOT NULL CHECK (status IN ('active', 'superseded', 'failed')),
    last_error text,
    synced_at timestamptz,
    synced_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (program_id, regency_id, local_date, document_type, version)
);
CREATE UNIQUE INDEX bast_daily_bundles_active_uq ON bast_daily_bundles (program_id, regency_id, local_date, document_type) WHERE status = 'active';
CREATE INDEX bast_daily_bundles_date_idx ON bast_daily_bundles (program_id, regency_id, local_date DESC, version DESC);

CREATE TABLE bast_daily_bundle_items (
    bundle_id uuid NOT NULL REFERENCES bast_daily_bundles(id) ON DELETE CASCADE,
    individual_document_id uuid NOT NULL REFERENCES bast_individual_documents(id) ON DELETE RESTRICT,
    item_order integer NOT NULL CHECK (item_order > 0),
    page_start integer NOT NULL CHECK (page_start > 0),
    page_end integer NOT NULL CHECK (page_end >= page_start),
    PRIMARY KEY (bundle_id, individual_document_id),
    UNIQUE (bundle_id, item_order)
);

-- +goose Down
DROP TABLE bast_daily_bundle_items;
DROP TABLE bast_daily_bundles;
DROP TABLE bast_individual_documents;
DROP TABLE program_regency_bast_settings;
