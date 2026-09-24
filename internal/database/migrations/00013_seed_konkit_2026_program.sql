-- +goose Up
INSERT INTO programs (code, name, program_type, fiscal_year, status) VALUES
    ('KONKIT-2026', 'PEMBAGIAN KONVERTER KIT 2026', 'farmer', 2026, 'active')
ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, program_type = EXCLUDED.program_type, fiscal_year = EXCLUDED.fiscal_year;

INSERT INTO package_template_versions (template_code, version, name, program_type, values_json, status, published_at) VALUES
    ('KONKIT-2026', 1, 'PAKET KONVERTER KIT 2026', 'farmer', '{
        "converter_brand": "ERGAS",
        "machine_options": [
            {"code": "shark-spwp8030", "brand": "SHARK", "type": "SPWP 80-30/3\""}
        ],
        "hose_options": [
            {"code": "triliunhose", "brand": "TRILIUNHOSE", "spec": "6M/10M"}
        ],
        "components": [
            {"code": "lpg_tank", "label": "Tabung LPG 3 Kg", "quantity": 1, "unit": "Tabung"},
            {"code": "regulator", "label": "Regulator", "quantity": 1, "unit": "Pcs"},
            {"code": "suction_hose", "label": "Selang Hisap", "quantity": 1, "unit": "Set"},
            {"code": "discharge_hose", "label": "Selang Buang", "quantity": 1, "unit": "Set"},
            {"code": "manual", "label": "Buku Manual", "quantity": 1, "unit": "Pcs"},
            {"code": "mixer_nipple", "label": "Mixer Nipple", "quantity": 1, "unit": "Pcs"},
            {"code": "machine_warranty", "label": "Kartu Garansi Mesin", "quantity": 1, "unit": "Ea"},
            {"code": "converter_warranty", "label": "Kartu Garansi Konkit/Reducer", "quantity": 1, "unit": "Ea"},
            {"code": "oil", "label": "Oli", "quantity": 2, "unit": "Botol"},
            {"code": "bracket", "label": "Bracket", "quantity": 1, "unit": "Ea"}
        ]
    }'::jsonb, 'published', now())
ON CONFLICT (template_code, version) DO UPDATE SET name = EXCLUDED.name, values_json = EXCLUDED.values_json, status = EXCLUDED.status;

-- +goose Down
DELETE FROM package_template_versions WHERE template_code = 'KONKIT-2026' AND version = 1;
DELETE FROM programs WHERE code = 'KONKIT-2026';
