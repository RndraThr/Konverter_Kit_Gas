-- +goose Up
-- Seeds role templates for field teams, matching the granular distribution.pos_* permissions that
-- already exist specifically for this purpose (00010_distribution_pos.sql). None of these roles are
-- given a regency scope here — all_regencies_access stays false and role_regencies stays empty, since
-- which kabupaten each real team covers is operational configuration, not something to hardcode into
-- a migration. After seeding, an admin assigns regencies per role (Administrasi > Role), cloning a
-- role per team/kabupaten when different field teams need different areas.
INSERT INTO roles (code, name, description, is_system)
VALUES
    ('pos_mesin', 'POS Mesin', 'Mencatat unit mesin dan mengunggah bukti mesin di lapangan', false),
    ('pos_dokumen', 'POS Dokumen', 'Mengaitkan identitas penerima ke nomor bagi di lapangan', false),
    ('pos_penyerahan', 'POS Penyerahan', 'Menyelesaikan serah terima dan mengunggah bukti penyerahan', false),
    ('tim_lapangan', 'Tim Lapangan (Seluruh POS)', 'Menjalankan ketiga POS pendistribusian dalam satu tim kecil', false),
    ('dokumentasi_kegiatan', 'Dokumentasi Kegiatan', 'Mengunggah dokumentasi foto/video kegiatan lapangan', false),
    ('koordinator_lapangan', 'Koordinator Lapangan', 'Mengawasi pendistribusian, dokumentasi, dan berita acara di wilayahnya', false)
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id FROM roles CROSS JOIN permissions
WHERE roles.code = 'pos_mesin'
  AND permissions.code IN ('dashboard.view', 'distribution.view', 'distribution.pos_mesin', 'documentation.manage')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id FROM roles CROSS JOIN permissions
WHERE roles.code = 'pos_dokumen'
  AND permissions.code IN ('dashboard.view', 'distribution.view', 'distribution.pos_dokumen', 'documentation.manage')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id FROM roles CROSS JOIN permissions
WHERE roles.code = 'pos_penyerahan'
  AND permissions.code IN ('dashboard.view', 'distribution.view', 'distribution.pos_penyerahan', 'documentation.manage')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id FROM roles CROSS JOIN permissions
WHERE roles.code = 'tim_lapangan'
  AND permissions.code IN ('dashboard.view', 'distribution.view', 'distribution.pos_mesin', 'distribution.pos_dokumen', 'distribution.pos_penyerahan', 'documentation.manage')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id FROM roles CROSS JOIN permissions
WHERE roles.code = 'dokumentasi_kegiatan'
  AND permissions.code IN ('dashboard.view', 'activities.view', 'activities.manage')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id FROM roles CROSS JOIN permissions
WHERE roles.code = 'koordinator_lapangan'
  AND permissions.code IN (
      'dashboard.view', 'programs.view',
      'distribution.view', 'distribution.manage', 'distribution.pos_mesin', 'distribution.pos_dokumen', 'distribution.pos_penyerahan', 'documentation.manage',
      'recipients.view', 'activities.view', 'activities.manage', 'bast.view', 'bast.manage'
  )
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM roles
WHERE code IN ('pos_mesin', 'pos_dokumen', 'pos_penyerahan', 'tim_lapangan', 'dokumentasi_kegiatan', 'koordinator_lapangan');
