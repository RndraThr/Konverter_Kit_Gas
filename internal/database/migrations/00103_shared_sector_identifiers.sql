-- +goose Up
-- Penerima tanpa kartu tani memakai nomor surat keterangan dinas sebagai
-- nomor kartu, dan satu surat sering berlaku untuk banyak penerima. Nomor
-- kartu sektor kini boleh sama untuk beberapa orang; NIK yang membedakan
-- penerima. Satu orang tetap punya paling banyak satu nomor per jenis.
ALTER TABLE person_sector_identifiers
    DROP CONSTRAINT person_sector_identifiers_identifier_type_normalized_value_key;
CREATE INDEX person_sector_identifiers_value_idx
    ON person_sector_identifiers (identifier_type, normalized_value);

-- +goose Down
-- Nomor yang dipakai bersama hanya disimpan untuk pemilik pertamanya.
DROP INDEX person_sector_identifiers_value_idx;
DELETE FROM person_sector_identifiers later
USING person_sector_identifiers earlier
WHERE later.identifier_type = earlier.identifier_type
    AND later.normalized_value = earlier.normalized_value
    AND (later.created_at, later.id) > (earlier.created_at, earlier.id);
ALTER TABLE person_sector_identifiers
    ADD CONSTRAINT person_sector_identifiers_identifier_type_normalized_value_key UNIQUE (identifier_type, normalized_value);
