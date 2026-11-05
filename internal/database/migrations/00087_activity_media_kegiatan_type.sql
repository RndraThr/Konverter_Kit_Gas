-- +goose Up
-- Jenis dokumentasi kegiatan baru "Kegiatan" (folder Drive "KEGIATAN" di
-- bawah DOKUMENTASI (FOTO) tiap kabupaten), sesuai activityTypeCodes di
-- internal/activities/models.go.
ALTER TABLE activity_media DROP CONSTRAINT activity_media_activity_type_check;
ALTER TABLE activity_media ADD CONSTRAINT activity_media_activity_type_check CHECK (activity_type IN (
    'ceremony_sosialisasi', 'pelatihan_teknis', 'rakor',
    'training_10', 'training_100',
    'unloading_konkit', 'unloading_mesin_pompa', 'unloading_oli',
    'unloading_selang', 'unloading_tabung_gas',
    'dokumen_pendukung', 'kegiatan'
));

-- +goose Down
-- Gagal selama masih ada dokumentasi "kegiatan": pindahkan atau hapus dulu.
ALTER TABLE activity_media DROP CONSTRAINT activity_media_activity_type_check;
ALTER TABLE activity_media ADD CONSTRAINT activity_media_activity_type_check CHECK (activity_type IN (
    'ceremony_sosialisasi', 'pelatihan_teknis', 'rakor',
    'training_10', 'training_100',
    'unloading_konkit', 'unloading_mesin_pompa', 'unloading_oli',
    'unloading_selang', 'unloading_tabung_gas',
    'dokumen_pendukung'
));
