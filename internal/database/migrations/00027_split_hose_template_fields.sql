-- +goose Up
UPDATE package_template_versions
SET values_json = jsonb_set(
        jsonb_set(values_json, '{hose_options}', '[
            {
                "code":"triliunhose",
                "suction_brand":"TRILLIUNHOSE",
                "suction_spec":"6 M",
                "discharge_brand":"YAMAKOYO",
                "discharge_spec":"10 M"
            }
        ]'::jsonb, true),
        '{components}', '[
            {"code":"lpg_tank","label":"Tabung LPG 3 Kg","quantity":1,"unit":"Tabung"},
            {"code":"regulator","label":"Regulator","quantity":1,"unit":"Pcs"},
            {"code":"hose_clamp_accessories","label":"Selang, Clamp & Aksesorisnya","quantity":1,"unit":"Set"},
            {"code":"manual","label":"Buku Manual","quantity":1,"unit":"Pcs"},
            {"code":"mixer_nipple","label":"Mixer Nipple","quantity":1,"unit":"Pcs"},
            {"code":"machine_warranty","label":"Kartu Garansi Mesin","quantity":1,"unit":"Ea"},
            {"code":"converter_warranty","label":"Kartu Garansi Konkit/Reducer","quantity":1,"unit":"Ea"},
            {"code":"oil","label":"OLI","quantity":2,"unit":"Ltr"},
            {"code":"bracket","label":"Braket","quantity":1,"unit":"Ea"}
        ]'::jsonb,
        true
    ),
    updated_at = now()
WHERE template_code = 'KONKIT-2026' AND version = 1;

-- +goose Down
UPDATE package_template_versions
SET values_json = jsonb_set(
        jsonb_set(values_json, '{hose_options}', '[
            {"code":"triliunhose","brand":"TRILIUNHOSE","spec":"6M/10M"}
        ]'::jsonb, true),
        '{components}', '[
            {"code":"lpg_tank","label":"Tabung LPG 3 Kg","quantity":1,"unit":"Tabung"},
            {"code":"regulator","label":"Regulator","quantity":1,"unit":"Pcs"},
            {"code":"hose_clamp_accessories","label":"Selang, Clamp & Aksesorisnya","quantity":1,"unit":"Set"},
            {"code":"manual","label":"Buku Manual","quantity":1,"unit":"Pcs"},
            {"code":"mixer_nipple","label":"Mixer Nipple","quantity":1,"unit":"Pcs"},
            {"code":"machine_warranty","label":"Kartu Garansi Mesin","quantity":1,"unit":"Ea"},
            {"code":"converter_warranty","label":"Kartu Garansi Konkit/Reducer","quantity":1,"unit":"Ea"},
            {"code":"oil","label":"Oli","quantity":2,"unit":"Botol"},
            {"code":"bracket","label":"Bracket","quantity":1,"unit":"Ea"}
        ]'::jsonb,
        true
    ),
    updated_at = now()
WHERE template_code = 'KONKIT-2026' AND version = 1;
