-- +goose Up
-- dashboard.view is held by the field and finance roles (the user summary
-- of the home page), so the progress charts get their own permission.
INSERT INTO permissions (code, name, description) VALUES
    ('progress_dashboard.view', 'Lihat Dashboard Progres', 'Melihat grafik progres pekerjaan per program, zona, dan kabupaten')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id FROM roles CROSS JOIN permissions
WHERE roles.code = 'super_admin' AND permissions.code = 'progress_dashboard.view'
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM permissions WHERE code = 'progress_dashboard.view';
