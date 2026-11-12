-- +goose Up
-- Revised KONKIT-2026 distribution totals for four Zona 2 kabupaten. The zone total stays 4.770
-- unit: Tanjung Jabung Timur 170 -> 104, Muaro Jambi 150 -> 216, Lampung Tengah 293 -> 287,
-- Lampung Timur 27 -> 33.
UPDATE program_schedules s
SET slot_quota = q.quota, updated_at = now()
FROM (VALUES ('TJT', 104), ('MJB', 216), ('LGT', 287), ('LTM', 33)) AS q(document_code, quota)
JOIN regencies r ON r.document_code = q.document_code
JOIN programs p ON p.code = 'KONKIT-2026'
WHERE s.regency_id = r.id AND s.program_id = p.id;

-- +goose Down
UPDATE program_schedules s
SET slot_quota = q.quota, updated_at = now()
FROM (VALUES ('TJT', 170), ('MJB', 150), ('LGT', 293), ('LTM', 27)) AS q(document_code, quota)
JOIN regencies r ON r.document_code = q.document_code
JOIN programs p ON p.code = 'KONKIT-2026'
WHERE s.regency_id = r.id AND s.program_id = p.id;
