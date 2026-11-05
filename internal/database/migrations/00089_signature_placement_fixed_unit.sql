-- +goose Up
-- Posisi tanda tangan/stempel kini dalam satuan tetap 28 mm (tinggi kotak
-- tanda tangan sebagian besar BA), bukan tinggi kotak masing-masing BA, agar
-- ukuran dan letaknya sama di semua BA. Aturan khusus BA yang kotaknya lain
-- (BA Perorangan 15,5 mm, Servis Berkala 29 mm) dikonversi supaya hasil
-- cetaknya tidak berubah. Aturan global dibuat di BA berkotak 28 mm.
-- +goose StatementBegin
CREATE FUNCTION pg_temp.scale_placement(placement jsonb, factor numeric) RETURNS jsonb AS $$
    SELECT CASE WHEN placement IS NULL OR jsonb_typeof(placement) <> 'object' THEN placement
    ELSE placement || jsonb_build_object(
        'x', round((placement->>'x')::numeric * factor, 3),
        'y', round((placement->>'y')::numeric * factor, 3),
        'size', round((placement->>'size')::numeric * factor, 3))
    END
$$ LANGUAGE sql IMMUTABLE;
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE bast_signature_rules r
SET rules = (
    SELECT jsonb_object_agg(role, CASE WHEN jsonb_typeof(rule) <> 'object' THEN rule ELSE
        jsonb_strip_nulls(rule || jsonb_build_object(
            'signature_placement', pg_temp.scale_placement(rule->'signature_placement', f.factor),
            'stamp_placement', pg_temp.scale_placement(rule->'stamp_placement', f.factor)))
        END)
    FROM jsonb_each(r.rules) AS e(role, rule)
)
FROM (VALUES ('ba-perorangan', 15.5 / 28.0), ('servis-berkala', 29.0 / 28.0)) AS f(document_key, factor)
WHERE r.document_key = f.document_key AND r.rules::text LIKE '%placement%';
-- +goose StatementEnd

-- +goose Down
SELECT 1;
