-- +goose Up
-- Same bug class as 00066: the dokumen_pendukung upload path stores PDFs
-- with media_type = 'document' (internal/media/upload.go's KindDocument),
-- but activity_media.media_type's CHECK constraint only ever allowed
-- 'image' and 'video', so every dokumen_pendukung insert still failed
-- after 00066 alone. Drop and recreate the constraint with 'document' added.
ALTER TABLE activity_media DROP CONSTRAINT activity_media_media_type_check;
ALTER TABLE activity_media ADD CONSTRAINT activity_media_media_type_check CHECK (media_type IN ('image', 'video', 'document'));

-- +goose Down
ALTER TABLE activity_media DROP CONSTRAINT activity_media_media_type_check;
ALTER TABLE activity_media ADD CONSTRAINT activity_media_media_type_check CHECK (media_type IN ('image', 'video'));
