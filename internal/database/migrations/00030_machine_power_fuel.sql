-- +goose Up
-- Setiap machine_option paket Petani/Nelayan kini membawa `power` (Daya) dan `fuel_type`
-- (Jenis BBM) agar snapshot alokasi DP3/Rekap Harian dapat menyimpan data mesin lengkap
-- per penerima tanpa lookup ke template terkini. Nilai default mengikuti varian mesin tunggal
-- yang sudah di-seed; tetap dapat diedit lewat form template paket.
UPDATE package_template_versions
SET values_json = jsonb_set(
        values_json,
        '{machine_options}',
        (
            SELECT jsonb_agg(
                CASE
                    WHEN option->>'code' = 'shark-spwp8030'
                        THEN option || jsonb_build_object('power', '5.5 HP', 'fuel_type', 'Bensin')
                    ELSE option
                END
                ORDER BY ordinal
            )
            FROM jsonb_array_elements(values_json->'machine_options') WITH ORDINALITY AS machine(option, ordinal)
        ),
        true
    ),
    updated_at = now()
WHERE (template_code IN ('KONKIT-2026', 'PETANI-LPG', 'NELAYAN-LPG') AND version = 1)
  AND jsonb_typeof(values_json->'machine_options') = 'array'
  AND EXISTS (
      SELECT 1
      FROM jsonb_array_elements(values_json->'machine_options') AS machine(option)
      WHERE option->>'code' = 'shark-spwp8030'
  );

-- +goose Down
-- Kembalikan machine_options tanpa power/fuel_type (nilai label tidak dapat direkonstruksi dari
-- template yang sudah berubah, sehingga rollback mengembalikan ke bentuk seed awal).
UPDATE package_template_versions
SET values_json = jsonb_set(
        values_json,
        '{machine_options}',
        (
            SELECT jsonb_agg(
                CASE
                    WHEN option->>'code' = 'shark-spwp8030'
                         AND option->>'power' = '5.5 HP'
                         AND option->>'fuel_type' = 'Bensin'
                        THEN option - 'power' - 'fuel_type'
                    ELSE option
                END
                ORDER BY ordinal
            )
            FROM jsonb_array_elements(values_json->'machine_options') WITH ORDINALITY AS machine(option, ordinal)
        ),
        true
    ),
    updated_at = now()
WHERE (template_code IN ('KONKIT-2026', 'PETANI-LPG', 'NELAYAN-LPG') AND version = 1)
  AND jsonb_typeof(values_json->'machine_options') = 'array'
  AND EXISTS (
      SELECT 1
      FROM jsonb_array_elements(values_json->'machine_options') AS machine(option)
      WHERE option->>'code' = 'shark-spwp8030'
  );
