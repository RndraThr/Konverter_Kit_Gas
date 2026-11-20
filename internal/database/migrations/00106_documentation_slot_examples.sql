-- +goose Up
-- Contoh foto per jenis slot dokumentasi pendistribusian (kode slot), sebagai
-- acuan sudut dan komposisi foto bagi petugas lapangan. Contoh berlaku untuk
-- semua jadwal dan kabupaten. Hanya super admin yang mengelolanya.
CREATE TABLE documentation_slot_examples (
    slot_code text PRIMARY KEY CHECK (slot_code ~ '^[a-z0-9_-]{1,64}$'),
    caption text NOT NULL DEFAULT '' CHECK (length(caption) <= 500),
    storage_key text NOT NULL,
    original_filename text NOT NULL,
    mime_type text NOT NULL CHECK (mime_type IN ('image/jpeg', 'image/png', 'image/webp')),
    byte_size bigint NOT NULL CHECK (byte_size > 0),
    checksum text NOT NULL,
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO permissions (code, name, description) VALUES
    ('documentation_examples.manage', 'Kelola Contoh Foto', 'Mengunggah dan mengubah contoh foto slot dokumentasi pendistribusian')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id FROM roles CROSS JOIN permissions
WHERE roles.code = 'super_admin' AND permissions.code = 'documentation_examples.manage'
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM permissions WHERE code = 'documentation_examples.manage';
DROP TABLE documentation_slot_examples;
