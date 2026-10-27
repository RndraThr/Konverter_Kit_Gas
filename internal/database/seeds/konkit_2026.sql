-- Persiapan data for the KONKIT-2026 program: the program, its tender zones, every regency's zone
-- assignment, and one schedule per regency with its official slot quota. It mirrors what migrations
-- 00013/00014/00017/00022 seeded once, so a wiped database can get it back with `go run ./cmd/seed`.
--
-- Only missing rows are inserted; anything already there (including edits made in Persiapan) is left
-- alone, so running it twice is harmless. The program's tender logos come from the
-- programs_seed_builtin_ba_logos trigger (migration 00072).
-- Depends on master data kept across resets: regencies (00012), package template KONKIT-2026 v1
-- (00013/00026) and documentation template DOK-PETANI v1 (00004).
DO $$
DECLARE
    konkit_program_id uuid;
    package_id uuid;
    documentation_id uuid;
    missing text;
BEGIN
    SELECT id INTO package_id FROM package_template_versions WHERE template_code = 'KONKIT-2026' AND version = 1;
    IF package_id IS NULL THEN
        RAISE EXCEPTION 'seed: package template KONKIT-2026 v1 not found';
    END IF;
    SELECT id INTO documentation_id FROM documentation_template_versions WHERE template_code = 'DOK-PETANI' AND version = 1;
    IF documentation_id IS NULL THEN
        RAISE EXCEPTION 'seed: documentation template DOK-PETANI v1 not found';
    END IF;

    INSERT INTO programs (code, name, program_type, fiscal_year, status)
    VALUES ('KONKIT-2026', 'PEMBAGIAN KONVERTER KIT 2026', 'farmer', 2026, 'active')
    ON CONFLICT (code) DO NOTHING;
    SELECT id INTO STRICT konkit_program_id FROM programs WHERE code = 'KONKIT-2026';

    INSERT INTO program_zones (program_id, code, name, sort_order, is_placeholder)
    VALUES
        (konkit_program_id, 'UNASSIGNED', 'ZONA BELUM DIATUR', 0, true),
        (konkit_program_id, 'ZONA-1', 'ZONA 1', 10, false),
        (konkit_program_id, 'ZONA-2', 'ZONA 2', 20, false)
    ON CONFLICT (program_id, code) DO NOTHING;

    CREATE TEMP TABLE seed_konkit_regencies (document_code text PRIMARY KEY, zone_code text NOT NULL, quota integer NOT NULL) ON COMMIT DROP;
    INSERT INTO seed_konkit_regencies (document_code, zone_code, quota) VALUES
        -- Zona 1: 23 kabupaten/kota, 4.470 unit.
        ('LNG', 'ZONA-1', 160), ('BRN', 'ZONA-1', 160), ('SBG', 'ZONA-1', 100), ('DLS', 'ZONA-1', 220),
        ('TDT', 'ZONA-1', 210), ('SJJ', 'ZONA-1', 110), ('PDP', 'ZONA-1', 180), ('IRH', 'ZONA-1', 200),
        ('IRL', 'ZONA-1', 160), ('SIK', 'ZONA-1', 160), ('RHL', 'ZONA-1', 160), ('KPR', 'ZONA-1', 280),
        ('CLP', 'ZONA-1', 225), ('BMS', 'ZONA-1', 225), ('GRB', 'ZONA-1', 160), ('BLR', 'ZONA-1', 160),
        ('WNG', 'ZONA-1', 320), ('BBS', 'ZONA-1', 230), ('TGL', 'ZONA-1', 110), ('JPR', 'ZONA-1', 300),
        ('KBM', 'ZONA-1', 200), ('PBG', 'ZONA-1', 120), ('PML', 'ZONA-1', 320),
        -- Zona 2: 23 kabupaten/kota, 4.770 unit.
        ('OKI', 'ZONA-2', 170), ('OKU', 'ZONA-2', 100), ('OKT', 'ZONA-2', 50), ('TJT', 'ZONA-2', 170),
        ('MJB', 'ZONA-2', 150), ('JMB', 'ZONA-2', 192), ('KRC', 'ZONA-2', 298), ('MRG', 'ZONA-2', 150),
        ('LGS', 'ZONA-2', 300), ('LGT', 'ZONA-2', 293), ('LTM', 'ZONA-2', 27), ('BKA', 'ZONA-2', 300),
        ('BKB', 'ZONA-2', 200), ('JBR', 'ZONA-2', 300), ('LMJ', 'ZONA-2', 150), ('PCT', 'ZONA-2', 470),
        ('PNG', 'ZONA-2', 170), ('BKL', 'ZONA-2', 320), ('NGJ', 'ZONA-2', 150), ('MJK', 'ZONA-2', 170),
        ('TBN', 'ZONA-2', 220), ('BJN', 'ZONA-2', 100), ('MLG', 'ZONA-2', 320);

    SELECT string_agg(s.document_code, ', ' ORDER BY s.document_code) INTO missing
    FROM seed_konkit_regencies s
    WHERE NOT EXISTS (SELECT 1 FROM regencies r WHERE r.document_code = s.document_code);
    IF missing IS NOT NULL THEN
        RAISE EXCEPTION 'seed: regencies not found: %', missing;
    END IF;

    INSERT INTO program_regency_assignments (program_id, regency_id, zone_id)
    SELECT konkit_program_id, r.id, z.id
    FROM seed_konkit_regencies s
    JOIN regencies r ON r.document_code = s.document_code
    JOIN program_zones z ON z.program_id = konkit_program_id AND z.code = s.zone_code
    ON CONFLICT (program_id, regency_id) DO NOTHING;

    INSERT INTO program_schedules (program_id, regency_id, package_template_version_id, documentation_template_version_id, name, start_date, end_date, status, slot_quota)
    SELECT konkit_program_id, r.id, package_id, documentation_id, UPPER(r.name) || ' - KONVERTER KIT 2026',
           DATE '2026-09-24', DATE '2026-11-08', 'active', s.quota
    FROM seed_konkit_regencies s
    JOIN regencies r ON r.document_code = s.document_code
    WHERE NOT EXISTS (SELECT 1 FROM program_schedules ps WHERE ps.program_id = konkit_program_id AND ps.regency_id = r.id);
END $$;
