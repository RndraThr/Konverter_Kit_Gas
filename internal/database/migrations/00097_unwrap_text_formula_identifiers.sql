-- +goose Up
-- File DCP3 sering menulis nomor kartu sebagai formula teks Excel, misalnya
-- ="6032988517402660", agar angka panjang tidak berubah. Importer dulu
-- menyimpan teks formulanya apa adanya, sehingga nomor kartu tampil dengan
-- =" " di laporan dan BA. Importer kini mengambil isi teksnya; migrasi ini
-- membersihkan nomor yang sudah terlanjur tersimpan. normalized_value tidak
-- berubah karena sejak awal hanya berisi huruf dan angka.
UPDATE person_sector_identifiers
SET display_value = btrim(replace(substring(display_value FROM '^="(.*)"$'), '""', '"')),
    updated_at = now()
WHERE display_value ~ '^="([^"]|"")*"$';

-- +goose Down
-- Nilai asli berupa formula sengaja tidak dikembalikan.
SELECT 1;
