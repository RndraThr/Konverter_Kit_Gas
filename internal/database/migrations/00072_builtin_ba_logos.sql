-- +goose Up
-- Every program gets the four standard tender logos (Pertamina, logo kuning, Pertamina Patra Niaga,
-- PT Kian Santang Muliatama Tbk) without an upload. The files ship with the app
-- (internal/bast/builtin_logos); a "builtin:" storage key tells the logo loader to read them from
-- there instead of media storage. They can still be replaced or hidden per program in Persiapan.
-- +goose StatementBegin
CREATE FUNCTION seed_builtin_ba_logos(target_program_id uuid) RETURNS void
LANGUAGE sql AS $$
    INSERT INTO program_ba_logo_assets (program_id, slot_code, storage_key, original_filename, mime_type, byte_size, checksum, sort_order, max_width_mm, max_height_mm, is_visible)
    SELECT target_program_id, logo.slot_code, 'builtin:' || target_program_id::text || '/' || logo.filename, logo.filename, 'image/png', logo.byte_size, logo.checksum, logo.sort_order, 35, 18, true
    FROM (VALUES
        ('logo_1', '04-pertamina-transparan.png', 3365, 'aeab273d44a22adbcd6e848b199bc981dc0bf586c40ea2084013e553ff3438fa', 0),
        ('logo_2', '01-logo-kuning-transparan.png', 39674, 'c0aa655da1c3accea99610ba7272b27c5dd4bfd52c184ed160657e23737bb3c2', 1),
        ('logo_3', '02-pertamina-patra-niaga-transparan.png', 415839, 'f0bb96ee4b43ce86ef7f68d3498b3b6a68c36e05903e0d6cf4a813b460b3c71b', 2),
        ('logo_4', '03-pt-kian-santang-muliatama-transparan.png', 627963, 'b62c2976ee63d07b04bd06e47fcd3780c17d2d62da99a6fa6223754b15e58d6d', 3)
    ) AS logo(slot_code, filename, byte_size, checksum, sort_order)
    ON CONFLICT (program_id, slot_code) DO NOTHING;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION seed_builtin_ba_logos_on_program_insert() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    PERFORM seed_builtin_ba_logos(NEW.id);
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER programs_seed_builtin_ba_logos
    AFTER INSERT ON programs
    FOR EACH ROW EXECUTE FUNCTION seed_builtin_ba_logos_on_program_insert();

-- Programs that already exist but have no logo yet get the standard set too.
SELECT seed_builtin_ba_logos(p.id)
FROM programs p
WHERE NOT EXISTS (SELECT 1 FROM program_ba_logo_assets l WHERE l.program_id = p.id);

-- +goose Down
DROP TRIGGER IF EXISTS programs_seed_builtin_ba_logos ON programs;
DROP FUNCTION IF EXISTS seed_builtin_ba_logos_on_program_insert();
DROP FUNCTION IF EXISTS seed_builtin_ba_logos(uuid);
DELETE FROM program_ba_logo_assets WHERE storage_key LIKE 'builtin:%';
