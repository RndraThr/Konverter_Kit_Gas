-- +goose Up
-- Set each KONKIT-2026 schedule's slot_quota to the per-kabupaten distribution total from the
-- official "Pembagian Konverter Kit 2026" plan (Zona 1: 23 kab = 4.470 unit, Zona 2: 23 kab = 4.770
-- unit). Keyed by regency document_code so it maps to the right schedule regardless of row order.
UPDATE program_schedules s
SET slot_quota = q.quota
FROM (VALUES
    -- Zona 1 (4.470)
    ('LNG', 160), ('BRN', 160), ('SBG', 100), ('DLS', 220), ('TDT', 210), ('SJJ', 110), ('PDP', 180),
    ('IRH', 200), ('IRL', 160), ('SIK', 160), ('RHL', 160), ('KPR', 280),
    ('CLP', 225), ('BMS', 225), ('GRB', 160), ('BLR', 160), ('WNG', 320), ('BBS', 230), ('TGL', 110),
    ('JPR', 300), ('KBM', 200), ('PBG', 120), ('PML', 320),
    -- Zona 2 (4.770)
    ('OKI', 170), ('OKU', 100), ('OKT', 50), ('TJT', 170), ('MJB', 150), ('JMB', 192), ('KRC', 298),
    ('MRG', 150), ('LGS', 300), ('LGT', 293), ('LTM', 27), ('BKA', 300), ('BKB', 200),
    ('JBR', 300), ('LMJ', 150), ('PCT', 470), ('PNG', 170), ('BKL', 320), ('NGJ', 150), ('MJK', 170),
    ('TBN', 220), ('BJN', 100), ('MLG', 320)
) AS q(document_code, quota)
JOIN regencies r ON r.document_code = q.document_code
WHERE s.regency_id = r.id;

-- +goose Down
-- Clear the quotas set above, returning those schedules to unlimited (sequential) mode.
UPDATE program_schedules s
SET slot_quota = NULL
FROM regencies r
WHERE s.regency_id = r.id
  AND r.document_code IN (
      'LNG', 'BRN', 'SBG', 'DLS', 'TDT', 'SJJ', 'PDP', 'IRH', 'IRL', 'SIK', 'RHL', 'KPR',
      'CLP', 'BMS', 'GRB', 'BLR', 'WNG', 'BBS', 'TGL', 'JPR', 'KBM', 'PBG', 'PML',
      'OKI', 'OKU', 'OKT', 'TJT', 'MJB', 'JMB', 'KRC', 'MRG', 'LGS', 'LGT', 'LTM', 'BKA', 'BKB',
      'JBR', 'LMJ', 'PCT', 'PNG', 'BKL', 'NGJ', 'MJK', 'TBN', 'BJN', 'MLG'
  );
