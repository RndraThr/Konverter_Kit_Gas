-- +goose Up
-- Foto Training dari distribusi (slot recipient_training) disalin otomatis ke
-- dokumentasi kegiatan Training 100%, dan untuk 10% peserta pertama tiap
-- tanggal juga ke Training 10%. Baris salinan menunjuk foto asalnya.
ALTER TABLE activity_media DROP CONSTRAINT activity_media_source_check;
ALTER TABLE activity_media ADD CONSTRAINT activity_media_source_check CHECK (source IN ('camera', 'gallery', 'distribution'));

ALTER TABLE activity_media
    ADD COLUMN distribution_media_id uuid REFERENCES media_files(id) ON DELETE SET NULL,
    -- true: dihapus oleh penyalin karena foto asal tidak lagi memenuhi syarat
    -- (bukan dihapus pengguna), sehingga boleh disalin ulang bila kembali.
    ADD COLUMN removed_by_sync boolean NOT NULL DEFAULT false;

CREATE UNIQUE INDEX activity_media_distribution_copy_idx
    ON activity_media (distribution_media_id, activity_type)
    WHERE distribution_media_id IS NOT NULL AND status = 'active';

-- +goose Down
DROP INDEX activity_media_distribution_copy_idx;
UPDATE activity_media SET status = 'deleted', updated_at = now() WHERE source = 'distribution' AND status = 'active';
ALTER TABLE activity_media DROP COLUMN removed_by_sync, DROP COLUMN distribution_media_id;
ALTER TABLE activity_media DROP CONSTRAINT activity_media_source_check;
UPDATE activity_media SET source = 'gallery' WHERE source = 'distribution';
ALTER TABLE activity_media ADD CONSTRAINT activity_media_source_check CHECK (source IN ('camera', 'gallery'));
