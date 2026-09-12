-- +goose Up
ALTER TABLE candidate_nominations ALTER COLUMN batch_id DROP NOT NULL;
ALTER TABLE candidate_nominations ALTER COLUMN import_row_id DROP NOT NULL;

INSERT INTO permissions (code, name, description) VALUES
    ('recipients.view', 'Lihat Data Penerima', 'Melihat data penerima lintas kabupaten dan program'),
    ('recipients.manage', 'Kelola Data Penerima', 'Menambah, mengubah, membatalkan, dan memulihkan data penerima')
ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id
FROM roles CROSS JOIN permissions
WHERE roles.code = 'super_admin'
  AND permissions.code IN ('recipients.view', 'recipients.manage')
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM permissions WHERE code IN ('recipients.view', 'recipients.manage');
ALTER TABLE candidate_nominations ALTER COLUMN import_row_id SET NOT NULL;
ALTER TABLE candidate_nominations ALTER COLUMN batch_id SET NOT NULL;
