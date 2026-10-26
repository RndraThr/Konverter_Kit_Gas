-- +goose Up
CREATE TABLE final_report_settings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    program_id uuid NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
    regency_id uuid NOT NULL REFERENCES regencies(id) ON DELETE CASCADE,
    leader_count integer NOT NULL DEFAULT 0 CHECK (leader_count >= 0),
    admin_count integer NOT NULL DEFAULT 0 CHECK (admin_count >= 0),
    technician_count integer NOT NULL DEFAULT 0 CHECK (technician_count >= 0),
    helper_count integer NOT NULL DEFAULT 0 CHECK (helper_count >= 0),
    machine_manufacturer_name text NOT NULL DEFAULT '',
    delivery_hose_date date,
    delivery_machine_group_date date,
    delivery_gas_cylinder_date date,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    UNIQUE (program_id, regency_id)
);

CREATE TABLE final_report_photo_picks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    program_id uuid NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
    regency_id uuid NOT NULL REFERENCES regencies(id) ON DELETE CASCADE,
    slide_key text NOT NULL CHECK (slide_key IN ('pengiriman_penerimaan', 'rapat_koordinasi', 'sosialisasi')),
    activity_media_id uuid NOT NULL REFERENCES activity_media(id) ON DELETE CASCADE,
    sort_order integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (program_id, regency_id, slide_key, activity_media_id)
);
CREATE INDEX idx_final_report_photo_picks_lookup ON final_report_photo_picks(program_id, regency_id, slide_key, sort_order);

CREATE TABLE final_report_documents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    program_id uuid NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
    regency_id uuid NOT NULL REFERENCES regencies(id) ON DELETE CASCADE,
    version integer NOT NULL CHECK (version > 0),
    filename text NOT NULL,
    checksum char(64) NOT NULL,
    storage_key text NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'superseded')),
    finalized_at timestamptz NOT NULL DEFAULT now(),
    finalized_by uuid REFERENCES users(id) ON DELETE SET NULL,
    UNIQUE (program_id, regency_id, version)
);
CREATE INDEX idx_final_report_documents_active ON final_report_documents(program_id, regency_id) WHERE status = 'active';

INSERT INTO permissions (code, name, description) VALUES
    ('final_report.view', 'Lihat Laporan Akhir', 'Melihat dan menampilkan presentasi Laporan Akhir per kabupaten'),
    ('final_report.manage', 'Kelola Laporan Akhir', 'Mengatur susunan tim, memilih foto, dan membuat PPTX Laporan Akhir')
ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id
FROM roles CROSS JOIN permissions
WHERE roles.code = 'super_admin'
  AND permissions.code IN ('final_report.view', 'final_report.manage')
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM permissions WHERE code IN ('final_report.view', 'final_report.manage');
DROP TABLE final_report_documents;
DROP TABLE final_report_photo_picks;
DROP TABLE final_report_settings;
