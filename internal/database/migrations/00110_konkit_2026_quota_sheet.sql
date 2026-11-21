-- +goose Up
-- Target distribusi KONKIT-2026 disamakan dengan lembar "Jadwal
-- Pendistribusian dan Pemasangan Konkit 2026": Bireuen 234 (+74),
-- Langsa 260 (+100), Padang Panjang 6 (-174). Kabupaten lain sudah sama.
UPDATE program_schedules ps SET slot_quota = v.quota, updated_at = now()
FROM (VALUES ('KAB. BIREUEN', 234), ('KOTA LANGSA', 260), ('KOTA PADANG PANJANG', 6)) AS v(regency_name, quota),
    regencies r, programs p
WHERE r.id = ps.regency_id AND upper(r.name) = v.regency_name AND p.id = ps.program_id AND p.code = 'KONKIT-2026'
    AND ps.status <> 'cancelled';

-- +goose Down
UPDATE program_schedules ps SET slot_quota = v.quota, updated_at = now()
FROM (VALUES ('KAB. BIREUEN', 160), ('KOTA LANGSA', 160), ('KOTA PADANG PANJANG', 180)) AS v(regency_name, quota),
    regencies r, programs p
WHERE r.id = ps.regency_id AND upper(r.name) = v.regency_name AND p.id = ps.program_id AND p.code = 'KONKIT-2026'
    AND ps.status <> 'cancelled';
