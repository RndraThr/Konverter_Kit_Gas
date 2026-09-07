-- +goose Up
ALTER TABLE program_schedules ADD COLUMN supervisor_name text;

ALTER TABLE distribution_records
    ADD COLUMN machine_option_code text,
    ADD COLUMN machine_serial_number text,
    ADD COLUMN hose_option_code text,
    ADD COLUMN hose_serial_number text,
    ADD COLUMN converter_serial_number text;

UPDATE package_template_versions
SET values_json = values_json || '{"machine_options":[{"code":"shark-spwp8030","brand":"SHARK","type":"SPWP 80-30/3\""}],"hose_options":[{"code":"triliunhose-yamakoyo","brand":"TRILIUNHOSE/YAMAKOYO","spec":"Panjang Selang Hisap: 6m, Panjang Selang Buang: 10m"}]}'::jsonb
WHERE template_code IN ('PETANI-LPG', 'NELAYAN-LPG') AND version = 1;

-- +goose Down
UPDATE package_template_versions
SET values_json = values_json - 'machine_options' - 'hose_options'
WHERE template_code IN ('PETANI-LPG', 'NELAYAN-LPG') AND version = 1;

ALTER TABLE distribution_records
    DROP COLUMN machine_option_code,
    DROP COLUMN machine_serial_number,
    DROP COLUMN hose_option_code,
    DROP COLUMN hose_serial_number,
    DROP COLUMN converter_serial_number;

ALTER TABLE program_schedules DROP COLUMN supervisor_name;
