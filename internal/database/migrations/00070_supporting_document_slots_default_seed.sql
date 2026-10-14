-- +goose Up
-- Default Dokumen Pendukung names, seeded once for every kabupaten already
-- in the system. The four unloading/handover document names are identical
-- across regencies; the last three carry the regency's own name.
INSERT INTO supporting_document_slots (regency_id, name, sort_order)
SELECT r.id, 'DO Konkit, Oli, Bracket', 1 FROM regencies r
UNION ALL SELECT r.id, 'DO Mesin', 2 FROM regencies r
UNION ALL SELECT r.id, 'DO Selang Buang', 3 FROM regencies r
UNION ALL SELECT r.id, 'DO Selang Hisap', 4 FROM regencies r
UNION ALL SELECT r.id, 'LO Gas ' || r.name, 5 FROM regencies r
UNION ALL SELECT r.id, 'SPP Tabung Gas ' || r.name, 6 FROM regencies r
UNION ALL SELECT r.id, 'Surat Kuasa Pengambilan Gas LPG 3Kg - ' || r.name, 7 FROM regencies r;

-- +goose Down
DELETE FROM supporting_document_slots
WHERE name IN ('DO Konkit, Oli, Bracket', 'DO Mesin', 'DO Selang Buang', 'DO Selang Hisap')
   OR name LIKE 'LO Gas %'
   OR name LIKE 'SPP Tabung Gas %'
   OR name LIKE 'Surat Kuasa Pengambilan Gas LPG 3Kg - %';
