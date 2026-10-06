-- +goose Up
ALTER TABLE documentation_template_slots
    ADD COLUMN media_kind text NOT NULL DEFAULT 'image'
    CHECK (media_kind IN ('image', 'video', 'image_video'));

ALTER TABLE documentation_slots
    ADD COLUMN media_kind text NOT NULL DEFAULT 'image'
    CHECK (media_kind IN ('image', 'video', 'image_video'));

ALTER TABLE media_files DROP CONSTRAINT media_files_mime_type_check;
ALTER TABLE media_files ADD CONSTRAINT media_files_mime_type_check
    CHECK (mime_type IN ('image/jpeg', 'image/png', 'image/webp', 'video/mp4', 'video/webm', 'video/quicktime'));

ALTER TABLE media_files DROP CONSTRAINT media_files_byte_size_check;
ALTER TABLE media_files ADD CONSTRAINT media_files_byte_size_check
    CHECK (byte_size > 0 AND byte_size <= 524288000);

ALTER TABLE activity_media DROP CONSTRAINT activity_media_byte_size_check;
ALTER TABLE activity_media ADD CONSTRAINT activity_media_byte_size_check
    CHECK (byte_size > 0 AND byte_size <= 524288000);

-- +goose Down
ALTER TABLE activity_media DROP CONSTRAINT activity_media_byte_size_check;
ALTER TABLE activity_media ADD CONSTRAINT activity_media_byte_size_check
    CHECK (byte_size > 0 AND byte_size <= 104857600);

ALTER TABLE media_files DROP CONSTRAINT media_files_byte_size_check;
ALTER TABLE media_files ADD CONSTRAINT media_files_byte_size_check
    CHECK (byte_size > 0 AND byte_size <= 10485760);

ALTER TABLE media_files DROP CONSTRAINT media_files_mime_type_check;
ALTER TABLE media_files ADD CONSTRAINT media_files_mime_type_check
    CHECK (mime_type IN ('image/jpeg', 'image/png', 'image/webp'));

ALTER TABLE documentation_slots DROP COLUMN media_kind;
ALTER TABLE documentation_template_slots DROP COLUMN media_kind;
