-- +goose Up
ALTER TABLE users
    ADD COLUMN full_name text NOT NULL DEFAULT '',
    ADD COLUMN updated_by uuid REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX users_active_created_idx ON users (is_active, created_at DESC);

CREATE TABLE system_settings (
    key text PRIMARY KEY,
    value jsonb NOT NULL,
    value_type text NOT NULL CHECK (value_type IN ('string', 'timezone', 'date_format', 'locale')),
    description text,
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE audit_logs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    action text NOT NULL,
    resource_type text NOT NULL,
    resource_id text,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    ip_address inet,
    user_agent text,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_logs_created_at_idx ON audit_logs (created_at DESC);
CREATE INDEX audit_logs_actor_user_id_idx ON audit_logs (actor_user_id);
CREATE INDEX audit_logs_action_idx ON audit_logs (action);
CREATE INDEX audit_logs_resource_idx ON audit_logs (resource_type, resource_id);

INSERT INTO permissions (code, name, description)
VALUES
    ('dashboard.view', 'Lihat Dashboard', 'Mengakses halaman dashboard'),
    ('users.view', 'Lihat Pengguna', 'Melihat daftar dan detail pengguna'),
    ('users.manage', 'Kelola Pengguna', 'Membuat dan mengubah pengguna serta role pengguna'),
    ('roles.view', 'Lihat Role', 'Melihat role dan permission'),
    ('roles.manage', 'Kelola Role', 'Membuat dan mengubah role beserta permission'),
    ('settings.view', 'Lihat Pengaturan', 'Melihat pengaturan sistem yang aman'),
    ('settings.manage', 'Kelola Pengaturan', 'Mengubah pengaturan sistem yang aman'),
    ('health.view', 'Lihat System Health', 'Melihat status dependency dan versi sistem'),
    ('audit.view', 'Lihat Audit Log', 'Melihat riwayat perubahan administratif')
ON CONFLICT (code) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description;

INSERT INTO system_settings (key, value, value_type, description)
VALUES
    ('application_name', '"Sistem Manajemen Program Konkit Gas"'::jsonb, 'string', 'Nama aplikasi yang tampil pada area administrasi'),
    ('timezone', '"Asia/Jakarta"'::jsonb, 'timezone', 'Zona waktu utama aplikasi'),
    ('date_format', '"02/01/2006"'::jsonb, 'date_format', 'Format tanggal utama aplikasi'),
    ('locale', '"id-ID"'::jsonb, 'locale', 'Bahasa dan locale aplikasi'),
    ('organization_name', '"PT Kian Santang Muliatama Tbk"'::jsonb, 'string', 'Nama organisasi pelaksana')
ON CONFLICT (key) DO NOTHING;

-- +goose Down
DROP TABLE audit_logs;
DROP TABLE system_settings;

DELETE FROM permissions
WHERE code IN (
    'users.view',
    'users.manage',
    'roles.view',
    'roles.manage',
    'settings.view',
    'settings.manage',
    'health.view',
    'audit.view'
);

DROP INDEX users_active_created_idx;

ALTER TABLE users
    DROP COLUMN updated_by,
    DROP COLUMN full_name;
