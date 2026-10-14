-- +goose Up
-- Same bug class as 00066 and 00067: a dokumen_pendukung upload is
-- detected with mime_type = 'application/pdf' (internal/media/upload.go),
-- but activity_media.mime_type's CHECK constraint never allowed it, so
-- every dokumen_pendukung insert still failed after 00066 and 00067 alone.
-- Drop and recreate the constraint with 'application/pdf' added.
ALTER TABLE activity_media DROP CONSTRAINT activity_media_mime_type_check;
ALTER TABLE activity_media ADD CONSTRAINT activity_media_mime_type_check CHECK (mime_type IN (
    'image/jpeg', 'image/png', 'image/webp',
    'video/mp4', 'video/webm', 'video/quicktime',
    'application/pdf'
));

-- +goose Down
ALTER TABLE activity_media DROP CONSTRAINT activity_media_mime_type_check;
ALTER TABLE activity_media ADD CONSTRAINT activity_media_mime_type_check CHECK (mime_type IN (
    'image/jpeg', 'image/png', 'image/webp',
    'video/mp4', 'video/webm', 'video/quicktime'
));
