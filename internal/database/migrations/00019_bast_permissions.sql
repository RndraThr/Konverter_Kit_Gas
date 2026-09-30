-- +goose Up
INSERT INTO permissions (code, name, description) VALUES
    ('bast.view', 'Lihat Berita Acara', 'Melihat ruang kerja dan dokumen berita acara')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id
FROM roles CROSS JOIN permissions
WHERE roles.code = 'super_admin'
  AND permissions.code = 'bast.view'
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM permissions WHERE code = 'bast.view';
