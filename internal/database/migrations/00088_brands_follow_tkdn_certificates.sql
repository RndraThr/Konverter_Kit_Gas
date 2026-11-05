-- +goose Up
-- Merek barang ditulis persis seperti di Sertifikat TKDN masing-masing
-- (bukan tabel Rekapitulasi): SHARK, ERGAS, FIGO DELUXE (selang hisap dan
-- buang), Trilliunhose. Template paket, salinan paket di alokasi, dan data
-- peralatan yang dibekukan saat nomor bagi diselesaikan disamakan, sehingga
-- BA yang dicetak berikutnya memakai tulisan sertifikat. BA yang sudah
-- difinalisasi adalah riwayat dan tidak diubah.
--
-- Hanya nilai string JSON yang persis sama (`": "LAMA"`) yang diganti, jadi
-- kode opsi seperti "figo" dan nomor seri tidak tersentuh.
-- +goose StatementBegin
DO $$
DECLARE
    pairs text[][] := ARRAY[
        ['Shark', 'SHARK'],
        ['Ergas', 'ERGAS'],
        ['Figo', 'FIGO DELUXE'],
        ['Figo\nFigo', 'FIGO DELUXE\nFIGO DELUXE'],
        ['Triliunhose', 'Trilliunhose'],
        ['Triliunhose\nYamakoyo', 'Trilliunhose\nYamakoyo']
    ];
    old_value text;
    new_value text;
BEGIN
    FOR i IN 1 .. array_length(pairs, 1) LOOP
        old_value := '": "' || pairs[i][1] || '"';
        new_value := '": "' || pairs[i][2] || '"';

        UPDATE package_template_versions
        SET values_json = replace(values_json::text, old_value, new_value)::jsonb
        WHERE strpos(values_json::text, old_value) > 0;

        UPDATE package_allocations
        SET package_snapshot_json = replace(package_snapshot_json::text, old_value, new_value)::jsonb
        WHERE strpos(package_snapshot_json::text, old_value) > 0;

        UPDATE distribution_slots
        SET verification_snapshot_json = replace(verification_snapshot_json::text, old_value, new_value)::jsonb
        WHERE strpos(verification_snapshot_json::text, old_value) > 0;
    END LOOP;
END
$$;
-- +goose StatementEnd

-- +goose Down
-- Tidak dikembalikan: tulisan sertifikat adalah acuan yang benar.
SELECT 1;
