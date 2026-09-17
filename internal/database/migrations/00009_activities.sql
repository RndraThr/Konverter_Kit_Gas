-- +goose Up
CREATE TABLE activity_media (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    regency_id uuid NOT NULL REFERENCES regencies(id),
    activity_type text NOT NULL CHECK (activity_type IN (
        'ceremony_sosialisasi', 'pelatihan_teknis', 'rakor',
        'training_10', 'training_100',
        'unloading_konkit', 'unloading_mesin_pompa', 'unloading_oli',
        'unloading_selang', 'unloading_tabung_gas'
    )),
    storage_key text NOT NULL UNIQUE,
    display_name text NOT NULL,
    original_filename text NOT NULL,
    media_type text NOT NULL CHECK (media_type IN ('image', 'video')),
    mime_type text NOT NULL CHECK (mime_type IN (
        'image/jpeg', 'image/png', 'image/webp',
        'video/mp4', 'video/webm', 'video/quicktime'
    )),
    byte_size bigint NOT NULL CHECK (byte_size > 0 AND byte_size <= 104857600),
    checksum char(64) NOT NULL,
    source text NOT NULL CHECK (source IN ('camera', 'gallery')),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'deleted')),
    uploaded_by uuid REFERENCES users(id) ON DELETE SET NULL,
    uploaded_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX activity_media_regency_type_idx ON activity_media (regency_id, activity_type, status, uploaded_at DESC);

INSERT INTO permissions (code, name, description) VALUES
    ('activities.view', 'Lihat Dokumentasi Kegiatan', 'Melihat dokumentasi foto/video kegiatan lapangan lintas kabupaten'),
    ('activities.manage', 'Kelola Dokumentasi Kegiatan', 'Mengunggah dan menghapus dokumentasi foto/video kegiatan lapangan')
ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id
FROM roles CROSS JOIN permissions
WHERE roles.code = 'super_admin'
  AND permissions.code IN ('activities.view', 'activities.manage')
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM permissions WHERE code IN ('activities.view', 'activities.manage');
DROP TABLE activity_media;
