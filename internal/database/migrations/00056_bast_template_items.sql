-- +goose Up
-- Realisasi TKDN dan BA Pemeriksaan membaca merk dan % TKDN dari barang
-- Template Paket (opsi mesin/konkit/selang dan komponen). Susunan dokumen
-- (nama baris, kelompok, form) tetap di profil BA; No. PO per zona per
-- barang+merk disimpan terpisah karena tidak ikut versi template.
CREATE TABLE bast_zone_po_numbers (
    zone_id uuid NOT NULL REFERENCES program_zones(id) ON DELETE CASCADE,
    item_kind text NOT NULL CHECK (item_kind IN ('machine', 'converter', 'hose_suction', 'hose_discharge', 'component')),
    item_code text NOT NULL CHECK (btrim(item_code) <> ''),
    po_number text NOT NULL CHECK (btrim(po_number) <> ''),
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (zone_id, item_kind, item_code)
);

CREATE TABLE bast_schedule_item_variants (
    schedule_id uuid PRIMARY KEY REFERENCES program_schedules(id) ON DELETE CASCADE,
    selection_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(selection_json) = 'object'),
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE bast_schedule_settings DROP COLUMN pemeriksaan_po_number;

-- Susunan TKDN lama (barang + merk + %) tidak dipakai lagi; susunan baru
-- menunjuk barang template dan diisi bawaan saat kosong.
UPDATE bast_tkdn_profiles SET items_json = '[]'::jsonb;

-- Lengkapi template Petani dengan % TKDN dan merk komponen sesuai dokumen
-- referensi Realisasi TKDN, serta komponen Isi LPG dan Plat Anti Karat yang
-- hanya dipakai TKDN (tidak tampil di checklist BA serah terima).
UPDATE package_template_versions SET values_json = values_json
    || jsonb_build_object('machine_options', COALESCE((
        SELECT jsonb_agg(o || jsonb_build_object('tkdn_percent', CASE WHEN upper(o->>'brand') = 'SHARK' THEN 61.42 ELSE 0 END) ORDER BY n)
        FROM jsonb_array_elements(values_json->'machine_options') WITH ORDINALITY AS m(o, n)), '[]'::jsonb))
    || jsonb_build_object('converter_options', COALESCE((
        SELECT jsonb_agg(o || jsonb_build_object('tkdn_percent', CASE WHEN upper(o->>'brand') = 'ERGAS' THEN 93.27 ELSE 0 END) ORDER BY n)
        FROM jsonb_array_elements(values_json->'converter_options') WITH ORDINALITY AS m(o, n)), '[]'::jsonb))
    || jsonb_build_object('hose_options', COALESCE((
        SELECT jsonb_agg(o || jsonb_build_object('suction_tkdn_percent', 98.45, 'discharge_tkdn_percent', 0) ORDER BY n)
        FROM jsonb_array_elements(values_json->'hose_options') WITH ORDINALITY AS m(o, n)), '[]'::jsonb))
    || jsonb_build_object('components', COALESCE((
        SELECT jsonb_agg(o || CASE o->>'code'
            WHEN 'lpg_tank' THEN '{"brand":"Pertamina","tkdn_percent":58.47}'::jsonb
            WHEN 'regulator' THEN '{"brand":"Top Gas","tkdn_percent":56.73}'::jsonb
            WHEN 'hose_clamp_accessories' THEN '{"brand":"Mondea","tkdn_percent":87.58}'::jsonb
            WHEN 'oil' THEN '{"brand":"Pertamina Enduro","tkdn_percent":53.42}'::jsonb
            WHEN 'bracket' THEN '{"brand":"N.A","tkdn_percent":0}'::jsonb
            ELSE '{}'::jsonb END ORDER BY n)
        FROM jsonb_array_elements(values_json->'components') WITH ORDINALITY AS m(o, n)), '[]'::jsonb)
        || '[{"code":"lpg_refill","label":"Isi LPG 3 Kg","quantity":1,"unit":"Tabung","brand":"-","tkdn_percent":51.99,"handover_hidden":true},
             {"code":"anti_rust_plate","label":"Plat Anti Karat","quantity":1,"unit":"Pcs","brand":"N.A","tkdn_percent":0,"handover_hidden":true}]'::jsonb),
    updated_at = now()
WHERE program_type = 'farmer'
  AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(COALESCE(values_json->'components', '[]'::jsonb)) c WHERE c->>'code' = 'lpg_refill');

-- +goose Down
UPDATE package_template_versions SET values_json = values_json
    || jsonb_build_object('machine_options', COALESCE((SELECT jsonb_agg(o - 'tkdn_percent' ORDER BY n) FROM jsonb_array_elements(values_json->'machine_options') WITH ORDINALITY AS m(o, n)), '[]'::jsonb))
    || jsonb_build_object('converter_options', COALESCE((SELECT jsonb_agg(o - 'tkdn_percent' ORDER BY n) FROM jsonb_array_elements(values_json->'converter_options') WITH ORDINALITY AS m(o, n)), '[]'::jsonb))
    || jsonb_build_object('hose_options', COALESCE((SELECT jsonb_agg(o - 'suction_tkdn_percent' - 'discharge_tkdn_percent' ORDER BY n) FROM jsonb_array_elements(values_json->'hose_options') WITH ORDINALITY AS m(o, n)), '[]'::jsonb))
    || jsonb_build_object('components', COALESCE((SELECT jsonb_agg(o - 'brand' - 'tkdn_percent' - 'handover_hidden' ORDER BY n) FROM jsonb_array_elements(values_json->'components') WITH ORDINALITY AS m(o, n) WHERE o->>'code' NOT IN ('lpg_refill', 'anti_rust_plate')), '[]'::jsonb))
WHERE program_type = 'farmer';

ALTER TABLE bast_schedule_settings ADD COLUMN pemeriksaan_po_number text NOT NULL DEFAULT '';
DROP TABLE bast_schedule_item_variants;
DROP TABLE bast_zone_po_numbers;
