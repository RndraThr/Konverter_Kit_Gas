-- +goose Up
-- Oli diserahkan per botol (2 botol Pertamina Enduro), bukan per liter.
-- Satuan komponen oli di semua versi template paket menjadi "Botol".
UPDATE package_template_versions t
SET values_json = jsonb_set(t.values_json, '{components}', (
        SELECT jsonb_agg(CASE WHEN c->>'code' = 'oil' AND c->>'unit' IN ('Liter', 'Ltr', 'LTR') THEN c || '{"unit": "Botol"}' ELSE c END ORDER BY ord)
        FROM jsonb_array_elements(t.values_json->'components') WITH ORDINALITY AS e(c, ord)
    ))
WHERE EXISTS (
    SELECT 1 FROM jsonb_array_elements(t.values_json->'components') AS c
    WHERE c->>'code' = 'oil' AND c->>'unit' IN ('Liter', 'Ltr', 'LTR')
);

-- BA yang sudah dibuat dengan satuan liter dibuat ulang saat dibuka lagi.
UPDATE bast_daily_bundles b SET status = 'stale', updated_at = now()
WHERE b.status = 'active' AND EXISTS (
    SELECT 1 FROM bast_daily_bundle_items bi
    JOIN bast_individual_documents d ON d.id = bi.individual_document_id
    WHERE bi.bundle_id = b.id AND d.status = 'final' AND d.snapshot_json::text LIKE '%"Liter"%'
);
UPDATE bast_individual_documents SET status = 'stale'
WHERE status = 'final' AND snapshot_json::text LIKE '%"Liter"%';
UPDATE bast_aggregate_documents SET status = 'stale', updated_at = now()
WHERE status = 'active' AND snapshot_json::text LIKE '%"Liter"%';

-- +goose Down
UPDATE package_template_versions t
SET values_json = jsonb_set(t.values_json, '{components}', (
        SELECT jsonb_agg(CASE WHEN c->>'code' = 'oil' AND c->>'unit' = 'Botol' THEN c || '{"unit": "Liter"}' ELSE c END ORDER BY ord)
        FROM jsonb_array_elements(t.values_json->'components') WITH ORDINALITY AS e(c, ord)
    ))
WHERE EXISTS (
    SELECT 1 FROM jsonb_array_elements(t.values_json->'components') AS c
    WHERE c->>'code' = 'oil' AND c->>'unit' = 'Botol'
);
