-- +goose Up
-- Make the converter/konkit a per-unit selectable option like the machine and hose, instead of a
-- single fixed package brand. Adds the slot column that records the chosen option and rewrites the
-- template's `converter_brand` string into a `converter_options` list carrying the same brand.
ALTER TABLE distribution_slots ADD COLUMN converter_option_code text;

UPDATE package_template_versions
SET values_json = (values_json - 'converter_brand')
    || jsonb_build_object(
        'converter_options',
        jsonb_build_array(jsonb_build_object(
            'code', lower(values_json->>'converter_brand'),
            'brand', values_json->>'converter_brand'
        ))
    )
WHERE values_json ? 'converter_brand'
  AND NULLIF(values_json->>'converter_brand', '') IS NOT NULL;

-- +goose Down
UPDATE package_template_versions
SET values_json = (values_json - 'converter_options')
    || jsonb_build_object('converter_brand', values_json->'converter_options'->0->>'brand')
WHERE values_json ? 'converter_options'
  AND jsonb_array_length(values_json->'converter_options') > 0;

ALTER TABLE distribution_slots DROP COLUMN converter_option_code;
