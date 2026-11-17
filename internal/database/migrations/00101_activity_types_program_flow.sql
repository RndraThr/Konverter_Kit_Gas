-- +goose Up
-- Jenis dokumentasi kegiatan mengikuti urutan Panduan Kegiatan
-- Pendistribusian: kunjungan kantor dinas, gudang titik serah, perakitan
-- mesin, pemeriksaan barang, kegiatan pendistribusian, dan closing kabupaten.
-- "kegiatan" tetap ada sebagai Kegiatan Lainnya.
ALTER TABLE activity_media DROP CONSTRAINT activity_media_activity_type_check;
ALTER TABLE activity_media ADD CONSTRAINT activity_media_activity_type_check CHECK (activity_type IN (
    'ceremony_sosialisasi', 'pelatihan_teknis', 'rakor',
    'training_10', 'training_100',
    'unloading_konkit', 'unloading_mesin_pompa', 'unloading_oli',
    'unloading_selang', 'unloading_tabung_gas',
    'dokumen_pendukung', 'kegiatan',
    'kunjungan_dinas', 'gudang_titik_serah', 'perakitan_mesin',
    'pemeriksaan_barang', 'kegiatan_distribusi', 'closing_kabupaten'
));

-- +goose Down
-- Gagal selama masih ada dokumentasi jenis baru: pindahkan atau hapus dulu.
ALTER TABLE activity_media DROP CONSTRAINT activity_media_activity_type_check;
ALTER TABLE activity_media ADD CONSTRAINT activity_media_activity_type_check CHECK (activity_type IN (
    'ceremony_sosialisasi', 'pelatihan_teknis', 'rakor',
    'training_10', 'training_100',
    'unloading_konkit', 'unloading_mesin_pompa', 'unloading_oli',
    'unloading_selang', 'unloading_tabung_gas',
    'dokumen_pendukung', 'kegiatan'
));
