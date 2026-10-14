-- +goose Up
-- The "dokumen_pendukung" activity type was added at the Go application
-- level (internal/activities/models.go's activityTypeCodes map) but the
-- database-level CHECK constraint on activity_media.activity_type was
-- never updated to match, so every insert of a dokumen_pendukung row
-- fails with a check constraint violation. Postgres has no
-- "ALTER CONSTRAINT ... ADD VALUE" for a plain CHECK constraint, so the
-- constraint must be dropped and recreated with the additional value.
ALTER TABLE activity_media DROP CONSTRAINT activity_media_activity_type_check;
ALTER TABLE activity_media ADD CONSTRAINT activity_media_activity_type_check CHECK (activity_type IN (
    'ceremony_sosialisasi', 'pelatihan_teknis', 'rakor',
    'training_10', 'training_100',
    'unloading_konkit', 'unloading_mesin_pompa', 'unloading_oli',
    'unloading_selang', 'unloading_tabung_gas',
    'dokumen_pendukung'
));

-- +goose Down
ALTER TABLE activity_media DROP CONSTRAINT activity_media_activity_type_check;
ALTER TABLE activity_media ADD CONSTRAINT activity_media_activity_type_check CHECK (activity_type IN (
    'ceremony_sosialisasi', 'pelatihan_teknis', 'rakor',
    'training_10', 'training_100',
    'unloading_konkit', 'unloading_mesin_pompa', 'unloading_oli',
    'unloading_selang', 'unloading_tabung_gas'
));
