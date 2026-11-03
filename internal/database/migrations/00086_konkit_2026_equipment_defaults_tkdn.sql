-- +goose Up
-- Konverter Kit 2026: semua jadwal memakai template paket KONKIT-2026 v4
-- (punya opsi selang Figo). Template v4 menandai opsi default yang terisi
-- otomatis saat distribusi (mesin Shark, konkit ERGAS tipe ECK-300, selang
-- Figo) dan memuat % TKDN perhitungan Komponen Biaya Barang (Jumlah Biaya
-- Barang 80,87%).
-- Merk konkit ERGAS ditulis kapital di semua template. Jadwal yang sudah
-- memilih Triliunhose tetap valid: kodenya ada di v4.
UPDATE program_schedules s
SET package_template_version_id = v4.id, updated_at = now()
FROM package_template_versions v1, package_template_versions v4
WHERE v1.template_code = 'KONKIT-2026' AND v1.version = 1
  AND v4.template_code = 'KONKIT-2026' AND v4.version = 4
  AND s.package_template_version_id = v1.id;

UPDATE package_template_versions
SET values_json = jsonb_set(values_json, '{converter_options}', (
        SELECT jsonb_agg(CASE WHEN upper(btrim(o->>'brand')) = 'ERGAS' THEN jsonb_set(o, '{brand}', '"ERGAS"') ELSE o END ORDER BY n)
        FROM jsonb_array_elements(values_json->'converter_options') WITH ORDINALITY AS e(o, n))),
    updated_at = now()
WHERE jsonb_typeof(values_json->'converter_options') = 'array'
  AND jsonb_array_length(values_json->'converter_options') > 0;

UPDATE package_template_versions
SET values_json = values_json
    || jsonb_build_object('machine_options', (
        SELECT jsonb_agg(CASE WHEN o->>'code' = 'shark-spwp8030'
            THEN o || '{"tkdn_percent": 82.95, "is_default": true}' ELSE o - 'is_default' END ORDER BY n)
        FROM jsonb_array_elements(values_json->'machine_options') WITH ORDINALITY AS e(o, n)))
    || jsonb_build_object('converter_options', (
        SELECT jsonb_agg(CASE WHEN o->>'code' = 'ergas'
            THEN o || '{"brand": "ERGAS", "spec": "ECK-300", "tkdn_percent": 93.72, "is_default": true}' ELSE o - 'is_default' END ORDER BY n)
        FROM jsonb_array_elements(values_json->'converter_options') WITH ORDINALITY AS e(o, n)))
    || jsonb_build_object('hose_options', (
        SELECT jsonb_agg(CASE WHEN o->>'code' = 'figo'
            THEN o || '{"suction_tkdn_percent": 97.06, "discharge_tkdn_percent": 93.95, "is_default": true}' ELSE o - 'is_default' END ORDER BY n)
        FROM jsonb_array_elements(values_json->'hose_options') WITH ORDINALITY AS e(o, n)))
    || jsonb_build_object('components', (
        SELECT jsonb_agg(CASE o->>'code'
            WHEN 'regulator' THEN o || '{"tkdn_percent": 80.33}'
            WHEN 'hose_clamp_accessories' THEN o || '{"tkdn_percent": 0}'
            WHEN 'lpg_tank' THEN o || '{"tkdn_percent": 0}'
            WHEN 'lpg_refill' THEN o || '{"tkdn_percent": 0}'
            WHEN 'bracket' THEN o || '{"tkdn_percent": 0}'
            WHEN 'anti_rust_plate' THEN o || '{"tkdn_percent": 0}'
            WHEN 'oil' THEN o || '{"tkdn_percent": 0}'
            ELSE o END ORDER BY n)
        FROM jsonb_array_elements(values_json->'components') WITH ORDINALITY AS e(o, n))),
    updated_at = now()
WHERE template_code = 'KONKIT-2026' AND version = 4
  AND jsonb_array_length(values_json->'machine_options') > 0
  AND jsonb_array_length(values_json->'converter_options') > 0
  AND jsonb_array_length(values_json->'hose_options') > 0
  AND jsonb_array_length(values_json->'components') > 0;

-- +goose Down
-- Jadwal tetap di v4 dan nilai TKDN/default tidak dikembalikan: keduanya
-- data yang sudah dipakai lapangan; kembalikan dari backup bila perlu.
SELECT 1;
