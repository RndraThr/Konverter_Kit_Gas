-- +goose Up
-- Permanently configure the official KONKIT-2026 tender zones and regency quotas.
-- The mapping is keyed by the stable three-letter regency document code.
-- +goose StatementBegin
DO $$
DECLARE
    konkit_program_id uuid;
    zone_1_id uuid;
    zone_2_id uuid;
    assigned_zone_1 integer;
    assigned_zone_2 integer;
BEGIN
    SELECT id INTO konkit_program_id
    FROM programs
    WHERE code = 'KONKIT-2026';

    IF konkit_program_id IS NULL THEN
        RAISE EXCEPTION 'seed 00022: program KONKIT-2026 not found';
    END IF;

    INSERT INTO program_zones (program_id, code, name, sort_order, is_placeholder)
    VALUES
        (konkit_program_id, 'ZONA-1', 'ZONA 1', 10, false),
        (konkit_program_id, 'ZONA-2', 'ZONA 2', 20, false)
    ON CONFLICT (program_id, code) DO UPDATE
    SET name = EXCLUDED.name,
        sort_order = EXCLUDED.sort_order,
        updated_at = now();

    SELECT id INTO STRICT zone_1_id
    FROM program_zones
    WHERE program_id = konkit_program_id AND code = 'ZONA-1';

    SELECT id INTO STRICT zone_2_id
    FROM program_zones
    WHERE program_id = konkit_program_id AND code = 'ZONA-2';

    WITH official_zones(document_code, zone_id, quota) AS (
        VALUES
            -- Zona 1: 23 kabupaten/kota, 4.470 unit.
            ('LNG', zone_1_id, 160), ('BRN', zone_1_id, 160), ('SBG', zone_1_id, 100),
            ('DLS', zone_1_id, 220), ('TDT', zone_1_id, 210), ('SJJ', zone_1_id, 110),
            ('PDP', zone_1_id, 180), ('IRH', zone_1_id, 200), ('IRL', zone_1_id, 160),
            ('SIK', zone_1_id, 160), ('RHL', zone_1_id, 160), ('KPR', zone_1_id, 280),
            ('CLP', zone_1_id, 225), ('BMS', zone_1_id, 225), ('GRB', zone_1_id, 160),
            ('BLR', zone_1_id, 160), ('WNG', zone_1_id, 320), ('BBS', zone_1_id, 230),
            ('TGL', zone_1_id, 110), ('JPR', zone_1_id, 300), ('KBM', zone_1_id, 200),
            ('PBG', zone_1_id, 120), ('PML', zone_1_id, 320),
            -- Zona 2: 23 kabupaten/kota, 4.770 unit.
            ('OKI', zone_2_id, 170), ('OKU', zone_2_id, 100), ('OKT', zone_2_id, 50),
            ('TJT', zone_2_id, 170), ('MJB', zone_2_id, 150), ('JMB', zone_2_id, 192),
            ('KRC', zone_2_id, 298), ('MRG', zone_2_id, 150), ('LGS', zone_2_id, 300),
            ('LGT', zone_2_id, 293), ('LTM', zone_2_id, 27), ('BKA', zone_2_id, 300),
            ('BKB', zone_2_id, 200), ('JBR', zone_2_id, 300), ('LMJ', zone_2_id, 150),
            ('PCT', zone_2_id, 470), ('PNG', zone_2_id, 170), ('BKL', zone_2_id, 320),
            ('NGJ', zone_2_id, 150), ('MJK', zone_2_id, 170), ('TBN', zone_2_id, 220),
            ('BJN', zone_2_id, 100), ('MLG', zone_2_id, 320)
    ), resolved AS (
        SELECT r.id AS regency_id, official_zones.zone_id, official_zones.quota
        FROM official_zones
        JOIN regencies r ON r.document_code = official_zones.document_code
    )
    INSERT INTO program_regency_assignments (program_id, regency_id, zone_id)
    SELECT konkit_program_id, regency_id, zone_id
    FROM resolved
    ON CONFLICT (program_id, regency_id) DO UPDATE
    SET zone_id = EXCLUDED.zone_id,
        updated_at = now();

    WITH official_quotas(document_code, quota) AS (
        VALUES
            ('LNG', 160), ('BRN', 160), ('SBG', 100), ('DLS', 220), ('TDT', 210), ('SJJ', 110),
            ('PDP', 180), ('IRH', 200), ('IRL', 160), ('SIK', 160), ('RHL', 160), ('KPR', 280),
            ('CLP', 225), ('BMS', 225), ('GRB', 160), ('BLR', 160), ('WNG', 320), ('BBS', 230),
            ('TGL', 110), ('JPR', 300), ('KBM', 200), ('PBG', 120), ('PML', 320),
            ('OKI', 170), ('OKU', 100), ('OKT', 50), ('TJT', 170), ('MJB', 150), ('JMB', 192),
            ('KRC', 298), ('MRG', 150), ('LGS', 300), ('LGT', 293), ('LTM', 27), ('BKA', 300),
            ('BKB', 200), ('JBR', 300), ('LMJ', 150), ('PCT', 470), ('PNG', 170), ('BKL', 320),
            ('NGJ', 150), ('MJK', 170), ('TBN', 220), ('BJN', 100), ('MLG', 320)
    )
    UPDATE program_schedules schedules
    SET slot_quota = official_quotas.quota,
        updated_at = now()
    FROM official_quotas
    JOIN regencies ON regencies.document_code = official_quotas.document_code
    WHERE schedules.program_id = konkit_program_id
      AND schedules.regency_id = regencies.id;

    SELECT count(*) INTO assigned_zone_1
    FROM program_regency_assignments
    WHERE program_id = konkit_program_id AND zone_id = zone_1_id;

    SELECT count(*) INTO assigned_zone_2
    FROM program_regency_assignments
    WHERE program_id = konkit_program_id AND zone_id = zone_2_id;

    IF assigned_zone_1 <> 23 OR assigned_zone_2 <> 23 THEN
        RAISE EXCEPTION 'seed 00022: invalid zone assignments (ZONA-1 %, ZONA-2 %, expected 23 each)',
            assigned_zone_1, assigned_zone_2;
    END IF;

    IF EXISTS (
        WITH official_quotas(document_code, quota) AS (
            VALUES
                ('LNG', 160), ('BRN', 160), ('SBG', 100), ('DLS', 220), ('TDT', 210), ('SJJ', 110),
                ('PDP', 180), ('IRH', 200), ('IRL', 160), ('SIK', 160), ('RHL', 160), ('KPR', 280),
                ('CLP', 225), ('BMS', 225), ('GRB', 160), ('BLR', 160), ('WNG', 320), ('BBS', 230),
                ('TGL', 110), ('JPR', 300), ('KBM', 200), ('PBG', 120), ('PML', 320),
                ('OKI', 170), ('OKU', 100), ('OKT', 50), ('TJT', 170), ('MJB', 150), ('JMB', 192),
                ('KRC', 298), ('MRG', 150), ('LGS', 300), ('LGT', 293), ('LTM', 27), ('BKA', 300),
                ('BKB', 200), ('JBR', 300), ('LMJ', 150), ('PCT', 470), ('PNG', 170), ('BKL', 320),
                ('NGJ', 150), ('MJK', 170), ('TBN', 220), ('BJN', 100), ('MLG', 320)
        )
        SELECT 1
        FROM official_quotas
        JOIN regencies ON regencies.document_code = official_quotas.document_code
        WHERE NOT EXISTS (
            SELECT 1
            FROM program_schedules schedules
            WHERE schedules.program_id = konkit_program_id
              AND schedules.regency_id = regencies.id
              AND schedules.slot_quota = official_quotas.quota
        )
    ) THEN
        RAISE EXCEPTION 'seed 00022: one or more KONKIT-2026 schedules are missing or have an invalid quota';
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
DECLARE
    konkit_program_id uuid;
    placeholder_zone_id uuid;
BEGIN
    SELECT id INTO konkit_program_id FROM programs WHERE code = 'KONKIT-2026';
    IF konkit_program_id IS NULL THEN
        RETURN;
    END IF;

    SELECT id INTO placeholder_zone_id
    FROM program_zones
    WHERE program_id = konkit_program_id AND is_placeholder = true;

    UPDATE program_regency_assignments assignments
    SET zone_id = placeholder_zone_id,
        updated_at = now()
    FROM program_zones zones
    WHERE assignments.program_id = konkit_program_id
      AND assignments.zone_id = zones.id
      AND zones.code IN ('ZONA-1', 'ZONA-2');

    DELETE FROM program_zones
    WHERE program_id = konkit_program_id AND code IN ('ZONA-1', 'ZONA-2');
END $$;
-- +goose StatementEnd
