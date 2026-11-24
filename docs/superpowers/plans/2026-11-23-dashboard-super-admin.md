# Dashboard Super Admin Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Halaman `/dashboard` (khusus super admin) dengan grafik progres satu program: kartu total, per zona, per kabupaten dikelompokkan per zona, penyerahan per hari, dan dokumentasi kegiatan.

**Architecture:**
- Backend: `Dashboard` ditaruh sebagai method baru di `internal/monitoring.Service`. Ini menyimpang sedikit dari spec yang menyebut paket baru. Alasannya, method ini bisa memakai ulang `photoCounts` dan daftar program apa adanya, sehingga angka Diserahkan dan Foto disetujui pasti sama dengan Monitoring.
- API: route sendiri `GET /api/v1/dashboard/summary` dengan permission `dashboard.view`.
- Frontend: fitur baru yang di-load lazy, grafik memakai Recharts.

**Tech Stack:** Go + pgx, PostgreSQL (goose), React 19, TanStack Query, Recharts 2.15.x, Vitest.

**Spec:** `docs/superpowers/specs/2026-11-22-dashboard-super-admin-design.md`

## Global Constraints

- Branch: `feat/super-admin-dashboard`, dibuat dari `origin/main`. Satu PR.
- Migrasi berikutnya: `00112_dashboard_permission.sql`. Permission `dashboard.view` hanya diberikan ke `super_admin`.
- Scope dihitung untuk jadwal yang `ps.status <> 'cancelled'` dan mengikuti `auth.RegencyScope`.
- Tanggal harian memakai `(completed_at AT TIME ZONE 'Asia/Jakarta')::date`.
- Jangan ubah kontrak API yang sudah ada. Endpoint ini murni tambahan.
- Teks UI dalam Bahasa Indonesia, angka diformat `id-ID`.
- Ikuti `design-system/konkit/MASTER.md`.
- Recharts hanya boleh masuk chunk lazy halaman dashboard.
- Jangan sentuh dev server di `:8080`.
- Test dijalankan dengan `TEST_DATABASE_URL` yang mengarah ke `konkit_test` dan `-p 1`.
- Commit diakhiri `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Review Focus

1. **Kabupaten tanpa zona** (tidak punya `program_regency_assignments.zone_id`) tetap tampil di grup "Tanpa zona" dan ikut dijumlahkan di total. Test ada di Task 1.
2. **Kuota 0 atau program tanpa jadwal**: persentase tidak boleh NaN/Infinity (tampil `0%`), dan muncul state kosong. Test ada di Task 3.
3. **Slot `cancelled` atau slot `open` tanpa alokasi** tidak dihitung sebagai Tertaut. Test ada di Task 1.
4. **BA final ganda** untuk satu slot (versi lama `superseded` atau dokumen final lebih dari satu) hanya dihitung sekali. Test ada di Task 1.
5. **Pengguna dengan scope kabupaten terbatas** hanya melihat kabupatennya, termasuk di bagian harian dan dokumentasi. Test ada di Task 1.

---

### Task 1: Query dashboard + permission

**Files:**
- Create: `internal/database/migrations/00112_dashboard_permission.sql` (pola sama dengan `00106_documentation_slot_examples.sql`: `INSERT INTO permissions`, `INSERT INTO role_permissions` untuk `super_admin`, lalu Down menghapus keduanya).
- Create: `internal/monitoring/dashboard.go`
- Test: `internal/monitoring/dashboard_integration_test.go` (memakai `testPool`, `must`, dan gaya `seed` dari `monitoring_integration_test.go`, dengan fixture sendiri).

**Interfaces:**
- Produces (`internal/monitoring/dashboard.go`):

```go
type DashboardStages struct {
	Quota     int `json:"quota"`
	Linked    int `json:"linked"`
	Completed int `json:"completed"`
	Approved  int `json:"approved"`
	BAFinal   int `json:"ba_final"`
}
type DashboardRegency struct {
	ScheduleID   string `json:"schedule_id"`
	RegencyID    string `json:"regency_id"`
	RegencyName  string `json:"regency_name"`
	ProvinceName string `json:"province_name"`
	ZoneName     string `json:"zone_name"` // "" = tanpa zona
	ZoneSort     int    `json:"zone_sort"`
	DashboardStages
}
type DashboardZone struct {
	ZoneName string `json:"zone_name"`
	ZoneSort int    `json:"zone_sort"`
	DashboardStages
}
type DashboardDay struct {
	Date      string `json:"date"` // YYYY-MM-DD WIB
	Completed int    `json:"completed"`
}
type DashboardActivity struct {
	ActivityType string `json:"activity_type"`
	Media        int    `json:"media"`
}
type Dashboard struct {
	Programs   []Program           `json:"programs"`
	ProgramID  string              `json:"program_id"`
	Totals     DashboardStages     `json:"totals"`
	Zones      []DashboardZone     `json:"zones"`
	Regencies  []DashboardRegency  `json:"regencies"`
	Daily      []DashboardDay      `json:"daily"`
	Activities []DashboardActivity `json:"activities"`
}

func (s *Service) Dashboard(ctx context.Context, programID string, scope auth.RegencyScope) (Dashboard, error)
```

- [ ] **Step 1: Tulis test yang gagal.**

  Fixture:
  - satu program;
  - kabupaten A di zona "Zona I" (sort 1), dengan `slot_quota` 10;
  - kabupaten B tanpa zona, dengan `slot_quota` 5.

  Slot di A:
  - slot 1: completed 2026-11-20 (WIB), semua foto approved, punya 2 baris `bast_individual_documents` (satu `final`, satu `superseded`);
  - slot 2: completed 2026-11-21, ada foto yang belum dicek;
  - slot 3: `linked` dengan alokasi;
  - slot 4: `cancelled`.

  Slot di B:
  - slot 1: `open` tanpa alokasi.

  `activity_media`:
  - 2 baris `rakor` aktif untuk A dan program ini;
  - 1 baris `rakor` dengan `status = 'deleted'`;
  - 1 baris `rakor` milik program lain.

  Tests:
  - `TestDashboardCountsEachStage`:
    - Totals = `{Quota:15, Linked:3, Completed:2, Approved:1, BAFinal:1}`;
    - regency A = `{10,3,2,1,1}`, B = `{5,0,0,0,0}`;
    - `Zones` berurutan `["Zona I", ""]` dengan angka yang sama dengan regency-nya;
    - `Daily` = `[{2026-11-20,1},{2026-11-21,1}]`;
    - `Activities` = `[{rakor,2}]`.
  - `TestDashboardFollowsRegencyScope`: dengan scope `{RegencyIDs: [B]}`, `Regencies` hanya B, Totals.Quota = 5, `Daily` dan `Activities` kosong (slice non-nil).
  - `TestDashboardUnknownProgram`: program ID acak mengembalikan `ErrProgramNotFound`. Program ID kosong memilih program default, sama dengan `Overview`.

- [ ] **Step 2:** Jalankan `go test -p 1 ./internal/monitoring -run Dashboard`. Harus FAIL dengan pesan `undefined: Dashboard`.

- [ ] **Step 3: Implementasi `Dashboard`.**
  - Daftar program dan pemilihan default sama dengan `Overview`. Ekstrak keduanya ke helper bersama `programs(ctx) ([]Program, error)` dan `pickProgram`, supaya `Overview` ikut memakainya. Tidak boleh ada salinan.
  - Tiga query:
    1. **Per jadwal.** Join yang sama dengan `Overview`: `program_schedules`, `regencies`, `program_regency_assignments`, dan `program_zones`, ditambah `WITH photos AS (photoCounts)`. Hitungan:
       - Linked = `count(ds.id) FILTER (WHERE ds.allocation_id IS NOT NULL AND ds.status IN ('linked','completed'))`;
       - Approved memakai filter yang sama dengan Overview;
       - BAFinal = `count(ds.id) FILTER (WHERE ds.status='completed' AND EXISTS (SELECT 1 FROM bast_individual_documents d WHERE d.distribution_slot_id = ds.id AND d.status='final'))`;
       - urutan sama dengan Overview.
    2. **Harian.** Slot completed `GROUP BY` tanggal WIB, `ORDER BY` tanggal.
    3. **Dokumentasi.** `activity_media` dengan `program_id = $1`, `status = 'active'`, dan regency di scope, `GROUP BY activity_type ORDER BY count DESC, activity_type`.
  - Totals dan Zones dijumlahkan di Go dari baris jadwal. Urutan zona mengikuti kemunculan pertama.

- [ ] **Step 4:** Jalankan `go test -p 1 ./internal/monitoring` dan `go vet ./...`. Harus PASS.

- [ ] **Step 5:** Commit dengan pesan `feat(monitoring): dashboard numbers per zone and kabupaten`.

### Task 2: Endpoint `GET /api/v1/dashboard/summary`

**Files:**
- Create: `internal/api/dashboard_routes.go`
- Modify: `internal/api/handler.go`. Tambahkan field deps `Dashboard DashboardService` di dekat `Monitoring` (baris ~328) dan case `path == "dashboard/summary"` di dekat case `monitoring/` (baris ~421).
- Modify: `cmd/server/main.go:248`. Pakai instance `monitoring.NewService` yang sama untuk `Monitoring` dan `Dashboard`.
- Test: `internal/api/dashboard_routes_test.go`. Ikuti pola test route yang sudah ada di `internal/api` dengan fake service.

**Interfaces:**
- Consumes: `monitoring.Service.Dashboard` dan `monitoring.ErrProgramNotFound`.
- Produces: `type DashboardService interface { Dashboard(ctx context.Context, programID string, scope auth.RegencyScope) (monitoring.Dashboard, error) }`. Response `{"data": monitoring.Dashboard}`.

- [ ] **Step 1: Tulis test yang gagal.**
  - `TestDashboardSummaryNeedsPermission`: principal dengan `monitoring.view` saja mendapat 403.
  - `TestDashboardSummaryReturnsData`: principal dengan `dashboard.view` dan `program_id=p1` mendapat 200. Fake menerima `p1`, dan body `data.program_id == "p1"`.
  - `TestDashboardSummaryUnknownProgram`: fake mengembalikan `ErrProgramNotFound`, response 404 `not_found`.
  - Selain GET, response 404.
- [ ] **Step 2:** Jalankan `go test ./internal/api -run DashboardSummary`. Harus FAIL.
- [ ] **Step 3: Implementasi handler.**
  - Urutan: `authorize(..., "dashboard.view")`, lalu `regencyScope`, lalu `service.Dashboard(r.Context(), query.Get("program_id"), scope)`.
  - Hasil dan error ditulis lewat `writeMonitoringResult`.
  - `nil` service menghasilkan `writeUnavailable`.
- [ ] **Step 4:** Jalankan `go test -p 1 ./internal/api ./internal/monitoring` dan `go vet ./...`. Harus PASS.
- [ ] **Step 5:** Commit dengan pesan `feat(api): dashboard summary endpoint for super admin`.

### Task 3: Halaman Dashboard (frontend)

**Files:**
- Modify: `frontend/package.json`. Jalankan `npm install recharts@2.15.4` di `frontend/`.
- Create: `frontend/src/features/super-dashboard/types.ts`. Cerminan tipe Go dari Task 1 dengan nama JSON yang sama.
- Create: `frontend/src/features/super-dashboard/api.ts` berisi `useDashboardSummary(programId: string)`.
  - Query key `['dashboard','summary',programId]`.
  - URL `/api/v1/dashboard/summary?program_id=`. Program kosong berarti parameter tidak dikirim.
  - `refetchInterval: 5 * 60_000`.
- Create: `frontend/src/features/super-dashboard/stats.ts` berisi:
  - `percent(part: number, quota: number): number`, dibulatkan dan bernilai 0 bila kuota ≤ 0;
  - `cumulative(days: DashboardDay[]): { date: string; completed: number; total: number }[]`;
  - `zoneLabel(name: string): string`, dengan `""` menjadi `"Tanpa zona"`.
- Create: `frontend/src/features/super-dashboard/SuperDashboardPage.tsx`. Bagian-bagiannya:
  - pemilih program (`SearchableSelect`/Select dari `programs`);
  - 5 kartu dengan label persis `Kuota`, `Tertaut`, `Diserahkan`, `Foto disetujui`, `BA final`, berisi angka dan persen dari kuota (kecuali kartu Kuota);
  - "Progres per zona": BarChart horizontal, satu baris per zona, 4 seri tahap dibandingkan kuota;
  - "Per kabupaten": judul per zona, lalu BarChart per zona. Klik bar menuju `/monitoring-distribusi`;
  - "Penyerahan per hari": ComposedChart, Bar `completed` + Line `total` + `ReferenceLine` di nilai kuota;
  - "Dokumentasi kegiatan": BarChart horizontal dengan label dari `activityTypeLabels` (`frontend/src/features/activities/types.ts`). Jenis yang tidak dikenal memakai kode mentah;
  - empty state "Program ini belum punya jadwal kabupaten" bila `regencies` kosong, plus state loading dan error dengan tombol Coba lagi;
  - satu kolom di lebar < `md`.
- Modify: `frontend/src/app/routes.tsx`. Tambahkan `{ path: 'dashboard', element: <ProtectedPage permission="dashboard.view"><Suspense fallback={…}><SuperDashboardPage/></Suspense></ProtectedPage> }`, dengan `SuperDashboardPage = lazy(() => import(...).then(m => ({ default: m.SuperDashboardPage })))`.
- Modify: `frontend/src/app/AppShell.tsx:33-35`. Item pertama grup Ringkasan menjadi `{ label: 'Dashboard', to: '/dashboard', permission: 'dashboard.view', icon: <LayoutDashboard /> }`.
- Test: `frontend/src/features/super-dashboard/SuperDashboardPage.test.tsx` dan `stats.test.ts`. Mock `apiRequest` seperti di `MonitoringPage.test.tsx`. Mock `recharts` `ResponsiveContainer` agar punya ukuran di jsdom (atau render children dengan width/height tetap).

**Interfaces:**
- Consumes: response JSON dari Task 2.
- Produces: rute `/dashboard` dan export `SuperDashboardPage`.

- [ ] **Step 1: Tulis test yang gagal.**
  - `stats.test.ts`: `percent(1,0) === 0`, `percent(1,3) === 33`, `cumulative` menjumlah berurutan, `zoneLabel('') === 'Tanpa zona'`.
  - Test halaman:
    - render data tiruan (2 zona, salah satunya `""`) lalu periksa kartu `Diserahkan` menampilkan angka berformat `id-ID` dan persen, serta muncul judul "Zona I" dan "Tanpa zona";
    - `regencies: []` menampilkan teks empty state;
    - API reject menampilkan pesan error dan tombol "Coba lagi";
    - mengganti program memanggil URL dengan `program_id` baru.
  - Test AppShell/rute (tambahkan ke test AppShell yang sudah ada, bila ada): tanpa `dashboard.view`, menu "Dashboard" tidak tampil.
- [ ] **Step 2:** Jalankan `npx vitest run src/features/super-dashboard`. Harus FAIL.
- [ ] **Step 3:** Implementasi file-file di atas.
- [ ] **Step 4:** Jalankan `npx vitest run`, `npm run typecheck`, dan `npx eslint src`. Semua harus PASS. Lalu jalankan `npm run build` dan pastikan Recharts ada di chunk terpisah, bukan di `index-*.js`. Cek dengan grep nama `recharts` di output build.
- [ ] **Step 5:** Commit dengan pesan `feat(web): super admin dashboard with charts per zone`.

### Task 4: Panduan, verifikasi penuh, dan PR

**Files:**
- Create: `frontend/src/features/guide/topics/dashboard.tsx`. Bentuk `GuideTopic` mengikuti `laporan.tsx`, dengan:
  - `view: ['dashboard.view']`, `route: '/dashboard'`;
  - field per kartu yang menjelaskan definisi angka dari spec;
  - FAQ "Angka beda dengan Monitoring?" (Diserahkan dan Foto disetujui dihitung sama) dan "Kenapa kabupaten masuk Tanpa zona?" (belum diberi zona di Persiapan program).
- Modify: daftar topik di `frontend/src/features/guide/guideTopics.ts` atau `guideContent.tsx` (tempat topik lain didaftarkan).

- [ ] **Step 1:** Daftarkan topik. Jalankan `npx vitest run src/features/guide` dan pastikan test panduan yang memeriksa semua topik tetap PASS.
- [ ] **Step 2:** Jalankan verifikasi penuh: `go vet ./...`, `go test -p 1 ./...` (dengan `TEST_DATABASE_URL`), `npx vitest run`, `npm run typecheck`, dan `npx eslint src`. Kegagalan lama di `administration`, `bast`, atau `geo` karena sisa data `konkit_test` dicatat, bukan diperbaiki.
- [ ] **Step 3:** Uji end-to-end. Jalankan server di `:8099` dengan DB lokal, login sebagai super admin, lalu `GET /api/v1/dashboard/summary` harus mengembalikan 200. Akun non super admin harus mendapat 403.
- [ ] **Step 4:** Commit, push, lalu buat PR lewat GitHub API (token dari `git credential fill` di `python -I`, tidak di-echo). Body PR menyebut migrasi `00112` yang perlu `migrate up` di VPS, dan diakhiri baris atribusi Claude Code.
