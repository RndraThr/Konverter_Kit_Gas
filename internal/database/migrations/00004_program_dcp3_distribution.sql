-- +goose Up
CREATE TABLE regencies (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    province_name text NOT NULL,
    name text NOT NULL,
    document_code varchar(3) NOT NULL CHECK (document_code ~ '^[A-Z]{3}$'),
    is_active boolean NOT NULL DEFAULT true,
    notes text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX regencies_name_province_uq ON regencies (lower(name), lower(province_name));
CREATE UNIQUE INDEX regencies_document_code_uq ON regencies (document_code);

CREATE TABLE programs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code text NOT NULL UNIQUE,
    name text NOT NULL,
    program_type text NOT NULL CHECK (program_type IN ('farmer', 'fisherman')),
    fiscal_year integer NOT NULL CHECK (fiscal_year >= 2000),
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'completed', 'archived')),
    notes text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE package_template_versions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    template_code text NOT NULL,
    version integer NOT NULL CHECK (version > 0),
    name text NOT NULL,
    program_type text NOT NULL CHECK (program_type IN ('farmer', 'fisherman')),
    values_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'retired')),
    published_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (template_code, version)
);

CREATE TABLE documentation_template_versions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    template_code text NOT NULL,
    version integer NOT NULL CHECK (version > 0),
    name text NOT NULL,
    program_type text NOT NULL CHECK (program_type IN ('farmer', 'fisherman')),
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'retired')),
    published_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (template_code, version)
);

CREATE TABLE documentation_template_slots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    template_version_id uuid NOT NULL REFERENCES documentation_template_versions(id) ON DELETE CASCADE,
    slot_code text NOT NULL,
    label text NOT NULL,
    stage text NOT NULL DEFAULT 'distribution',
    is_required boolean NOT NULL DEFAULT true,
    min_files integer NOT NULL DEFAULT 1 CHECK (min_files >= 0),
    max_files integer NOT NULL DEFAULT 1 CHECK (max_files >= min_files),
    input_source text NOT NULL DEFAULT 'both' CHECK (input_source IN ('camera', 'gallery', 'both')),
    require_location boolean NOT NULL DEFAULT false,
    require_captured_at boolean NOT NULL DEFAULT false,
    instructions text,
    sort_order integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (template_version_id, slot_code)
);

CREATE TABLE program_schedules (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    program_id uuid NOT NULL REFERENCES programs(id),
    regency_id uuid NOT NULL REFERENCES regencies(id),
    package_template_version_id uuid NOT NULL REFERENCES package_template_versions(id),
    documentation_template_version_id uuid NOT NULL REFERENCES documentation_template_versions(id),
    name text NOT NULL,
    start_date date NOT NULL,
    end_date date NOT NULL,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'completed', 'cancelled')),
    distribution_number_padding integer NOT NULL DEFAULT 4 CHECK (distribution_number_padding BETWEEN 1 AND 8),
    receipt_policy_json jsonb NOT NULL DEFAULT '{"mode":"block_repeat"}'::jsonb,
    notes text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (end_date >= start_date)
);
CREATE INDEX program_schedules_context_idx ON program_schedules (regency_id, status, start_date);

CREATE TABLE dcp3_import_batches (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    schedule_id uuid NOT NULL REFERENCES program_schedules(id),
    original_filename text NOT NULL,
    file_checksum char(64) NOT NULL,
    source_name text,
    received_at date,
    sheet_name text NOT NULL,
    mapping_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'imported', 'failed', 'cancelled')),
    total_rows integer NOT NULL DEFAULT 0 CHECK (total_rows >= 0),
    valid_rows integer NOT NULL DEFAULT 0 CHECK (valid_rows >= 0),
    warning_rows integer NOT NULL DEFAULT 0 CHECK (warning_rows >= 0),
    invalid_rows integer NOT NULL DEFAULT 0 CHECK (invalid_rows >= 0),
    imported_by uuid REFERENCES users(id) ON DELETE SET NULL,
    imported_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (schedule_id, file_checksum)
);

CREATE TABLE dcp3_import_rows (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id uuid NOT NULL REFERENCES dcp3_import_batches(id) ON DELETE CASCADE,
    source_row_number integer NOT NULL CHECK (source_row_number > 0),
    source_sequence_number integer CHECK (source_sequence_number > 0),
    raw_data_json jsonb NOT NULL,
    normalized_data_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    validation_status text NOT NULL DEFAULT 'pending' CHECK (validation_status IN ('pending', 'valid', 'warning', 'needs_review', 'invalid')),
    validation_messages_json jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (batch_id, source_row_number)
);

CREATE TABLE people (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    full_name text NOT NULL,
    nik varchar(16),
    date_of_birth date,
    address text,
    village text,
    district text,
    phone_number text,
    verification_status text NOT NULL DEFAULT 'unverified' CHECK (verification_status IN ('unverified', 'verified', 'needs_review')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (nik IS NULL OR nik ~ '^[0-9]{16}$')
);
CREATE UNIQUE INDEX people_nik_uq ON people (nik) WHERE nik IS NOT NULL;
CREATE INDEX people_name_search_idx ON people (lower(full_name) text_pattern_ops);

CREATE TABLE person_sector_identifiers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    person_id uuid NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    identifier_type text NOT NULL CHECK (identifier_type IN ('farmer_card', 'kusuka')),
    normalized_value text NOT NULL,
    display_value text NOT NULL,
    verified_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (identifier_type, normalized_value),
    UNIQUE (person_id, identifier_type)
);

CREATE TABLE candidate_nominations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id uuid NOT NULL REFERENCES dcp3_import_batches(id),
    import_row_id uuid NOT NULL REFERENCES dcp3_import_rows(id),
    person_id uuid REFERENCES people(id),
    program_type text NOT NULL CHECK (program_type IN ('farmer', 'fisherman')),
    source_snapshot_json jsonb NOT NULL,
    status text NOT NULL DEFAULT 'candidate' CHECK (status IN ('candidate', 'ready', 'needs_review', 'cancelled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (import_row_id)
);

CREATE TABLE package_allocations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    schedule_id uuid NOT NULL REFERENCES program_schedules(id),
    nomination_id uuid NOT NULL REFERENCES candidate_nominations(id),
    intended_person_id uuid REFERENCES people(id),
    actual_recipient_person_id uuid REFERENCES people(id),
    distribution_number integer NOT NULL CHECK (distribution_number > 0),
    status text NOT NULL DEFAULT 'candidate' CHECK (status IN ('candidate', 'ready', 'needs_review', 'distributed', 'replaced', 'cancelled')),
    package_snapshot_json jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (schedule_id, distribution_number),
    UNIQUE (nomination_id)
);
CREATE INDEX package_allocations_intended_person_idx ON package_allocations (intended_person_id);

CREATE TABLE distribution_records (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    allocation_id uuid NOT NULL UNIQUE REFERENCES package_allocations(id),
    recipient_person_id uuid REFERENCES people(id),
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'completed', 'cancelled')),
    verification_snapshot_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    distributed_at timestamptz,
    distributed_by uuid REFERENCES users(id) ON DELETE SET NULL,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX distribution_records_recipient_idx ON distribution_records (recipient_person_id, status, completed_at DESC);

CREATE TABLE eligibility_checks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    person_id uuid NOT NULL REFERENCES people(id),
    schedule_id uuid NOT NULL REFERENCES program_schedules(id),
    result text NOT NULL CHECK (result IN ('eligible', 'incomplete', 'previously_received', 'approval_required', 'override_approved', 'identity_conflict')),
    reasons_json jsonb NOT NULL DEFAULT '[]'::jsonb,
    checked_at timestamptz NOT NULL DEFAULT now(),
    checked_by uuid REFERENCES users(id) ON DELETE SET NULL
);
CREATE INDEX eligibility_checks_person_idx ON eligibility_checks (person_id, checked_at DESC);

CREATE TABLE documentation_slots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    distribution_id uuid NOT NULL REFERENCES distribution_records(id) ON DELETE CASCADE,
    slot_code text NOT NULL,
    label_snapshot text NOT NULL,
    is_required boolean NOT NULL,
    min_files integer NOT NULL CHECK (min_files >= 0),
    max_files integer NOT NULL CHECK (max_files >= min_files),
    input_source text NOT NULL CHECK (input_source IN ('camera', 'gallery', 'both')),
    require_location boolean NOT NULL DEFAULT false,
    require_captured_at boolean NOT NULL DEFAULT false,
    status text NOT NULL DEFAULT 'missing' CHECK (status IN ('missing', 'local_draft', 'uploading', 'complete', 'rejected')),
    sort_order integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (distribution_id, slot_code)
);

CREATE TABLE media_files (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    documentation_slot_id uuid NOT NULL REFERENCES documentation_slots(id) ON DELETE CASCADE,
    storage_key uuid NOT NULL UNIQUE,
    original_filename text NOT NULL,
    mime_type text NOT NULL CHECK (mime_type IN ('image/jpeg', 'image/png', 'image/webp')),
    byte_size bigint NOT NULL CHECK (byte_size > 0 AND byte_size <= 10485760),
    checksum char(64) NOT NULL,
    source text NOT NULL CHECK (source IN ('camera', 'gallery')),
    captured_at timestamptz,
    latitude numeric(9,6),
    longitude numeric(9,6),
    status text NOT NULL DEFAULT 'accepted' CHECK (status IN ('accepted', 'rejected', 'deleted')),
    uploaded_by uuid REFERENCES users(id) ON DELETE SET NULL,
    uploaded_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (latitude IS NULL OR latitude BETWEEN -90 AND 90),
    CHECK (longitude IS NULL OR longitude BETWEEN -180 AND 180)
);
CREATE INDEX media_files_slot_status_idx ON media_files (documentation_slot_id, status);

INSERT INTO permissions (code, name, description)
VALUES
    ('programs.view', 'Lihat Persiapan Program', 'Melihat kabupaten, program, jadwal, dan template'),
    ('programs.manage', 'Kelola Persiapan Program', 'Mengelola kabupaten, program, jadwal, dan versi template'),
    ('dcp3.view', 'Lihat DCP3', 'Melihat batch dan hasil import DCP3'),
    ('dcp3.import', 'Import DCP3', 'Mengunggah, memetakan, dan mengimport DCP3'),
    ('distribution.view', 'Lihat Pendistribusian', 'Mencari dan memeriksa data penerima paket'),
    ('distribution.manage', 'Kelola Pendistribusian', 'Melengkapi data dan mengonfirmasi distribusi'),
    ('documentation.manage', 'Kelola Dokumentasi', 'Mengunggah dan menghapus dokumentasi distribusi')
ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id
FROM roles CROSS JOIN permissions
WHERE roles.code = 'super_admin'
  AND permissions.code IN ('programs.view', 'programs.manage', 'dcp3.view', 'dcp3.import', 'distribution.view', 'distribution.manage', 'documentation.manage')
ON CONFLICT DO NOTHING;

INSERT INTO package_template_versions (template_code, version, name, program_type, values_json, status, published_at)
VALUES
    ('PETANI-LPG', 1, 'Paket Perdana Petani LPG', 'farmer', '{"converter_brand":"ERGAS","components":[{"code":"lpg_tank","label":"Tabung LPG 3 Kg","quantity":1,"unit":"Tabung"},{"code":"regulator","label":"Regulator","quantity":1,"unit":"Pcs"},{"code":"suction_hose","label":"Selang Hisap","quantity":1,"unit":"Set"},{"code":"discharge_hose","label":"Selang Buang","quantity":1,"unit":"Set"},{"code":"manual","label":"Buku Manual","quantity":1,"unit":"Pcs"},{"code":"mixer_nipple","label":"Mixer Nipple","quantity":1,"unit":"Pcs"},{"code":"machine_warranty","label":"Kartu Garansi Mesin","quantity":1,"unit":"Ea"},{"code":"converter_warranty","label":"Kartu Garansi Konkit/Reducer","quantity":1,"unit":"Ea"},{"code":"oil","label":"Oli","quantity":2,"unit":"Botol"},{"code":"bracket","label":"Bracket","quantity":1,"unit":"Ea"}]}'::jsonb, 'published', now()),
    ('NELAYAN-LPG', 1, 'Paket Perdana Nelayan LPG', 'fisherman', '{"converter_brand":"ERGAS","components":[]}'::jsonb, 'published', now());

INSERT INTO documentation_template_versions (template_code, version, name, program_type, status, published_at)
VALUES
    ('DOK-PETANI', 1, 'Dokumentasi Distribusi Petani', 'farmer', 'published', now()),
    ('DOK-NELAYAN', 1, 'Dokumentasi Distribusi Nelayan', 'fisherman', 'published', now());

INSERT INTO documentation_template_slots
    (template_version_id, slot_code, label, is_required, min_files, max_files, input_source, sort_order)
SELECT templates.id, slots.slot_code, slots.label, true, 1, 1, 'both', slots.sort_order
FROM documentation_template_versions AS templates
CROSS JOIN (VALUES
    ('recipient_package', 'Penerima dan paket', 10),
    ('machine_serial', 'Serial number mesin', 20),
    ('package_completeness', 'Kelengkapan paket', 30),
    ('signed_bast', 'BAST bertanda tangan', 40)
) AS slots(slot_code, label, sort_order)
WHERE templates.version = 1 AND templates.template_code IN ('DOK-PETANI', 'DOK-NELAYAN');

-- +goose Down
DROP TABLE media_files;
DROP TABLE documentation_slots;
DROP TABLE eligibility_checks;
DROP TABLE distribution_records;
DROP TABLE package_allocations;
DROP TABLE candidate_nominations;
DROP TABLE person_sector_identifiers;
DROP TABLE people;
DROP TABLE dcp3_import_rows;
DROP TABLE dcp3_import_batches;
DROP TABLE program_schedules;
DROP TABLE documentation_template_slots;
DROP TABLE documentation_template_versions;
DROP TABLE package_template_versions;
DROP TABLE programs;
DROP TABLE regencies;

DELETE FROM permissions
WHERE code IN ('programs.view', 'programs.manage', 'dcp3.view', 'dcp3.import', 'distribution.view', 'distribution.manage', 'documentation.manage');
