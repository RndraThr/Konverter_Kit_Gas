-- +goose Up
-- One schedule per regency for the KONKIT-2026 program, all using the same
-- package/documentation templates and the same start/end window. Guarded
-- with NOT EXISTS (not ON CONFLICT) because program_schedules has no
-- natural unique key — a program can legitimately have multiple schedules
-- per regency across different phases, so this only skips a regency that
-- already has a schedule with this exact generated name.
--
-- The dependency checks below turn a silent zero-row insert into a loud
-- failure: goose only runs this migration once per database, so if the
-- rows it depends on (from migrations 00004/00013) were ever wiped by a
-- manual data reset without also resetting goose's bookkeeping, this
-- would otherwise insert nothing and report success.
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM programs WHERE code = 'KONKIT-2026') THEN
        RAISE EXCEPTION 'seed 00014: program KONKIT-2026 not found (expected from migration 00013)';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM package_template_versions WHERE template_code = 'KONKIT-2026' AND version = 1) THEN
        RAISE EXCEPTION 'seed 00014: package template KONKIT-2026 v1 not found (expected from migration 00013)';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM documentation_template_versions WHERE template_code = 'DOK-PETANI' AND version = 1) THEN
        RAISE EXCEPTION 'seed 00014: documentation template DOK-PETANI v1 not found (expected from migration 00004)';
    END IF;
END $$;
-- +goose StatementEnd

INSERT INTO program_schedules (program_id, regency_id, package_template_version_id, documentation_template_version_id, name, start_date, end_date, status, distribution_number_padding)
SELECT
    p.id,
    r.id,
    pt.id,
    dt.id,
    UPPER(r.name) || ' - KONVERTER KIT 2026',
    DATE '2026-09-24',
    DATE '2026-11-08',
    'active',
    4
FROM regencies r
CROSS JOIN (SELECT id FROM programs WHERE code = 'KONKIT-2026') p
CROSS JOIN (SELECT id FROM package_template_versions WHERE template_code = 'KONKIT-2026' AND version = 1) pt
CROSS JOIN (SELECT id FROM documentation_template_versions WHERE template_code = 'DOK-PETANI' AND version = 1) dt
WHERE r.document_code IN (
    'LNG', 'BRN', 'SBG', 'DLS', 'TDT', 'SJJ', 'PDP', 'IRH', 'IRL', 'SIK', 'RHL', 'KPR',
    'CLP', 'BMS', 'GRB', 'BLR', 'WNG', 'BBS', 'TGL', 'JPR', 'KBM', 'PBG', 'PML',
    'OKI', 'OKU', 'OKT', 'TJT', 'MJB', 'JMB', 'KRC', 'MRG', 'LGS', 'LGT', 'LTM',
    'BKA', 'BKB', 'JBR', 'LMJ', 'PCT', 'PNG', 'BKL', 'NGJ', 'MJK', 'TBN', 'BJN', 'MLG'
)
AND NOT EXISTS (
    SELECT 1 FROM program_schedules ps
    WHERE ps.program_id = p.id AND ps.regency_id = r.id AND ps.name = UPPER(r.name) || ' - KONVERTER KIT 2026'
);

-- +goose Down
DELETE FROM program_schedules
WHERE program_id = (SELECT id FROM programs WHERE code = 'KONKIT-2026')
  AND name LIKE '% - KONVERTER KIT 2026'
  AND start_date = DATE '2026-09-24'
  AND end_date = DATE '2026-11-08';
