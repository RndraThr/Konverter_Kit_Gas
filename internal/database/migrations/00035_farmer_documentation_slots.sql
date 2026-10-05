-- +goose Up
-- Replace the canonical farmer documentation template with the approved three-POS photo flow.
-- Existing distribution slots are snapshots and intentionally remain unchanged.
DELETE FROM documentation_template_slots
WHERE template_version_id IN (
    SELECT id FROM documentation_template_versions WHERE template_code = 'DOK-PETANI'
);

INSERT INTO documentation_template_slots
    (template_version_id, slot_code, label, stage, is_required, min_files, max_files,
     input_source, require_location, require_captured_at, instructions, sort_order)
SELECT templates.id, slots.slot_code, slots.label, slots.stage, slots.is_required, 1, 1,
       'both', false, false, NULL, slots.sort_order
FROM documentation_template_versions AS templates
CROSS JOIN (VALUES
    ('new_machine_with_number',
     'Foto Mesin Baru dan Nomor Urut',
     'mesin', true, 10),
    ('new_machine_serial_with_number',
     'Foto Nomor Seri Mesin Baru dan Nomor Urut',
     'mesin', true, 20),

    ('recipient_id_card',
     'Foto KTP dan Nomor Urut',
     'dokumen', true, 30),
    ('family_card',
     'Foto KK dan Nomor Urut',
     'dokumen', true, 40),
    ('farmer_card_or_certificate',
     'Foto Kartu Tani atau Surat Keterangan dan Nomor Urut',
     'dokumen', true, 50),
    ('land_area_certificate',
     'Foto Surat Keterangan Luas Lahan dan Nomor Urut',
     'dokumen', true, 60),
    ('recipient_with_id_card',
     'Foto Penerima dengan KTP dan Nomor Urut',
     'dokumen', true, 70),
    ('profession_certificate',
     'Foto Surat Keterangan Profesi dan Nomor Urut (apabila status pekerjaan di KTP bukan petani)',
     'dokumen', false, 80),

    ('recipient_with_package',
     'Foto Penerima dengan Paket Distribusi beserta Toolkit, Manual Book, Kartu Garansi, dan Nomor Urut (di depan banner backdrop)',
     'penyerahan', true, 90),
    ('recipient_with_technician',
     'Foto Penerima dengan Teknisi dan Nomor Urut (di depan banner backdrop)',
     'penyerahan', true, 100),
    ('recipient_training',
     'Foto Penerima untuk Training dan Nomor Urut (di depan banner pelatihan teknis)',
     'penyerahan', true, 110),
    ('recipient_with_old_machine',
     'Foto Penerima dengan Mesin Lama dan Nomor Urut',
     'penyerahan', true, 120),
    ('old_machine_serial',
     'Foto Nomor Seri Mesin Lama dan Nomor Urut',
     'penyerahan', false, 130)
) AS slots(slot_code, label, stage, is_required, sort_order)
WHERE templates.template_code = 'DOK-PETANI';

-- +goose Down
DELETE FROM documentation_template_slots
WHERE template_version_id IN (
    SELECT id FROM documentation_template_versions WHERE template_code = 'DOK-PETANI'
);

INSERT INTO documentation_template_slots
    (template_version_id, slot_code, label, stage, is_required, min_files, max_files,
     input_source, require_location, require_captured_at, instructions, sort_order)
SELECT templates.id, slots.slot_code, slots.label, slots.stage, true, 1, 1,
       'both', false, false, NULL, slots.sort_order
FROM documentation_template_versions AS templates
CROSS JOIN (VALUES
    ('recipient_package', 'Penerima dan paket', 'dokumen', 10),
    ('machine_serial', 'Serial number mesin', 'mesin', 20),
    ('package_completeness', 'Kelengkapan paket', 'penyerahan', 30),
    ('signed_bast', 'BAST bertanda tangan', 'penyerahan', 40)
) AS slots(slot_code, label, stage, sort_order)
WHERE templates.template_code = 'DOK-PETANI';
