-- +goose Up
-- Item text in Package Templates used to be forced to uppercase on input and
-- on print. Templates now keep the text as typed, so the existing values are
-- converted once to Indonesian spelling (EYD): brands with an initial capital,
-- SI unit symbols in lowercase (kg, m), a decimal comma for power, and counting
-- units as ordinary words. Copies already taken from templates (allocation
-- snapshots and distribution slot verification snapshots) get the same values so
-- Berita Acara printed from them match. Finalized Berita Acara snapshots are
-- history and are left untouched.
--
-- Only exact JSON string values are replaced (`": "OLD"`), so codes such as
-- "shark-spwp8030" and serial numbers are never affected.
-- +goose StatementBegin
DO $$
DECLARE
    pairs text[][] := ARRAY[
        ['SHARK', 'Shark'],
        ['ERGAS', 'Ergas'],
        ['TRILIUNHOSE/YAMAKOYO', 'Triliunhose/Yamakoyo'],
        ['TRILLIUNHOSE\nYAMAKOYO', 'Triliunhose\nYamakoyo'],
        ['TRILIUNHOSE', 'Triliunhose'],
        ['TRILLIUNHOSE', 'Triliunhose'],
        ['YAMAKOYO', 'Yamakoyo'],
        ['5.5 HP', '5,5 HP'],
        ['6 M\n10 M', '6 m\n10 m'],
        ['6 M', '6 m'],
        ['10 M', '10 m'],
        ['6M/10M', '6 m/10 m'],
        ['Panjang Selang Hisap: 6m, Panjang Selang Buang: 10m', 'Panjang Selang Hisap: 6 m, Panjang Selang Buang: 10 m'],
        ['OLI', 'Oli'],
        ['Bracket', 'Braket'],
        ['Tabung LPG 3 Kg', 'Tabung LPG 3 kg'],
        ['Isi LPG 3 Kg', 'Isi LPG 3 kg'],
        ['Pcs', 'Buah'],
        ['Ea', 'Buah'],
        ['Ltr', 'Liter'],
        -- Bentuk kapital semua dari template yang pernah disimpan saat input
        -- masih dipaksa kapital.
        ['TABUNG LPG 3 KG', 'Tabung LPG 3 kg'],
        ['ISI LPG 3 KG', 'Isi LPG 3 kg'],
        ['REGULATOR', 'Regulator'],
        ['SELANG HISAP', 'Selang Hisap'],
        ['SELANG BUANG', 'Selang Buang'],
        ['SELANG, CLAMP & AKSESORISNYA', 'Selang, Clamp & Aksesorisnya'],
        ['BUKU MANUAL', 'Buku Manual'],
        ['MIXER NIPPLE', 'Mixer Nipple'],
        ['KARTU GARANSI MESIN', 'Kartu Garansi Mesin'],
        ['KARTU GARANSI KONKIT/REDUCER', 'Kartu Garansi Konkit/Reducer'],
        ['BRAKET', 'Braket'],
        ['BRACKET', 'Braket'],
        ['PLAT ANTI KARAT', 'Plat Anti Karat'],
        ['TABUNG', 'Tabung'],
        ['SET', 'Set'],
        ['BOTOL', 'Botol'],
        ['PCS', 'Buah'],
        ['EA', 'Buah'],
        ['LTR', 'Liter'],
        ['BENSIN', 'Bensin'],
        ['SOLAR', 'Solar'],
        ['PERTAMINA', 'Pertamina'],
        ['PERTAMINA ENDURO', 'Pertamina Enduro'],
        ['TOP GAS', 'Top Gas'],
        ['MONDEA', 'Mondea']
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
-- Irreversible on purpose: the original uppercase text is not needed again.
SELECT 1;
