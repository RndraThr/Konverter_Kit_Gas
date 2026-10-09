# Rancangan Peta Distribusi (Lapis Agregat Wilayah)

Versi: 1.0
Tanggal: 2026-10-09
Status: Lapis A terimplementasi
Terkait: `2026-09-12-data-penerima-master-design.md`, `2026-10-07-staged-distribution-media-design.md`

## 1. Temuan Awal

`/dashboard/map-distribusi` sebelumnya hanya placeholder. Pengukuran pada basis data menunjukkan sistem **belum memiliki data spasial sama sekali**:

| Yang diukur | Hasil |
|---|---|
| `media_files` dengan `latitude`/`longitude` | 0 dari 21 baris |
| Pemanggilan `navigator.geolocation` di frontend | tidak ada |
| `documentation_slots` yang `require_location` | 0 dari 142 |
| Template dokumentasi petani (`00039`) | `require_location = false` untuk semua slot |
| Geometri wilayah (polygon/PostGIS/tabel desa) | tidak ada |
| Pustaka peta di `package.json` | tidak ada |

Wilayah hanya tersimpan sebagai teks bebas (`people.district`, `people.village`). Master `regencies` juga tidak menyimpan geometri.

Kesimpulan: peta "titik penerima" tidak dapat dibangun hari ini karena bahannya tidak ada. Karena itu peta dibagi dua lapis yang kebutuhan datanya berbeda.

## 2. Dua Lapis

**Lapis A — agregat wilayah (diimplementasikan sekarang).** Memakai tabel yang sudah terisi: `package_allocations`, `distribution_slots`, `documentation_slots`, `media_files`, `program_schedules`. Hanya membutuhkan satu aset batas wilayah. Menjawab sebaran penerima, progres penyaluran, dan kelengkapan bukti per kabupaten.

**Lapis B — titik dokumentasi ber-geotag (belum).** Memerlukan tiga hal lebih dulu: client meminta posisi saat slot mensyaratkan lokasi, `require_location` dinyalakan per template dokumentasi, lalu titik di-plot. Lapis ini juga dapat menurunkan centroid desa dari data lapangan sendiri sehingga tidak membutuhkan geocoding eksternal.

**Lapis C — kronologi (belum).** Pengelompokan titik per tanggal distribusi untuk membaca jalur penyaluran dan jeda antar tahap.

## 3. Aset Batas Wilayah

- Sumber: geoBoundaries gbOpen `IDN` ADM2 (2020), lisensi **CC BY 3.0 IGO**, sumber lisensi `data.humdata.org/dataset/indonesia-administrative-boundary-polygons-lines-and-places-levels-0-4b`.
- Penyederhanaan: Douglas-Peucker toleransi 0,012° dan ambang luas ring 0,0004°², koordinat dibulatkan 3 desimal, menyisakan 518 kabupaten/kota dengan 25.646 titik.
- Hasil: `frontend/src/features/dashboard/data/indonesia-regencies.json`, 437 kB, dimuat sebagai chunk terpisah (448 kB / 140 kB gzip) hanya ketika halaman peta dibuka.
- Atribusi ditampilkan pada halaman peta. Lisensi ini mewajibkan atribusi, jadi baris tersebut tidak boleh dihapus.
- Pencocokan nama: master menyimpan `KAB. BANGKA BARAT`/`KOTA PADANG PANJANG`, aset menyimpan `Bangka Barat`. Keduanya dinormalkan dengan membuang awalan KAB./KOTA/KABUPATEN dan tanda baca. Seluruh 46 kabupaten pada master dev cocok, tanpa tabrakan kunci.

## 4. Backend

Endpoint baru: `GET /api/v1/recipients/map` dengan permission `recipients.view`, memakai **kontrak filter yang sama** dengan daftar Data Penerima (`regency_id`, `program_id`, `program_type`, `schedule_id`, `zone_id`, `district`, `allocation_status`, `distribution_status`, `evidence_status`, `search`) dan cakupan kabupaten yang sama.

Respons berisi `regions[]` dan `totals`, masing-masing memuat:

- **Penerima**: `recipients`, `candidate`, `ready`, `distributed`, `needs_review`, `replaced`.
- **Kelengkapan bukti**: `evidence_complete`, `evidence_partial`, `evidence_empty`, `evidence_not_configured` — memakai `evidenceStatusSQL` yang sama dengan daftar, sehingga peta dan daftar tidak mungkin berbeda angka untuk filter yang sama.
- **Progres slot**: `slot_quota`, `slots_open`, `slots_linked`, `slots_completed`, `slots_cancelled`.

Catatan implementasi penting: agregat penerima memakai `recipientFrom`/`recipientWhere`, sedangkan agregat slot memakai CTE terpisah dengan **pre-agregasi per jadwal melalui `LEFT JOIN LATERAL`**. Tanpa itu, `sum(slot_quota)` dihitung setelah join ke `distribution_slots` sehingga kuota terduplikasi sebanyak jumlah slot pada jadwal tersebut (bug yang tertangkap test integrasi). Kuota hanya menjumlahkan jadwal yang tidak berstatus `cancelled`.

Kabupaten yang memiliki jadwal dan kuota tetapi belum memiliki penerima tetap muncul pada peta, supaya kuota yang belum terisi terlihat.

## 5. Frontend

- `mapRegions.ts` — fungsi murni yang dapat diuji: normalisasi nama, indeks kabupaten, definisi tiga metrik, skala warna berurutan berlabuh pada warna primer aplikasi (`#2f7d4a`), pemetaan metrik ke warna, dan proyeksi geometri ke koordinat SVG (ekuidistan dengan skala seragam, koordinat dibulatkan 1 desimal).
- `DistributionMap.tsx` — peta SVG: 518 path, hover tooltip, klik untuk memilih, zoom roda tetikus dan tombol, geser untuk pan, tombol reset, dan baris atribusi lisensi. Setiap path membawa `data-regency` dan `aria-label` berisi nama serta nilai metrik aktif.
- `DistributionMapPage.tsx` — pemilih metrik, filter peta, legenda beserta total, dan tabel peringkat wilayah. Tabel peringkat menjadi jalur aksesibel untuk data yang sama.
- Rendering memakai SVG dari aset yang dibundel, **tanpa tile server**: tetap bekerja setelah bundle dimuat, tidak ada kuota atau atribusi pihak ketiga, dan tidak menambah ketergantungan jaringan bagi petugas lapangan.

## 6. Kriteria Penerimaan

1. Peta menampilkan bentuk kabupaten/kota Indonesia tanpa tile server.
2. Angka peta untuk satu filter sama dengan angka daftar Data Penerima pada filter yang sama.
3. Tiga metrik dapat dipilih dan memetakan warna sesuai skala; wilayah tanpa data dibedakan secara visual.
4. Memilih wilayah (dari peta maupun tabel) menampilkan rincian wilayah tersebut.
5. Kuota tidak terduplikasi ketika sebuah jadwal memiliki banyak slot.
6. Filter cakupan kabupaten tetap ditegakkan pada endpoint peta.
7. Atribusi lisensi batas wilayah tampil pada halaman.
8. Seluruh tes Go, tes frontend, typecheck, dan build lulus.

## 7. Di Luar Cakupan

- Penangkapan koordinat dokumentasi (Lapis B) dan kronologi tanggal (Lapis C).
- Peta tile (OpenStreetMap/Google) dan zoom sampai level jalan.
- Polygon kecamatan/desa; agregat saat ini berhenti di kabupaten/kota.
- Geocoding alamat ke koordinat.
- Dukungan offline/PWA untuk halaman peta.
