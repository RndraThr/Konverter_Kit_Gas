# Rancangan Data Penerima (Master Data Lintas Kabupaten)

Versi: 1.0
Tanggal: 2026-09-12
Status: Menunggu review pengguna

## 1. Tujuan

Halaman `Dashboard > Data Penerima` saat ini adalah placeholder ("Modul sedang disiapkan"). Fase ini menjadikannya pusat data penerima bantuan lintas kabupaten dan program, dengan kemampuan:

- Melihat seluruh data penerima (bukan per-jadwal seperti Pendistribusian/Laporan saat ini) dengan statistik ringkas.
- Mencari dan memfilter data secara kombinatif (nama/NIK/no. kartu, kabupaten, program, jenis program, status alokasi, status distribusi).
- Tabel dengan pagination server-side yang konsisten dengan pola yang sudah ada (halaman Pengguna).
- CRUD penuh: tambah penerima manual, edit identitas, batalkan (soft delete), dan pulihkan.

## 2. Non-tujuan

- Tidak mengubah alur import DCP3 maupun alur Pendistribusian (verifikasi lapangan, dokumentasi foto, penyelesaian distribusi) yang sudah ada — keduanya tetap menjadi sumber utama data dan tempat status alokasi/distribusi berubah secara normal.
- Halaman ini **tidak** bisa mengubah status alokasi/distribusi (mis. menandai "distributed") — itu tetap eksklusif milik alur Pendistribusian, supaya tidak ada dua jalur yang saling menimpa status.
- Tidak membangun ulang model people/allocation yang sudah ada; hanya menambahkan cara baru untuk membaca lintas jadwal dan operasi create/update/cancel/restore yang belum ada.
- Tidak menambah ekspor Excel/PDF pada fase ini (sudah ada di halaman Laporan, per-jadwal).

## 3. Model Data

Menggunakan tabel yang sudah ada (`internal/database/migrations/00004_program_dcp3_distribution.sql`): `people`, `person_sector_identifiers`, `candidate_nominations`, `package_allocations`, `program_schedules`, `programs`, `regencies`, `distribution_records`. Tidak ada tabel baru.

### 3.1 Migration baru: `00007_recipients.sql`

Satu perubahan skema wajib: `candidate_nominations.import_row_id` saat ini `NOT NULL REFERENCES dcp3_import_rows(id)`. Penerima manual tidak berasal dari baris import, sehingga kolom ini perlu jadi nullable:

```sql
-- +goose Up
ALTER TABLE candidate_nominations ALTER COLUMN import_row_id DROP NOT NULL;

INSERT INTO permissions (code, name, description) VALUES
    ('recipients.view', 'Lihat Data Penerima', 'Melihat data penerima lintas kabupaten dan program'),
    ('recipients.manage', 'Kelola Data Penerima', 'Menambah, mengubah, membatalkan, dan memulihkan data penerima')
ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id FROM roles CROSS JOIN permissions
WHERE roles.code = 'super_admin' AND permissions.code IN ('recipients.view', 'recipients.manage')
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM permissions WHERE code IN ('recipients.view', 'recipients.manage');
ALTER TABLE candidate_nominations ALTER COLUMN import_row_id SET NOT NULL;
```

Catatan: `ALTER COLUMN ... SET NOT NULL` pada down-migration akan gagal jika sudah ada baris manual dengan `import_row_id NULL` — ini diterima sebagai batasan wajar rollback (konsisten dengan pola migration lain di proyek ini yang tidak menjamin down-migration aman setelah data produksi berubah).

## 4. Backend

Domain baru `internal/recipients` (bukan perluasan `internal/distribution`, karena scope-nya lintas jadwal, bukan workflow satu alokasi).

### 4.1 Model (`internal/recipients/models.go`)

```go
type Recipient struct {
    AllocationID         string
    DistributionNumber   int
    AllocationStatus     string // candidate|ready|needs_review|distributed|replaced|cancelled
    DistributionStatus   *string // draft|completed|cancelled, nullable (distribution_records mungkin belum ada)
    FullName             string
    NIK                  *string
    SectorIdentifierType *string // farmer_card|kusuka
    SectorIdentifier     *string
    Address, Village, District, PhoneNumber string
    ProgramID, ProgramName, ProgramType string
    RegencyID, RegencyName, RegencyDocumentCode string
    ScheduleID, ScheduleName string
    CreatedAt, UpdatedAt time.Time
}

type Filter struct {
    Page, PageSize int
    Search string // cocok ke full_name ILIKE, nik, normalized sector identifier
    RegencyID, ProgramID, ProgramType string
    AllocationStatus, DistributionStatus string
}

type Page struct {
    Items    []Recipient `json:"items"`
    Page     int         `json:"page"`
    PageSize int         `json:"page_size"`
    Total    int64       `json:"total"`
}

type Stats struct {
    Total int64
    ByAllocationStatus map[string]int64
}

type CreateInput struct {
    ScheduleID       string `json:"schedule_id"`
    FullName         string `json:"full_name"`
    NIK              string `json:"nik"`
    SectorIdentifier string `json:"sector_identifier"`
    Address          string `json:"address"`
    Village          string `json:"village"`
    District         string `json:"district"`
    PhoneNumber      string `json:"phone_number"`
}

type UpdateInput struct {
    FullName         string `json:"full_name"`
    NIK              string `json:"nik"`
    SectorIdentifier string `json:"sector_identifier"`
    Address          string `json:"address"`
    Village          string `json:"village"`
    District         string `json:"district"`
    PhoneNumber      string `json:"phone_number"`
}
```

### 4.2 Repository (`internal/recipients/repository.go`)

- `List(ctx, filter, scope) (Page, error)` — join `package_allocations pa JOIN candidate_nominations cn ON cn.id=pa.nomination_id JOIN people p ON p.id=COALESCE(pa.actual_recipient_person_id, pa.intended_person_id) JOIN program_schedules ps ON ps.id=pa.schedule_id JOIN programs prog ON prog.id=ps.program_id JOIN regencies r ON r.id=ps.regency_id LEFT JOIN distribution_records dr ON dr.allocation_id=pa.id LEFT JOIN person_sector_identifiers psi ON psi.person_id=p.id`. Filter kombinatif via `WHERE` fragment yang dipakai ulang di count query dan select query (pola persis `administration.Repository.ListUsers`). Default `pa.status != 'cancelled'` kecuali `filter.AllocationStatus == "cancelled"` eksplisit diminta. Regency scope: `AND ($N OR ps.regency_id::text = ANY($N+1))` — pola yang sama dengan `distribution.Repository.Search`.
- `Stats(ctx, scope) (Stats, error)` — `SELECT pa.status, count(*) FROM package_allocations pa JOIN program_schedules ps ON ... WHERE (scope) GROUP BY pa.status`.
- `Create(ctx, actor, input, meta, scope) (Recipient, error)` — transaksi: validasi `scope.Allows(schedule.regency_id)`, INSERT `people`, INSERT `person_sector_identifiers` (jika diisi), INSERT `candidate_nominations` (`import_row_id = NULL`, `status='ready'`), INSERT `package_allocations` (`distribution_number` = `MAX(distribution_number)+1` per schedule, `status='ready'`), audit `recipient.created`.
- `Update(ctx, actor, allocationID, input, meta, scope) (Recipient, error)` — transaksi mirip `distribution.SaveDraft` tapi hanya field identitas, scope-checked via join ke `program_schedules`, audit `recipient.updated`.
- `Cancel(ctx, actor, allocationID, meta, scope) error` — `UPDATE package_allocations SET status='cancelled' WHERE id=$1 AND status != 'cancelled' AND (scope)`, audit `recipient.cancelled`. Tidak menyentuh `distribution_records`.
- `Restore(ctx, actor, allocationID, meta, scope) error` — kembalikan ke `'ready'` (bukan status sebelumnya yang lebih spesifik — disederhanakan, admin bisa proses ulang lewat Pendistribusian jika perlu), audit `recipient.restored`.

### 4.3 Service (`internal/recipients/service.go`)

Validasi input (nama wajib, NIK 16 digit jika diisi, jenis identifier sesuai `program_type` jadwal terpilih), delegasi ke repository. Error sentinel: `ErrNotFound`, `ErrInvalidInput`, `ErrScheduleNotFound` (kalau `ScheduleID` tidak ada/di luar scope).

### 4.4 API (`internal/api/recipients_routes.go`)

Registrasi di `routeProtected`:
```go
case path == "recipients": h.handleRecipients(w, r, rc)
case path == "recipients/stats": h.handleRecipientStats(w, r, rc)
case strings.HasPrefix(path, "recipients/"): h.handleRecipient(w, r, rc, strings.TrimPrefix(path, "recipients/"))
```

- `GET /api/v1/recipients` — `recipients.view` — query: `page, page_size, search, regency_id, program_id, program_type, allocation_status, distribution_status`.
- `GET /api/v1/recipients/stats` — `recipients.view`.
- `POST /api/v1/recipients` — `recipients.manage`.
- `GET /api/v1/recipients/{id}` — `recipients.view` (untuk prefill dialog edit; opsional — bisa dilewati kalau frontend cukup pakai data baris tabel yang sudah lengkap).
- `PATCH /api/v1/recipients/{id}` — `recipients.manage`.
- `POST /api/v1/recipients/{id}/cancel` — `recipients.manage`.
- `POST /api/v1/recipients/{id}/restore` — `recipients.manage`.

## 5. Frontend

Rombak total `frontend/src/features/dashboard/DashboardPage.tsx` (placeholder dihapus).

### 5.1 Struktur halaman

1. `PageHeader` — "Data Penerima" + tombol "Tambah penerima" (jika `recipients.manage`).
2. Baris kartu statistik: Total, Sudah distribusi, Siap/menunggu, Perlu ditinjau, Dibatalkan (dari `GET /recipients/stats`).
3. Toolbar filter (state di URL search params, pola sama seperti `UsersPage.tsx`): input pencarian (debounce 300ms) + `Select` Kabupaten, Program, Jenis program, Status alokasi, Status distribusi — semua kirim ulang `page=1` saat berubah.
4. `DataTable`: Kabupaten, Program, Jadwal, No. Pembagian, Nama, NIK, No. Kartu/KUSUKA, Desa/Kecamatan, Status alokasi (`StatusBadge`), Status distribusi, Aksi (Edit, Batalkan/Pulihkan tergantung status).
5. Pagination nav — identik dengan `UsersPage.tsx` (label "Sebelumnya"/"Berikutnya", `page * pageSize >= total`).
6. `RecipientDialog` (Tambah/Edit) — pilih Jadwal (Select, hanya saat Tambah; terkunci saat Edit) lalu field identitas, mengikuti pola `SetupDialog`.
7. `AlertDialog` konfirmasi Batalkan, tombol Pulihkan langsung tanpa konfirmasi (reversibel, low-risk).

### 5.2 Kontrak API di frontend

```ts
type Recipient = { allocation_id: string; distribution_number: number; allocation_status: string; distribution_status: string | null; full_name: string; nik: string | null; sector_identifier: string | null; sector_identifier_type: string | null; address: string; village: string; district: string; phone_number: string; program: { id: string; name: string; program_type: string }; regency: { id: string; name: string; document_code: string }; schedule: { id: string; name: string } };
type RecipientPage = { items: Recipient[]; page: number; page_size: number; total: number };
type RecipientStats = { total: number; by_allocation_status: Record<string, number> };
```

## 6. Testing

- Backend: `internal/recipients/service_test.go` (validasi input) + `internal/recipients/repository_integration_test.go` (mengikuti pola `internal/distribution/repository_integration_test.go` — pakai `TEST_DATABASE_URL`, skip jika tidak ada), cakupan: filter kombinasi, regency scope, create dengan `import_row_id NULL`, cancel lalu tidak muncul di list default, restore.
- Frontend: `DashboardPage.test.tsx` — render stats, filter kombinasi memicu query param yang benar, dialog tambah/edit submit, tombol batalkan/pulihkan.
- Verifikasi manual: jalankan `go build ./...`, `go test ./...`, `npm run build`, `npm run test`, `tsc --noEmit` seperti sesi-sesi sebelumnya.

## 7. Risiko & Catatan Terbuka

- Migration mengubah constraint tabel inti (`candidate_nominations`) — perlu dijalankan (`go run ./cmd/migrate up`) di semua environment sebelum backend baru di-deploy.
- `distribution_number` untuk penerima manual dihitung `MAX+1` per jadwal — perlu memastikan tidak bentrok dengan nomor yang direservasi proses import batch berikutnya pada jadwal yang sama (kandidat race condition ringan, ditangani dengan `SELECT ... FOR UPDATE` pada baris terakhir jadwal saat create).
- Restore mengembalikan status ke `'ready'` secara seragam, bukan status asal sebelum dibatalkan (status asal tidak disimpan). Ini keputusan penyederhanaan yang perlu disetujui pengguna.
