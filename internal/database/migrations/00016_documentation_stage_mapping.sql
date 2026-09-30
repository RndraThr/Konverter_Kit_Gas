-- +goose Up
-- Distribute the seeded documentation slots across the three POS stations so each station shows the
-- evidence captured there, instead of every photo landing at POS Penyerahan. Only the template slots
-- are changed; already-created distribution slots keep their snapshot taken at creation time.
--   machine_serial       -> mesin       (foto serial mesin dicatat di POS Mesin)
--   recipient_package    -> dokumen     (foto penerima & paket saat penerima dihubungkan)
--   package_completeness -> penyerahan  (kelengkapan paket saat serah terima)
--   signed_bast          -> penyerahan  (BAST bertanda tangan saat serah terima)
UPDATE documentation_template_slots dts
SET stage = mapping.stage
FROM (VALUES
    ('machine_serial', 'mesin'),
    ('recipient_package', 'dokumen'),
    ('package_completeness', 'penyerahan'),
    ('signed_bast', 'penyerahan')
) AS mapping(slot_code, stage)
WHERE dts.slot_code = mapping.slot_code
  AND dts.template_version_id IN (
      SELECT id FROM documentation_template_versions WHERE template_code IN ('DOK-PETANI', 'DOK-NELAYAN')
  );

-- +goose Down
-- Restore the previous state where every seeded documentation slot lived at POS Penyerahan.
UPDATE documentation_template_slots dts
SET stage = 'penyerahan'
WHERE dts.slot_code IN ('machine_serial', 'recipient_package', 'package_completeness', 'signed_bast')
  AND dts.template_version_id IN (
      SELECT id FROM documentation_template_versions WHERE template_code IN ('DOK-PETANI', 'DOK-NELAYAN')
  );
