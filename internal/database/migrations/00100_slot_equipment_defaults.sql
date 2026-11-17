-- +goose Up
-- New slots start with the package template's default equipment (the option
-- marked default, or the only option). Slots still in progress get the same
-- values where nothing was chosen yet; completed slots keep their record.
WITH defaults AS (
    SELECT s.id AS schedule_id,
        (SELECT COALESCE(
            (SELECT o->>'code' FROM jsonb_array_elements(pt.values_json->list) o WHERE o->>'is_default' = 'true' LIMIT 1),
            CASE WHEN jsonb_array_length(pt.values_json->list) = 1 THEN pt.values_json->list->0->>'code' END)
         FROM (VALUES ('machine_options')) l(list) WHERE jsonb_typeof(pt.values_json->list) = 'array') AS machine,
        (SELECT COALESCE(
            (SELECT o->>'code' FROM jsonb_array_elements(pt.values_json->list) o WHERE o->>'is_default' = 'true' LIMIT 1),
            CASE WHEN jsonb_array_length(pt.values_json->list) = 1 THEN pt.values_json->list->0->>'code' END)
         FROM (VALUES ('converter_options')) l(list) WHERE jsonb_typeof(pt.values_json->list) = 'array') AS converter,
        (SELECT COALESCE(
            (SELECT o->>'code' FROM jsonb_array_elements(pt.values_json->list) o WHERE o->>'is_default' = 'true' LIMIT 1),
            CASE WHEN jsonb_array_length(pt.values_json->list) = 1 THEN pt.values_json->list->0->>'code' END)
         FROM (VALUES ('hose_options')) l(list) WHERE jsonb_typeof(pt.values_json->list) = 'array') AS hose
    FROM program_schedules s
    JOIN package_template_versions pt ON pt.id = s.package_template_version_id
)
UPDATE distribution_slots ds SET
    machine_option_code = COALESCE(ds.machine_option_code, NULLIF(d.machine, '')),
    converter_option_code = COALESCE(ds.converter_option_code, NULLIF(d.converter, '')),
    hose_option_code = COALESCE(ds.hose_option_code, NULLIF(d.hose, '')),
    updated_at = now()
FROM defaults d
WHERE ds.schedule_id = d.schedule_id AND ds.status IN ('open', 'linked')
    AND ((ds.machine_option_code IS NULL AND d.machine IS NOT NULL)
        OR (ds.converter_option_code IS NULL AND d.converter IS NOT NULL)
        OR (ds.hose_option_code IS NULL AND d.hose IS NOT NULL));

-- +goose Down
-- The filled values cannot be told apart from chosen ones, so nothing is undone.
SELECT 1;
