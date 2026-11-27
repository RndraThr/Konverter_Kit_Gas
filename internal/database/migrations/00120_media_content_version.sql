-- +goose Up
-- content_version naik setiap isi file diganti di tempat (mis. diputar dari
-- monitoring). URL konten menyertakan ?v= sehingga cache browser dan aplikasi
-- mengambil isi baru; file yang belum pernah diubah tetap memakai URL lama.
ALTER TABLE media_files ADD COLUMN content_version integer NOT NULL DEFAULT 0;
ALTER TABLE activity_media ADD COLUMN content_version integer NOT NULL DEFAULT 0;
ALTER TABLE bast_rakorda_uploads ADD COLUMN content_version integer NOT NULL DEFAULT 0;
ALTER TABLE bast_full_scans ADD COLUMN content_version integer NOT NULL DEFAULT 0;
ALTER TABLE bast_wet_signature_scans ADD COLUMN content_version integer NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE bast_wet_signature_scans DROP COLUMN content_version;
ALTER TABLE bast_full_scans DROP COLUMN content_version;
ALTER TABLE bast_rakorda_uploads DROP COLUMN content_version;
ALTER TABLE activity_media DROP COLUMN content_version;
ALTER TABLE media_files DROP COLUMN content_version;
