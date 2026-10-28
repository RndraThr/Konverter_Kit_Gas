# Rancangan Map Distribusi v2 (Titik, Milestone, Zona)

Versi: 0.1 (draf untuk review)
Tanggal: 2026-10-28
Status: Terimplementasi
Terkait: `2026-10-09-distribution-map-design.md` (Lapis A), `2026-10-23-web-watermark-camera-design.md`
Mockup: artifact "Map Distribusi v2" (desktop interaktif + artboard HP); demo peta asli MapLibre di `scratchpad/mapdemo`.

## 1. Tujuan

Rancangan 2026-10-09 menunda **Lapis B (titik ber-geotag)** dan **Lapis C (kronologi)** karena belum ada data koordinat. Sekarang kamera watermark (web dan aplikasi) sudah mengisi `media_files.latitude/longitude`, jadi kedua lapis bisa dibangun. v2 mengganti peta SVG statis dengan peta interaktif bergaya Google Maps:

1. **Titik = lokasi serah terima satu nomor bagi**, warnanya menunjukkan milestone yang sudah dicapai.
2. **Peta dasar asli**: Biasa (jalan, nama tempat, POI), Satelit, dan Hybrid.
3. **Zona** bisa disorot sebagai mode pewarnaan dan dipakai untuk navigasi.
4. **Zoom otomatis ke batas wilayah**: semua titik, zona, kabupaten, atau satu titik.
5. **Validasi lokasi**: titik yang koordinatnya di luar kabupaten penerima ditandai "perlu ditinjau".
6. Bisa dipakai di HP (petugas lapangan), termasuk banyak penerima di satu titik serah.

Lapis A (agregat per kabupaten, `GET /api/v1/recipients/map`) tetap dipakai sebagai lapis warna wilayah dan tabel progres.

## 2. Keputusan Utama

| Topik | Keputusan | Alasan |
|---|---|---|
| Mesin peta | **MapLibre GL JS** (open source, WebGL) | Vector tiles, label bertahap sesuai zoom, ribuan titik tetap lancar, clustering bawaan, tanpa lisensi Mapbox. |
| Peta dasar Biasa | **OpenFreeMap** (`tiles.openfreemap.org/styles/liberty`) untuk awal; dapat diganti **MapTiler** atau **Protomaps** yang di-host sendiri | Gratis dan tanpa API key; sumber dikonfigurasi lewat env, jadi bisa diganti tanpa mengubah kode. |
| Satelit & Hybrid | **Esri World Imagery** + overlay Esri `World_Transportation` dan `World_Boundaries_and_Places` | Sudah terbukti di demo. Syarat pemakaian Esri harus dicek sebelum produksi; alternatifnya MapTiler Satellite (perlu key). |
| Basis data spasial | **PostGIS** di PostgreSQL 17 | Point-in-polygon untuk validasi lokasi, `ST_Union` untuk batas zona, indeks GiST untuk query per area tampilan, pengelompokan titik serah. |
| Peta ringan | Peta SVG lama tetap ada sebagai **mode ringan** | Dipakai kalau tile gagal dimuat (sinyal lemah); tidak butuh jaringan setelah bundle dimuat. Ini menjaga alasan rancangan 10-09. |

## 3. Definisi Milestone

Satu milestone per nomor bagi (`distribution_slots`). Slot `cancelled` tidak ditampilkan. Milestone diambil dari tahap tertinggi yang sudah tercapai:

| Kode | Label | Syarat di basis data | Waktu milestone |
|---|---|---|---|
| M1 | Terjadwal | slot ada, `status = 'open'` | `created_at` |
| M2 | Penerima tertaut | `status = 'linked'` | waktu penautan (lihat catatan) |
| M3 | Diserahkan | `status = 'completed'` | `completed_at` (atau `distributed_at`) |
| M4 | Bukti lengkap | M3 dan semua `documentation_slots` wajib `complete` (memakai `evidenceStatusSQL` yang sama dengan Data Penerima) | `max(media_files.uploaded_at)` dari slot wajib |
| M5 | BA ditandatangani | M4 dan ada `bast_individual_documents` `status = 'final'` untuk slot itu | `created_at` dokumen final |

Catatan:
- Milestone diurutkan dan saling mensyaratkan, jadi satu titik selalu berada di satu tahap.
- Waktu penautan (M2) belum disimpan secara eksplisit. Usulan: tambah `distribution_slots.linked_at` (diisi saat status jadi `linked`, backfill dari `updated_at`). Tanpa kolom ini, pemutaran timeline untuk M2 tidak akurat.
- **Perlu dikonfirmasi:** apakah M5 cukup berdasarkan BA perorangan final, atau juga harus ada tanda tangan penerima (`distribution_slot_signatures`)?

## 4. Lokasi Titik

### 4.1 Sumber koordinat

Koordinat sebuah nomor bagi diambil dari `media_files` yang `status = 'accepted'` dan punya `latitude`/`longitude`, milik `documentation_slots` slot itu. Urutan pilihan:

1. foto di tahap `penyerahan`, lalu tahap lain;
2. di dalam tahap, `sort_order` slot dokumentasi terkecil;
3. lalu `captured_at` paling awal.

Aturan ini tidak bergantung pada `slot_code` tertentu, sehingga tetap berlaku untuk template petani maupun nelayan. Nomor bagi tanpa foto ber-GPS dihitung sebagai **"belum ada lokasi"** dan ditampilkan sebagai angka yang bisa diklik (daftar), bukan disembunyikan.

**Prasyarat data:** `require_location` saat ini `false` untuk semua slot template. Supaya titik terisi, `require_location` perlu dinyalakan minimal untuk slot tahap penyerahan (lihat §11).

### 4.2 Validasi lokasi

`location_status` per titik:

- `inside`: titik berada di dalam batas kabupaten jadwalnya (`ST_Covers` dengan toleransi buffer 500 m untuk menutupi penyederhanaan batas dan error GPS).
- `outside`: di luar, ditampilkan dengan lingkar oranye **"Perlu ditinjau"**.
- `unmatched`: kabupaten belum punya batas (nama tidak cocok); titik tetap tampil tanpa penanda.

Validasi ini hanya penanda visual. Tidak memblokir alur distribusi.

### 4.3 Titik serah bertumpuk

Banyak penerima sering diserahkan di satu tempat (balai desa). `main` sudah mencatat titik serah per nomor bagi (`distribution_slots.handover_location_id` → `bast_handover_locations`, satu daftar per kabupaten, migrasi 00078–00079). Pengelompokan memakai data itu lebih dulu:

1. Slot dengan `handover_location_id` → satu **titik serah** per lokasi, bernama sesuai `bast_handover_locations.name`; posisi penanda = median koordinat titik anggotanya (lokasi titik serah belum punya koordinat sendiri, jadi koordinatnya diturunkan dari foto lapangan).
2. Slot tanpa `handover_location_id` → dikelompokkan jika jaraknya ≤ 30 m dalam jadwal yang sama (`ST_ClusterDBSCAN`, eps 30 m, minpoints 1), tanpa nama.

Anggota titik serah yang koordinatnya lebih dari 1 km dari median ditandai "perlu ditinjau" (kemungkinan salah pilih titik serah atau GPS meleset). Di peta, titik serah tampil sebagai satu penanda berangka; kliknya membuka daftar penerima di lokasi itu (desktop: panel; HP: bottom sheet). Warna penanda mengikuti milestone **terendah** di titik serah itu, supaya pekerjaan yang belum selesai tetap terlihat.

## 5. Zona

Zona sudah ada: `program_zones` dan `program_regency_assignments` (satu kabupaten tepat satu zona per program). Batas zona = gabungan batas kabupatennya, jadi **tidak perlu data geo baru**.

- **Mode warna "Zona"**: setiap zona diberi warna kategori yang berbeda jelas (palet 8 warna, beda terang-gelap juga, bukan hanya hue). Garis luar zona tebal; label zona di tengah area.
- **Di mode progres**: batas zona tetap tampil sebagai garis putus-putus tipis plus label kecil.
- Zona `is_placeholder = true` tampil abu-abu bergaris dengan label **"Belum dizonasi"**.
- Navigasi: breadcrumb `Indonesia › Zona › Kabupaten`; filter `zone_id` (sudah ada di kontrak filter) otomatis zoom ke batas zona; saat zoom jauh, cluster dikelompokkan per zona; tabel progres dikelompokkan per zona dengan subtotal.
- Pewarnaan zona hanya bermakna jika satu program dipilih. Tanpa filter program, mode Zona memakai program aktif pertama dan menampilkan pilihan program.

## 6. Data Spasial dan PostGIS

### 6.1 Infrastruktur

- Image basis data: `postgres:17-alpine` → **`postgis/postgis:17-3.5-alpine`** di `docker-compose.yml` dan service CI (`.github/workflows/ci.yml`). Versi mayor PostgreSQL sama, jadi volume data yang ada tetap terbaca.
- Deploy produksi: backup (`pg_dump`) → ganti image → jalankan migrasi yang memanggil `CREATE EXTENSION IF NOT EXISTS postgis`. Langkah lengkap dan rollback: §6.1.1.

### 6.1.1 Runbook deploy dan rollback (migrasi 00080)

Deploy (staging/produksi, `docker compose`):

1. **Backup dulu**: `docker compose exec postgres pg_dump -U "$POSTGRES_USER" -Fc "$POSTGRES_DB" > konkit-pre-00080.dump`, lalu salin file keluar dari server.
2. **Pastikan aplikasi terhubung sebagai superuser image**: `DATABASE_URL` di `.env.staging` harus memakai `POSTGRES_USER` dari `.env.compose` (image Postgres membuat user itu sebagai superuser). `CREATE EXTENSION postgis` di 00080 butuh hak superuser; dengan user lain migrasi gagal dan server tidak start. Cek: `docker compose exec postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT rolsuper FROM pg_roles WHERE rolname = current_user"` → `t`.
3. **Ganti image dan naikkan**: `docker-compose.yml` sudah memakai `postgis/postgis:17-3.5-alpine` (mayor 17 sama, volume lama terbaca). Jalankan `docker compose pull postgres && docker compose up -d --build`. Entrypoint app menjalankan `./migrate up` sebelum `./server`, jadi selama migrasi aplikasi belum melayani.
4. **Perkirakan jeda**: `ALTER TABLE media_files ADD COLUMN geom … GENERATED … STORED` menulis ulang seluruh tabel di bawah `ACCESS EXCLUSIVE`; durasinya sebanding dengan jumlah baris `media_files`. Aplikasi memang mati selama itu, jadi tidak ada request yang tertahan, tetapi jadwalkan di luar jam kerja lapangan.
5. **Verifikasi**: log app menampilkan `OK 00080_postgis_distribution_map.sql`, impor batas kabupaten selesai, dan `/map` memuat titik.

Rollback (pilih salah satu):

- **Pulihkan backup (disarankan)**: hentikan stack, buat volume baru (atau hapus `postgres-data`), kembalikan image `postgres:17-alpine`, `docker compose up -d postgres`, lalu `pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists konkit-pre-00080.dump`, kemudian naikkan app versi lama.
- **Turunkan migrasi di tempat**: hentikan app (`docker compose stop app`), lalu dengan image PostGIS masih berjalan jalankan `./migrate down` sampai versi 79 (`docker compose run --rm --entrypoint ./migrate app down` sekali), lalu `DROP EXTENSION postgis;`, baru ganti image kembali ke `postgres:17-alpine`. Urutan ini wajib: selama kolom `media_files.geom` (generated, memanggil fungsi PostGIS di setiap INSERT/UPDATE) atau ekstensinya masih ada, Postgres tanpa PostGIS tidak bisa memuat fungsi tersebut dan setiap upload foto gagal.

### 6.2 Skema (migrasi baru)

```sql
CREATE EXTENSION IF NOT EXISTS postgis;

CREATE TABLE regency_boundaries (
    boundary_name text PRIMARY KEY,          -- nama dari geoBoundaries, mis. "Bangka Barat"
    normalized_key text NOT NULL UNIQUE,     -- kunci normalRegencyName yang sama dengan frontend
    regency_id uuid REFERENCES regencies(id) ON DELETE SET NULL,
    geom geometry(MultiPolygon, 4326) NOT NULL,
    source text NOT NULL,
    license text NOT NULL
);
CREATE INDEX regency_boundaries_geom_idx ON regency_boundaries USING gist (geom);
CREATE INDEX regency_boundaries_regency_idx ON regency_boundaries (regency_id);

ALTER TABLE media_files ADD COLUMN geom geometry(Point, 4326)
    GENERATED ALWAYS AS (
        CASE WHEN latitude IS NOT NULL AND longitude IS NOT NULL
             THEN ST_SetSRID(ST_MakePoint(longitude::float8, latitude::float8), 4326) END
    ) STORED;
CREATE INDEX media_files_geom_idx ON media_files USING gist (geom) WHERE geom IS NOT NULL;
```

- Kolom `latitude/longitude` tetap jadi sumber; `geom` terisi otomatis, sehingga kode upload tidak berubah.
- **Impor batas**: aset `indonesia-regencies.json` disalin ke paket Go (`internal/geo/boundaries`, `embed`). Saat start, `geo.EnsureRegencyBoundaries` mengisi tabel jika kosong atau jika checksum aset berubah, lalu mencocokkan `regency_id` dengan normalisasi nama yang sama dengan `mapRegions.normalizeRegencyName` (port ke Go, diuji dengan kasus KAB./KOTA dan pasangan kota/kabupaten bernama sama).
- Kabupaten master yang tidak menemukan batas dicatat di log startup (peringatan di Pusat Administrasi diusulkan untuk iterasi berikutnya).

## 7. API

Semua endpoint memakai otorisasi yang sama dengan `/recipients/map` (`recipients.view` atau `distribution.view`), `RegencyScope` yang sama, dan kontrak filter Data Penerima (`regency_id`, `program_id`, `schedule_id`, `zone_id`, `district`, `allocation_status`, `evidence_status`, …).

### 7.1 `GET /api/v1/map/points`

Parameter tambahan: `bbox=minLon,minLat,maxLon,maxLat` (opsional), `stage=M1..M5` (opsional). Respons GeoJSON:

```json
{
  "type": "FeatureCollection",
  "features": [{
    "type": "Feature",
    "geometry": { "type": "Point", "coordinates": [108.19533, -6.45012] },
    "properties": {
      "slot_id": "…", "code": "NB-012", "schedule_id": "…",
      "regency_id": "…", "zone_id": "…", "site_id": "s-3",
      "stage": 5, "stage_at": ["2026-11-02T…", "…"],
      "location_status": "inside"
    }
  }],
  "meta": { "total": 120, "located": 108, "unlocated": 12, "outside": 3 }
}
```

- Nama dan NIK penerima **tidak** dikirim dalam respons titik. Rincian diambil saat titik diklik lewat endpoint slot yang sudah ada, sehingga data pribadi tidak terkumpul dalam satu respons besar.
- `stage_at` berisi waktu tiap milestone yang sudah dicapai, untuk pemutaran timeline di klien.
- `site_id` mengelompokkan titik serah bertumpuk (§4.3).

### 7.2 `GET /api/v1/map/zones?program_id=`

GeoJSON batas zona: `ST_SimplifyPreserveTopology(ST_Union(geom), 0.005)` per zona, dengan `zone_id`, `code`, `name`, `is_placeholder`, `regency_count`. Hasil di-cache di memori per program dan dibuang saat `program_regency_assignments` berubah.

### 7.3 `GET /api/v1/map/regencies/{regency_id}/boundary`

Batas satu kabupaten (GeoJSON) untuk sorotan dan zoom otomatis saat dicari. Batas semua kabupaten untuk lapis warna tetap memakai aset frontend yang sudah ada (cache browser, tanpa beban server).

### 7.4 Perubahan `GET /api/v1/recipients/map`

Ditambah `zone_id`, `zone_code`, `zone_name` per region (jika `program_id` dipilih), supaya tabel progres bisa dikelompokkan per zona.

## 8. Frontend

### 8.1 Struktur

- Dependensi baru: `maplibre-gl` (dimuat sebagai chunk terpisah hanya di halaman peta).
- `features/dashboard/map/`:
  - `MapCanvas.tsx`: inisialisasi MapLibre, `ResizeObserver` → `map.resize()` (terbukti perlu di demo), kontrol zoom/kompas/skala.
  - `basemaps.ts`: definisi Biasa / Satelit / Hybrid dari konfigurasi; pergantian mode hanya mengubah `visibility` layer, tidak memuat ulang style (lapis titik dan zona tidak hilang).
  - `layers.ts`: lapis warna kabupaten (metrik atau zona), garis dan label zona, cluster, titik milestone, penanda titik serah, sorotan batas terpilih.
  - `useMapPoints.ts`, `useZoneBoundaries.ts`: query TanStack dengan kunci yang menyertakan filter.
  - `useMapUrlState.ts`: posisi, zoom, basemap, mode warna, filter, titik terpilih disimpan di query URL agar tautan bisa dibagikan.
  - `PointDetailPanel.tsx` (desktop) / `PointBottomSheet.tsx` (HP < 768 px): milestone, foto bukti, koordinat, mesin, tombol "Buka nomor bagi" dan "Arahkan ke lokasi" (tautan `geo:`/Google Maps).
  - `MilestoneFilter.tsx`, `TimelinePlayer.tsx`, `ZoneBreadcrumb.tsx`, `MapSearch.tsx`.
- Peta SVG lama (`DistributionMap.tsx`) dipertahankan sebagai **mode ringan**: dipakai otomatis jika style peta gagal dimuat dalam 8 detik, dan bisa dipilih manual.

### 8.2 Perilaku

- **Zoom otomatis**: saat dibuka, `fitBounds` ke semua titik dalam cakupan user (petugas langsung melihat kabupatennya); pilih zona/kabupaten → `fitBounds` ke batasnya; pilih titik → `flyTo` zoom 15.
- **Cluster**: cluster bawaan MapLibre untuk titik; saat zoom < 7 dan mode Zona aktif, ringkasan per zona.
- **Pencarian**: kabupaten/kota seluruh Indonesia (dari aset batas), nomor bagi, dan desa.
- **Timeline**: slider tanggal + tombol putar memakai `stage_at`; tanpa request tambahan.
- **Pembaruan data**: `refetchInterval` 60 detik saat halaman terlihat dan refetch saat fokus kembali. Realtime web lewat hub yang ada diusulkan untuk iterasi berikutnya.
- **Aksesibilitas**: tabel progres dan daftar titik serah jadi jalur non-peta untuk data yang sama; semua kontrol punya label; warna milestone dibedakan juga dari terang-gelapnya.

### 8.3 Konfigurasi

Env backend yang diteruskan ke frontend lewat endpoint baru `GET /api/v1/map/config` (permission `recipients.view`; saat ini belum ada endpoint config umum), sehingga penyedia tile bisa diganti tanpa build ulang bundle:

- `MAP_STYLE_URL` (default OpenFreeMap liberty)
- `MAP_SATELLITE_TILES`, `MAP_HYBRID_OVERLAY_TILES`, `MAP_ATTRIBUTION`
- `MAP_TILE_API_KEY` (opsional, untuk MapTiler)

Atribusi geoBoundaries (CC BY 3.0 IGO), OpenStreetMap, dan Esri wajib tampil.

## 9. Privasi dan Keamanan

- Koordinat titik praktis adalah lokasi rumah/serah penerima: hanya untuk user dengan `recipients.view` dan dalam `RegencyScope`.
- Respons titik tanpa nama/NIK (§7.1). URL yang dibagikan berisi posisi peta dan id slot, bukan data pribadi; penerima tautan tetap harus login dan punya cakupan.
- Request tile ke penyedia pihak ketiga hanya membawa posisi peta, bukan data penerima.

## 10. Di Luar Lingkup v2 (usulan v3)

Heatmap, batas desa/kecamatan, rute petugas, mode offline (tile tersimpan di perangkat), ekspor peta PNG/PDF, realtime web.

## 11. Pertanyaan Terbuka

1. **Penyedia tile produksi**: OpenFreeMap + Esri (gratis, cek syarat Esri) atau MapTiler (key, berbayar jika besar)?
2. **`require_location`**: dinyalakan untuk slot tahap penyerahan saja, atau semua foto? Untuk data lama yang tanpa GPS, cukup dihitung "belum ada lokasi"?
3. **M5**: BA perorangan final saja, atau juga tanda tangan penerima?
4. **Radius titik serah**: 30 m cukup?
5. **Warna zona**: otomatis dari palet, atau bisa diatur admin per zona?

## 12. Pengujian

- Integrasi (DB PostGIS): impor batas dan pencocokan `regency_id`; `location_status` inside/outside/unmatched; pemilihan foto sumber koordinat; pengelompokan titik serah; `ST_Union` zona; cakupan kabupaten pada `/map/points`; milestone M1–M5 termasuk slot dibatalkan.
- Unit Go: normalisasi nama (port dari frontend, kasus sama).
- Frontend (Vitest): pemetaan stage → warna, state URL, filter milestone, timeline, pergantian basemap tanpa kehilangan lapis (MapLibre di-mock).
- E2E (Playwright): buka peta, zoom ke zona, klik titik serah, buka bottom sheet di viewport HP.

## Keputusan implementasi

Keputusan yang diambil saat membangun dan berlaku sebagai bagian dari rancangan ini:

- Nomor bagi ditampilkan sebagai nomor slot (`distribution_slots.slot_number`), bukan nomor lain.
- Zona placeholder digambar abu polos tanpa arsir.
- Tombol "Buka nomor bagi" tanpa deep link: membuka halaman Pendistribusian, bukan slot tertentu.
- Mode ringan hanya mewarnai kabupaten (peta SVG lama); titik dan batas zona tidak digambar di sana.
- Cache zona disimpan per proses (per program dan cakupan), dibatalkan saat penetapan zona berubah.
- `/map/unlocated` mengembalikan `{items,total}` dengan `items` dibatasi 500.
- Id situs `geo:` tidak pernah dimasukkan ke URL; hanya `hl:` (titik serah) yang boleh.
- `api_key` ditambahkan sebagai parameter query `key=` pada URL tile dan style.
