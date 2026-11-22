-- +goose Up
INSERT INTO permissions (code, name, description) VALUES
    ('dashboard.view', 'Lihat Dashboard', 'Melihat grafik progres pekerjaan per program, zona, dan kabupaten')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id FROM roles CROSS JOIN permissions
WHERE roles.code = 'super_admin' AND permissions.code = 'dashboard.view'
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM permissions WHERE code = 'dashboard.view';
