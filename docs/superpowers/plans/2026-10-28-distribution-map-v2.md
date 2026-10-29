# Map Distribusi v2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ganti peta SVG statis dengan peta MapLibre interaktif yang menampilkan titik serah per nomor bagi beserta milestone, zona, validasi lokasi PostGIS, dan tampilan HP.

**Architecture:** PostGIS menyimpan batas kabupaten (diimpor dari aset geoBoundaries yang sudah ada) dan kolom `geom` hasil generate dari `media_files`. Paket baru `internal/distmap` menghitung titik, milestone, status lokasi, dan titik serah; `internal/api/map_routes.go` mengeksposnya sebagai GeoJSON. Frontend `features/dashboard/map/` merender MapLibre dengan basemap yang dikonfigurasi dari server; peta SVG lama tetap menjadi mode ringan.

**Tech Stack:** Go + pgx, PostgreSQL 17 + PostGIS 3.5, React 19 + TanStack Query, `maplibre-gl` 4.7.x, Vitest, Playwright.

**Spec:** `docs/superpowers/specs/2026-10-28-distribution-map-v2-design.md`

## Global Constraints

- Image DB: `postgis/postgis:17-3.5-alpine` (docker-compose dan CI); PostgreSQL tetap mayor 17.
- Otorisasi semua endpoint peta = `/recipients/map`: `authorizeAny(..., "recipients.view", "distribution.view")` + `regencyScope`.
- Filter peta = kontrak `recipientFilterFromRequest` yang sudah ada; tidak ada filter baru selain `bbox` dan `stage`.
- Respons titik tidak boleh memuat nama atau NIK penerima.
- Milestone: 1 Terjadwal, 2 Penerima tertaut, 3 Diserahkan, 4 Bukti lengkap, 5 BA ditandatangani; slot `cancelled` tidak ditampilkan.
- Warna milestone: `#c9d1c9`, `#9fcdb0`, `#5fae7a`, `#2f7d4a`, `#143f24`; penanda "perlu ditinjau" `#d9711c`.
- Buffer validasi lokasi 500 m; radius titik serah 30 m; fallback mode ringan setelah 8 detik style gagal dimuat.
- Default basemap: style `https://tiles.openfreemap.org/styles/liberty`; satelit `https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}`; hybrid menambah `Reference/World_Transportation` dan `Reference/World_Boundaries_and_Places`.
- Atribusi geoBoundaries (CC BY 3.0 IGO), OpenStreetMap, dan Esri wajib tampil.
- Teks UI dalam Bahasa Indonesia. Nomor migrasi = nomor bebas berikutnya di `origin/main` saat eksekusi (saat ini `00080`; `00078_bast_handover_locations` dan `00079_handover_location_per_slot` sudah ada). Worktree dibuat dari `origin/main`.
- Titik serah memakai `distribution_slots.handover_location_id` → `bast_handover_locations(id, regency_id, name)` lebih dulu; DBSCAN 30 m hanya untuk slot tanpa titik serah.

## Review Focus

1. Petugas yang cakupannya belum punya titik ber-GPS → peta tetap zoom ke batas kabupaten cakupannya, bukan ke seluruh Indonesia atau error (Task 6).
2. Foto dengan koordinat `0,0` atau lat/lng tertukar (lon di −11..6) → dianggap tidak punya lokasi, bukan titik di Teluk Guinea (Task 3).
3. Kabupaten master yang namanya tidak cocok dengan aset batas → titik tetap tampil dengan `location_status = "unmatched"` (Task 3).
4. Tile peta gagal dimuat (sinyal lemah) → pindah otomatis ke mode ringan dengan pesan, bukan kanvas kosong (Task 7).
5. Penerima diganti (replaced) setelah foto diunggah → titik mengikuti slot, dan milestone tidak naik karena dokumen BA revisi lama (Task 3).

---

### Task 1: PostGIS dan skema spasial

**Files:**
- Modify: `docker-compose.yml:3`, `.github/workflows/ci.yml:19,121`
- Create: `internal/database/migrations/00080_postgis_distribution_map.sql`
- Test: `internal/database/migrations/00080_postgis_distribution_map_test.go` (ikuti pola `00039_farmer_documentation_slots_test.go`)

**Interfaces:**
- Produces: extension `postgis`; tabel `regency_boundaries(boundary_name text PK, normalized_key text UNIQUE, regency_id uuid NULL, geom geometry(MultiPolygon,4326), source text, license text, asset_checksum text)`; kolom `media_files.geom geometry(Point,4326)` GENERATED STORED (NULL jika lat/lng NULL, atau keduanya 0); kolom `distribution_slots.linked_at timestamptz` + trigger yang mengisinya saat `status` berubah menjadi `linked` dan backfill `linked_at = updated_at` untuk slot `linked`/`completed`.

- [ ] **Step 1: Ganti image ke `postgis/postgis:17-3.5-alpine`** di compose dan kedua service CI.
- [ ] **Step 2: Tulis test gagal** `TestPostGISDistributionMapMigration`: `SELECT postgis_version()` tidak error; insert `media_files` dengan lat −6.45 lng 108.19 → `ST_X(geom)=108.19`, `ST_Y(geom)=-6.45`; lat/lng NULL → `geom IS NULL`; lat 0 lng 0 → `geom IS NULL`; update slot `open→linked` → `linked_at` terisi; GiST index `regency_boundaries_geom_idx` dan `media_files_geom_idx` ada.
- [ ] **Step 3: Jalankan** `go test ./internal/database/migrations/ -run TestPostGISDistributionMapMigration` → FAIL (tabel/kolom belum ada).
- [ ] **Step 4: Tulis migrasi** sesuai §6.2 spec plus `linked_at`, `asset_checksum`, dan bagian `-- +goose Down` (drop kolom/tabel/trigger; extension tidak di-drop).
- [ ] **Step 5: Jalankan test** → PASS; jalankan `go test ./internal/database/...` untuk memastikan migrasi lain tetap lolos.
- [ ] **Step 6: Commit** `feat(db): enable PostGIS and spatial columns for distribution map`.

### Task 2: Impor batas kabupaten

**Files:**
- Create: `internal/geo/boundaries.go`, `internal/geo/normalize.go`, `internal/geo/data/indonesia-regencies.json` (salinan aset frontend; tambahkan test yang memastikan checksum sama dengan `frontend/src/features/dashboard/data/indonesia-regencies.json`)
- Modify: `cmd/server/main.go` (panggil saat start, setelah migrasi)
- Test: `internal/geo/normalize_test.go`, `internal/geo/boundaries_integration_test.go`

**Interfaces:**
- Produces: `geo.NormalizeRegencyName(name string) string` (port persis `normalizeRegencyName` di `frontend/src/features/dashboard/mapRegions.ts`); `geo.EnsureRegencyBoundaries(ctx context.Context, pool *pgxpool.Pool) (BoundaryImportResult, error)`; `type BoundaryImportResult struct { Imported, Matched int; Unmatched []string }` (Unmatched = nama regency master tanpa batas).

- [ ] **Step 1: Tulis test gagal** `TestNormalizeRegencyName`: `"KAB. BANGKA BARAT"→"BANGKABARAT"`, `"Kabupaten Bandung"→"BANDUNG"`, `"KOTA BANDUNG"→"KOTABANDUNG"`, `"Bangka Barat"→"BANGKABARAT"`.
- [ ] **Step 2: Tulis test gagal** `TestEnsureRegencyBoundaries`: tabel kosong → `Imported=518`; regency master `KAB. INDRAMAYU` ter-match; panggilan kedua dengan checksum sama tidak menulis ulang (`Imported=0`); `ST_IsValid(geom)` true untuk semua baris (perbaiki dengan `ST_MakeValid` saat insert); regency master `KAB. TIDAK ADA` muncul di `Unmatched`.
- [ ] **Step 3: Jalankan** `go test ./internal/geo/...` → FAIL.
- [ ] **Step 4: Implementasi.** Aset di-`embed`; ring `[lon,lat]` dirakit jadi GeoJSON MultiPolygon lalu `ST_Multi(ST_MakeValid(ST_SetSRID(ST_GeomFromGeoJSON($1),4326)))`; semua dalam satu transaksi dengan `COPY` atau batch; pencocokan `regency_id` lewat `NormalizeRegencyName(regencies.name)`. Log `Unmatched` saat start.
- [ ] **Step 5: Jalankan test** → PASS.
- [ ] **Step 6: Commit** `feat(geo): import regency boundaries into PostGIS`.

### Task 3: Titik, milestone, status lokasi, titik serah

**Files:**
- Create: `internal/distmap/models.go`, `internal/distmap/repository.go`, `internal/distmap/service.go`
- Modify: `internal/recipients/repository.go` (ekspor `RecipientFilterSQL(filter, scope) (from, where string, args []any)` agar `distmap` memakai filter yang sama, tanpa menyalin SQL)
- Test: `internal/distmap/repository_integration_test.go` (pakai ulang `seedRecipientFixture`; jika tidak bisa diimpor lintas paket, pindahkan ke `internal/recipients/recipienttest`)

**Interfaces:**
- Consumes: `regency_boundaries`, `media_files.geom`, `distribution_slots.linked_at` (Task 1–2).
- Produces:
  - `type Point struct { SlotID, Code, ScheduleID, RegencyID, ZoneID, SiteID string; Lon, Lat float64; Stage int; StageAt []time.Time; LocationStatus string; SiteName string; SiteOutlier bool }` (`LocationStatus` ∈ `inside|outside|unmatched`; `SiteID` = `hl:<handover_location_id>` atau `geo:<schedule_id>:<cluster>`; `SiteName` kosong untuk `geo:`)
  - `type PointsMeta struct { Total, Located, Unlocated, Outside int }`
  - `type PointQuery struct { Filter recipients.Filter; BBox *[4]float64; Stage int }`
  - `(*Repository).Points(ctx, q PointQuery, scope auth.RegencyScope) ([]Point, PointsMeta, error)`
  - `(*Repository).UnlocatedSlots(ctx, q PointQuery, scope auth.RegencyScope) ([]UnlocatedSlot, error)` dengan `UnlocatedSlot{SlotID, Code, RegencyID string; Stage int}`

- [ ] **Step 1: Tulis test gagal** `TestPointsMilestones`: lima slot dibuat masing-masing di tahap 1..5 → `Stage` 1..5 dan `len(StageAt) == Stage`; slot `cancelled` tidak muncul; slot dengan BA perorangan revisi lama `superseded` dan tanpa `final` tetap `Stage=4`.
- [ ] **Step 2: Tulis test gagal** `TestPointsLocationSource`: slot dengan foto tahap `dokumen` (lokasi A) dan `penyerahan` (lokasi B) → titik di B; dua foto penyerahan → yang `sort_order` slot dokumentasinya lebih kecil; foto `rejected` diabaikan; foto lat/lng `0,0` diabaikan → slot masuk `Unlocated`.
- [ ] **Step 3: Tulis test gagal** `TestPointsLocationStatus`: titik di dalam Indramayu → `inside`; 300 m di luar batas → `inside` (buffer 500 m); di Cirebon untuk jadwal Indramayu → `outside` dan `meta.Outside=1`; regency tanpa batas → `unmatched`.
- [ ] **Step 4: Tulis test gagal** `TestPointsSitesAndScope`: dua slot dengan `handover_location_id` sama tetapi berjauhan 300 m → `SiteID` sama (`hl:`) dan `SiteName` = nama lokasi; anggota 2 km dari median titik serahnya → `SiteOutlier=true`; tiga slot tanpa titik serah dalam radius 20 m di jadwal sama → `SiteID` sama (`geo:`); slot tanpa titik serah 100 m jauhnya → `SiteID` berbeda; scope kabupaten lain → 0 titik; `BBox` di luar titik → 0 titik; `Stage=3` hanya mengembalikan tahap 3.
- [ ] **Step 5: Jalankan** `go test ./internal/distmap/...` → FAIL.
- [ ] **Step 6: Implementasi.** Satu query: CTE slot terfilter (lewat `RecipientFilterSQL` + slot terbuka tanpa alokasi pada jadwal yang lolos filter, seperti CTE slot di `MapRegions`) → `LEFT JOIN LATERAL` foto sumber koordinat (urutan §4.1 spec) → tahap via `CASE` (bukti lengkap memakai `evidenceStatusSQL`) → `location_status` dengan `ST_DWithin(rb.geom::geography, m.geom::geography, 500)` → `SiteID` = `hl:<handover_location_id>` jika ada, selain itu `ST_ClusterDBSCAN(ST_Transform(geom, 3857), eps := 30, minpoints := 1) OVER (PARTITION BY schedule_id)` → `geo:<schedule_id>:<cluster>`; `SiteOutlier` = jarak ke median anggota titik serah `hl:` > 1000 m. Catatan: eps 3857 dalam meter terdistorsi kecil di khatulistiwa (±1%), cukup.
- [ ] **Step 7: Jalankan test** → PASS.
- [ ] **Step 8: Commit** `feat(distmap): distribution points with milestones and location checks`.

### Task 4: API peta

**Files:**
- Create: `internal/api/map_routes.go`, `internal/distmap/zones.go`
- Modify: `internal/api/handler.go` (route `map/points`, `map/unlocated`, `map/zones`, `map/config`, prefix `map/regencies/`; dependensi `DistributionMap`), `cmd/server/main.go`, `internal/config` (env `MAP_STYLE_URL`, `MAP_SATELLITE_TILES`, `MAP_HYBRID_OVERLAY_TILES`, `MAP_ATTRIBUTION`, `MAP_TILE_API_KEY` dengan default dari Global Constraints), `internal/recipients/models.go` + `repository.go` (`ZoneID`, `ZoneCode`, `ZoneName` di `MapRegion` saat `program_id` diisi)
- Test: `internal/api/map_routes_test.go`, `internal/distmap/zones_integration_test.go`, tambahan di `TestMapRegionsAggregatesRecipientsEvidenceAndSlotProgress`

**Interfaces:**
- Consumes: `distmap.Repository.Points`, `UnlocatedSlots` (Task 3).
- Produces (HTTP, semua `GET`, dibungkus `writeData`):
  - `/api/v1/map/points?…&bbox=&stage=` → `{ "type":"FeatureCollection", "features":[…], "meta":{total,located,unlocated,outside} }` (properti sesuai §7.1 spec, `stage_at` RFC 3339 UTC)
  - `/api/v1/map/unlocated?…` → `[{slot_id, code, regency_id, stage}]`
  - `/api/v1/map/zones?program_id=` → FeatureCollection dengan `zone_id, code, name, is_placeholder, regency_count`
  - `/api/v1/map/regencies/{id}/boundary` → Feature, 404 jika tidak ada batas
  - `/api/v1/map/config` → `{style_url, satellite_tiles, hybrid_overlay_tiles[], attribution, api_key}`
  - `distmap.(*Repository).ZoneBoundaries(ctx, programID string, scope auth.RegencyScope) ([]ZoneBoundary, error)` dengan cache memori per program, dibuang lewat `InvalidateZones(programID)` yang dipanggil oleh service program saat assignment berubah.

- [ ] **Step 1: Tulis test handler gagal**: tanpa permission → 403; `bbox=a,b` → 400 `validation_failed` dengan field `bbox`; `stage=9` → 400; respons titik tidak mengandung kunci `full_name`/`nik`; scope dari principal diteruskan ke repository (pakai fake seperti test `recipients/map` di `handler_test.go:1583`).
- [ ] **Step 2: Tulis test integrasi gagal** `TestZoneBoundaries`: dua kabupaten di zona sama → satu Feature `regency_count=2` yang `ST_Area`-nya ≈ jumlah keduanya; zona placeholder → `is_placeholder=true`; setelah `InvalidateZones`, perubahan assignment terlihat.
- [ ] **Step 3: Tambah asersi** di test `MapRegions`: dengan `program_id`, region membawa `zone_code` fixture.
- [ ] **Step 4: Jalankan** `go test ./internal/api/... ./internal/distmap/... ./internal/recipients/...` → FAIL.
- [ ] **Step 5: Implementasi** route, handler (ikuti `handleRecipientMap`), konfigurasi, zona (`ST_SimplifyPreserveTopology(ST_Union(geom), 0.005)`), wiring.
- [ ] **Step 6: Jalankan test** → PASS; `go vet ./...` bersih.
- [ ] **Step 7: Commit** `feat(api): distribution map points, zones, boundaries and config endpoints`.

### Task 5: Fondasi peta MapLibre di frontend

**Files:**
- Modify: `frontend/package.json` (`maplibre-gl@4.7.1`)
- Create: `frontend/src/features/dashboard/map/types.ts`, `basemaps.ts`, `MapCanvas.tsx`, `useMapConfig.ts`, `useMapUrlState.ts`, `milestones.ts`
- Test: `basemaps.test.ts`, `useMapUrlState.test.tsx`, `milestones.test.ts`

**Interfaces:**
- Consumes: `/api/v1/map/config` (Task 4).
- Produces:
  - `type BasemapMode = 'map' | 'sat' | 'hyb'`; `type ColorMode = 'zone' | 'evidence' | 'progress' | 'recipients'`
  - `MILESTONES: { stage: 1|2|3|4|5; label: string; color: string }[]` dan `REVIEW_COLOR`
  - `applyBasemap(map: maplibregl.Map, mode: BasemapMode, config: MapConfig): void` (hanya mengubah `visibility`; layer `konkit-*` tidak tersentuh)
  - `useMapUrlState(): { state: MapUrlState; update(patch: Partial<MapUrlState>): void }` dengan `MapUrlState = { center?: [number, number]; zoom?: number; base: BasemapMode; color: ColorMode; slot?: string; site?: string; stage?: number }` (filter tetap di param yang sudah dipakai Data Penerima)
  - `<MapCanvas config onReady={(map) => …} onStyleError={() => …} />` dengan `ResizeObserver` → `map.resize()`, NavigationControl, ScaleControl; `onStyleError` dipanggil jika event `load` tidak datang dalam 8000 ms atau ada `error` style.

- [ ] **Step 1: Tulis test gagal**: `applyBasemap` dengan map mock → mode `sat` menyembunyikan semua layer style dasar, menampilkan `konkit-img`, tidak menyentuh `konkit-dots`; `hyb` menampilkan juga dua overlay; `useMapUrlState` membaca dan menulis `base=hyb&color=zone&slot=…` tanpa menghapus `program_id`; urutan dan warna `MILESTONES` sama dengan Global Constraints.
- [ ] **Step 2: Jalankan** `npm --prefix frontend test -- map` → FAIL.
- [ ] **Step 3: Implementasi.** `maplibre-gl` di-import dinamis di `MapCanvas` agar menjadi chunk terpisah.
- [ ] **Step 4: Jalankan test**, `npm --prefix frontend run typecheck`, `npm --prefix frontend run lint` → PASS.
- [ ] **Step 5: Commit** `feat(map): MapLibre canvas, basemap modes and URL state`.

### Task 6: Lapis titik, zona, filter milestone, timeline, pencarian

**Files:**
- Create: `frontend/src/features/dashboard/map/layers.ts`, `useMapPoints.ts`, `useZoneBoundaries.ts`, `MilestoneFilter.tsx`, `TimelinePlayer.tsx`, `ZoneBreadcrumb.tsx`, `MapSearch.tsx`, `fitTargets.ts`
- Modify: `frontend/src/features/dashboard/DistributionMapPage.tsx` (susunan halaman baru sesuai mockup; tabel progres dikelompokkan per zona)
- Test: `layers.test.ts`, `fitTargets.test.ts`, `TimelinePlayer.test.tsx`, `MilestoneFilter.test.tsx`, `DistributionMapPage.test.tsx` (perbarui)

**Interfaces:**
- Consumes: Task 4 endpoint, Task 5 `MapCanvas`, `MILESTONES`, `useMapUrlState`.
- Produces:
  - `installLayers(map, { points, zones, regions, colorMode }): void` dan `updateLayers(...)` dengan id layer `konkit-regions`, `konkit-zone-line`, `konkit-zone-label`, `konkit-clusters`, `konkit-cluster-count`, `konkit-dots`, `konkit-sites`, `konkit-focus`
  - `stageAtDay(point, day: Date): number` (0 = belum muncul) untuk timeline
  - `initialFitTarget(points: FeatureCollection, scopeRegencyIds: string[], boundaries): LngLatBoundsLike` → batas titik jika ada, jika tidak batas kabupaten cakupan, jika tidak seluruh Indonesia `[[94,-11.5],[141.5,6.5]]`

- [ ] **Step 1: Tulis test gagal**: `initialFitTarget` dengan 0 titik dan scope `[Indramayu]` → bbox Indramayu (Review Focus 1); dengan titik → bbox titik; `stageAtDay` sebelum `stage_at[0]` → 0, di antara tahap 2 dan 3 → 2; `MilestoneFilter` menampilkan jumlah per tahap dan `aria-pressed`; mode warna `zone` memberi zona placeholder warna abu `#9aa39c`; penanda situs dengan beberapa slot memakai warna milestone **terendah**.
- [ ] **Step 2: Jalankan** test → FAIL.
- [ ] **Step 3: Implementasi** lapis, filter, timeline (interval 700 ms per hari, berhenti di hari terakhir), ringkasan cluster per zona saat zoom < 7 dan mode Zona aktif, `refetchInterval: 60_000` + refetch saat fokus pada `useMapPoints`, breadcrumb `Indonesia › Zona › Kabupaten`, pencarian kabupaten dari aset batas + nomor bagi dari titik, `fitBounds` padding 60 px (desktop kiri ditambah lebar panel).
- [ ] **Step 4: Jalankan test**, typecheck, lint → PASS.
- [ ] **Step 5: Commit** `feat(map): points, zones, milestone filter, timeline and search`.

### Task 7: Panel rincian, bottom sheet HP, titik tanpa lokasi, mode ringan

**Files:**
- Create: `frontend/src/features/dashboard/map/PointDetailPanel.tsx`, `PointBottomSheet.tsx`, `SiteList.tsx`, `UnlocatedDialog.tsx`, `LightModeNotice.tsx`
- Modify: `DistributionMapPage.tsx` (fallback ke `DistributionMap.tsx` saat `onStyleError`; tombol "Mode ringan" manual), `frontend/src/features/guide/topics/` (topik panduan Map Distribusi)
- Test: `PointDetailPanel.test.tsx`, `PointBottomSheet.test.tsx`, `DistributionMapPage.test.tsx`

**Interfaces:**
- Consumes: endpoint slot yang sudah ada (`/api/v1/distribution/slots/{id}`) untuk nama, foto, mesin; `/api/v1/map/unlocated`.

- [ ] **Step 1: Tulis test gagal**: klik titik (simulasikan `onSelect(slotId)`) → panel memuat milestone dengan tanggal dari `stage_at` (format `id-ID`, zona `Asia/Jakarta`) dan tautan "Buka nomor bagi"; situs dengan 4 slot → `SiteList` berisi 4 baris; viewport < 768 px → `PointBottomSheet` dipakai, bukan panel; badge "N belum ada lokasi" membuka dialog daftar; `onStyleError` → `DistributionMap` (SVG) dirender dan pesan "Peta ringan dipakai karena peta dasar gagal dimuat" tampil (Review Focus 4); `location_status="outside"` → peringatan "Perlu ditinjau" tampil.
- [ ] **Step 2: Jalankan** test → FAIL.
- [ ] **Step 3: Implementasi.** Tautan "Arahkan ke lokasi": `https://www.google.com/maps/dir/?api=1&destination={lat},{lon}`.
- [ ] **Step 4: Jalankan test**, typecheck, lint → PASS.
- [ ] **Step 5: Commit** `feat(map): point detail, mobile bottom sheet, unlocated list and light mode`.

### Task 8: E2E dan verifikasi menyeluruh

**Files:**
- Create: `frontend/e2e/distribution-map.spec.ts`
- Modify: `cmd/e2eseed/main.go` (beberapa foto ber-GPS di dalam dan satu di luar kabupaten, satu situs bertumpuk)

- [ ] **Step 1: Tulis E2E**: buka `/dashboard/map-distribusi` → titik/cluster tampil; pilih zona → breadcrumb berubah; klik situs → daftar 2+ slot; viewport 390×844 → bottom sheet; ganti ke Hybrid → tidak ada error konsol; route tile diblokir (`page.route('**/tiles.openfreemap.org/**', r => r.abort())`) → mode ringan tampil.
- [ ] **Step 2: Jalankan** suite E2E sesuai skill `run-konkit` → PASS.
- [ ] **Step 3: Jalankan** `go test ./...`, `npm --prefix frontend test`, typecheck, lint → semua PASS.
- [ ] **Step 4: Perbarui** status spec 10-09 (Lapis B/C → digantikan v2) dan status spec v2 → "Terimplementasi".
- [ ] **Step 5: Commit** `test(map): end-to-end coverage for distribution map v2`.
