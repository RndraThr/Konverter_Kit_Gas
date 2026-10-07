# Distribution POS Responsibility and Revision Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Memindahkan data peralatan ke POS Dokumen, menjadikan POS Mesin tanggal+dokumentasi saja, dan menambahkan revisi terkontrol agar setiap POS dapat memperbaiki domainnya tanpa merusak konsistensi alokasi, Google Drive, snapshot, atau BA.

**Architecture:** `distribution_slots` tetap menjadi aggregate root dan status operasional tetap `open → linked → completed`; revisi mengembalikan slot selesai ke `linked` sambil menandai `needs_recompletion`. Semua perubahan penting dilakukan dalam transaksi repository dengan row lock, permission tahap media diperiksa server-side, dan dokumen BA final yang terdampak diubah menjadi `stale` tanpa menghapus file historisnya.

**Tech Stack:** Go 1.26, PostgreSQL/pgx, Goose SQL migrations, `net/http`, React 19, TypeScript, TanStack Query, Vitest/Testing Library.

**Spec:** `docs/superpowers/specs/2026-10-07-distribution-pos-responsibility-and-revision-design.md`

## Global Constraints

- POS Mesin hanya mengelola tanggal dan dokumentasi `stage='mesin'`.
- POS Dokumen mengelola penerima, peralatan, dan dokumentasi `stage='dokumen'`.
- POS Penyerahan mengelola dokumentasi `stage='penyerahan'` dan penyelesaian.
- Tanggal tidak dapat berubah selama satu media accepted masih ada pada tahap mana pun.
- Folder tanggal dan nomor slot yang sudah ada harus dipakai ulang; folder kosong tidak dihapus otomatis.
- Perubahan slot `completed` harus didahului pembukaan revisi dengan alasan non-kosong.
- NIK tidak diedit bebas; perubahan identitas penerima dilakukan melalui pergantian alokasi yang atomik.
- Dokumen BA lama tidak dihapus dan tetap dapat diunduh sebagai riwayat.
- Batas unggahan tetap 25 MiB untuk gambar dan 500 MiB untuk video.
- Jangan menggabungkan perbaikan error settings DP3/RAKORDA 409/500 ke dalam perubahan ini.

## Review Focus

- Dua petugas membuka revisi bersamaan: hanya satu transaksi yang mengubah `completed` menjadi revisi; permintaan kedua membaca state terbaru secara idempotent atau mendapat conflict yang jelas (Task 3).
- Tanggal diubah ketika media accepted berada di tahap selain mesin: backend tetap menolak `distribution_date_locked` (Task 3).
- Pengguna dengan `documentation.manage` tetapi tanpa permission POS tahap media mencoba upload/delete: backend menjawab 403 sebelum menyimpan atau menghapus file (Task 5).
- Pergantian penerima gagal karena kandidat baru sudah digunakan: alokasi lama dan kaitan slot tidak berubah sama sekali (Task 4).
- Slot revisi diselesaikan ulang setelah BA lama dibuat: snapshot baru terbentuk, BA lama berstatus `stale`, dan finalisasi berikutnya membuat versi aktif baru (Task 6).

---

## File Structure

- `internal/database/migrations/00043_distribution_slot_revisions.sql`: metadata revisi dan status `stale` dokumen BA.
- `internal/database/migrations/00043_distribution_slot_revisions_test.go`: verifikasi constraint, index parsial, dan rollback migrasi.
- `internal/distribution/models.go`: input/output revisi dan data penerima yang dapat diedit.
- `internal/distribution/service.go`: normalisasi/validasi input dan kontrak repository baru.
- `internal/distribution/repository.go`: transaksi create/update/reopen/recipient/media-stage dan invalidasi BA.
- `internal/distribution/*_test.go`: unit tests service dan repository fakes per perilaku.
- `internal/distribution/repository_integration_test.go`: atomicity, row state, snapshot, dan invalidasi BA.
- `internal/api/handler.go`: perluasan `DistributionService`.
- `internal/api/distribution_routes.go`: route recipient/reopen dan permission media berdasarkan stage.
- `internal/api/handler_test.go`: routing, status code, dan authorization matrix.
- `frontend/src/features/distribution/types.ts`: shape slot/revisi/penerima.
- `frontend/src/features/distribution/RevisionDialog.tsx`: dialog alasan pembukaan revisi yang dipakai semua POS.
- `frontend/src/features/distribution/RevisionDialog.test.tsx`: validasi alasan dan payload.
- `frontend/src/features/distribution/DocumentationSlot.tsx`: menerima `canManage` dari section, bukan permission global.
- `frontend/src/features/distribution/SlotMesinCreate.tsx`: create slot dengan tanggal saja.
- `frontend/src/features/distribution/SlotMesinSection.tsx`: tanggal, dokumentasi mesin, dan entry revisi.
- `frontend/src/features/distribution/SlotDokumenSection.tsx`: penerima, peralatan, dokumentasi dokumen, edit/ganti penerima.
- `frontend/src/features/distribution/SlotPenyerahanSection.tsx`: revisi tahap penyerahan dan penyelesaian ulang.
- `frontend/src/features/distribution/SlotCatalogGrid.tsx`: badge `Perlu diselesaikan ulang`.
- `frontend/src/features/berita-acara/{BAIndividualPanel,DP3Panel,DailyRecapPanel,ClosingTitikSerahPanel,ClosingKabupatenPanel}.tsx`: menampilkan dokumen stale sebagai riwayat.
- Tes `.test.tsx` yang berdampingan dengan setiap komponen di atas.

### Task 1: Schema metadata revisi dan dokumen BA stale

**Files:**
- Create: `internal/database/migrations/00043_distribution_slot_revisions.sql`
- Create: `internal/database/migrations/00043_distribution_slot_revisions_test.go`

**Interfaces:**
- Consumes: tabel `distribution_slots`, `bast_individual_documents`, `bast_daily_bundles`, dan `bast_aggregate_documents` dari migrasi sebelumnya.
- Produces: kolom `needs_recompletion`, `reopened_at`, `reopened_by`, `reopened_stage`, `revision_reason`; status dokumen `stale` yang tidak dianggap aktif/final oleh unique index yang sudah ada.

- [ ] **Step 1: Write the failing migration test**

Tambahkan test yang menjalankan seluruh migrasi, membuat slot, mengisi metadata revisi valid, memastikan `reopened_stage='invalid'` ditolak, dan memastikan status `stale` diterima pada ketiga tabel BA.

```go
func TestDistributionRevisionSchema(t *testing.T) {
    pool := migrationTestPool(t)
    // Fixture mengikuti helper schema test yang sudah ada.
    // UPDATE distribution_slots SET needs_recompletion=true,
    // reopened_stage='dokumen', revision_reason='Koreksi nomor seri'.
    // Assert stage di luar mesin/dokumen/penyerahan gagal check constraint.
    // Assert final/active BA dapat diubah menjadi stale dan index aktif tidak memblok versi baru.
}
```

- [ ] **Step 2: Run the migration test and verify it fails**

Run: `go test ./internal/database/migrations -run TestDistributionRevisionSchema -count=1`

Expected: FAIL karena kolom revisi dan status `stale` belum tersedia.

- [ ] **Step 3: Add migration 00043**

Gunakan SQL berikut sebagai kontrak skema:

```sql
ALTER TABLE distribution_slots
  ADD COLUMN needs_recompletion boolean NOT NULL DEFAULT false,
  ADD COLUMN reopened_at timestamptz,
  ADD COLUMN reopened_by uuid REFERENCES users(id) ON DELETE SET NULL,
  ADD COLUMN reopened_stage text,
  ADD COLUMN revision_reason text,
  ADD CONSTRAINT distribution_slots_reopen_stage_check
    CHECK (reopened_stage IS NULL OR reopened_stage IN ('mesin','dokumen','penyerahan')),
  ADD CONSTRAINT distribution_slots_revision_metadata_check CHECK (
    (needs_recompletion=false) OR
    (reopened_at IS NOT NULL AND reopened_stage IS NOT NULL AND btrim(revision_reason) <> '')
  );
```

Ganti check constraint status ketiga tabel BA sehingga menerima status lama ditambah `stale`. Pertahankan partial unique index pada `status='final'` untuk BA perorangan dan `status='active'` untuk bundle/agregat. Bagian Down mengembalikan baris `stale` menjadi `superseded` sebelum mengembalikan constraint lama, lalu menghapus kolom revisi.

- [ ] **Step 4: Run migration tests**

Run: `go test ./internal/database/migrations -run 'TestDistributionRevisionSchema|TestFarmerDocumentationSlots|TestStreamingDocumentationMedia' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal/database/migrations/00043_distribution_slot_revisions.sql internal/database/migrations/00043_distribution_slot_revisions_test.go
git commit -m "feat: add distribution revision schema"
```

### Task 2: Domain contracts, validation, and minimal slot creation

**Files:**
- Modify: `internal/distribution/models.go`
- Modify: `internal/distribution/service.go`
- Modify: `internal/distribution/service_test.go`
- Modify: `internal/distribution/pos_mesin_test.go`
- Modify: `internal/distribution/pos_dokumen_test.go`

**Interfaces:**
- Consumes: revision columns from Task 1.
- Produces:
  - `UpdateRecipientInput{ScheduleID string, SlotNumber int, Address string, Village string, District string, PhoneNumber string, SectorIdentifier string}`
  - `ReplaceRecipientInput{ScheduleID string, SlotNumber int, NIK string, Address string, Village string, District string, PhoneNumber string, SectorIdentifier string}`
  - `ReopenSlotInput{ScheduleID string, SlotNumber int, Stage string, Reason string}`
  - service methods `UpdateRecipient`, `ReplaceRecipient`, `ReopenSlot`, `DocumentationSlotStage`, `MediaStage`.

- [ ] **Step 1: Write failing service tests for input normalization**

Test bahwa create hanya memerlukan tanggal; update/replace recipient menerapkan `BusinessUpper`, digit-only phone/NIK, dan identifier normalization; reopen menolak stage selain tiga nilai resmi dan alasan kosong.

```go
func TestReopenSlotRequiresReasonAndValidStage(t *testing.T) {
    service := NewService(&fakeDistributionRepository{})
    _, err := service.ReopenSlot(context.Background(), actor, ReopenSlotInput{ScheduleID: "schedule-1", SlotNumber: 1, Stage: "dokumen", Reason: "  "}, meta, scope)
    if !errors.Is(err, ErrRevisionReasonRequired) { t.Fatalf("err=%v", err) }
}
```

- [ ] **Step 2: Run focused service tests and verify failure**

Run: `go test ./internal/distribution -run 'Test(CreateSlot|UpdateRecipient|ReplaceRecipient|ReopenSlot)' -count=1`

Expected: FAIL karena kontrak baru belum ada dan create masih memproses field peralatan.

- [ ] **Step 3: Add models and repository interfaces**

Tambahkan error `ErrRevisionReasonRequired`, `ErrRevisionStageInvalid`, `ErrRevisionNotCompleted`, dan `ErrRecipientNotLinked`. Perluas `DistributionSlot` dengan field penerima (`sector_identifier`, `address`, `village`, `district`, `phone_number`) dan metadata revisi:

```go
NeedsRecompletion bool       `json:"needs_recompletion"`
ReopenedAt        *time.Time `json:"reopened_at,omitempty"`
ReopenedBy        *string    `json:"reopened_by,omitempty"`
ReopenedStage     string     `json:"reopened_stage,omitempty"`
RevisionReason    string     `json:"revision_reason,omitempty"`
```

Pindahkan `UpdateEquipment` dari `posMesinRepository` ke `posDokumenRepository`, lalu tambahkan tiga method mutasi baru dan dua lookup stage ke interface repository service.

- [ ] **Step 4: Implement service validation**

`CreateSlot` hanya trim/validasi schedule, slot number, dan tanggal; set seluruh field peralatan input menjadi string kosong sebelum repository dipanggil agar klien lama tidak dapat menyisipkan peralatan melalui endpoint POS Mesin. Gunakan helper tunggal `normalizeRecipientFields` untuk Link/Update/Replace dan validasi reopen berikut:

```go
switch input.Stage {
case "mesin", "dokumen", "penyerahan":
default:
    return DistributionSlot{}, ErrRevisionStageInvalid
}
input.Reason = strings.TrimSpace(input.Reason)
if input.Reason == "" { return DistributionSlot{}, ErrRevisionReasonRequired }
```

- [ ] **Step 5: Run distribution unit tests**

Run: `go test ./internal/distribution -run 'Test(CreateSlot|UpdateRecipient|ReplaceRecipient|ReopenSlot)' -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add internal/distribution/models.go internal/distribution/service.go internal/distribution/service_test.go internal/distribution/pos_mesin_test.go internal/distribution/pos_dokumen_test.go
git commit -m "feat: define editable distribution workflow"
```

### Task 3: Repository create, equipment, date, reopen, and BA invalidation

**Files:**
- Modify: `internal/distribution/repository.go`
- Modify: `internal/distribution/distribution_date_test.go`
- Modify: `internal/distribution/equipment_snapshot_test.go`
- Modify: `internal/distribution/repository_integration_test.go`

**Interfaces:**
- Consumes: models/service contracts from Task 2 and schema from Task 1.
- Produces: transactional `ReopenSlot`; slot reads that expose revision/recipient fields; equipment editable despite machine media; BA invalidation helper executed inside the reopen transaction.

- [ ] **Step 1: Write failing repository tests**

Tambahkan helper file-local `seedRevisionFixture(t, pool, slotStatus) revisionFixture` yang meng-insert regency, program, package/documentation template, schedule, person, allocation, slot, satu documentation slot per stage, dan media accepted sesuai kebutuhan test. `revisionFixture` memuat `ScheduleID`, `SlotID`, `AllocationID`, `PersonID`, dan `DocumentationSlotIDs map[string]string`. Gunakan helper itu pada tests berikut:

```go
func TestUpdateEquipmentAllowsMachineMedia(t *testing.T) {
    pool := distributionIntegrationPool(t)
    f := seedRevisionFixture(t, pool, "linked")
    insertAcceptedMedia(t, pool, f.DocumentationSlotIDs["mesin"])
    _, err := NewRepository(pool).UpdateEquipment(context.Background(), auth.Principal{}, UpdateEquipmentInput{
        ScheduleID: f.ScheduleID, SlotNumber: 1, MachineOptionCode: "M1", MachineSerialNumber: "SN-1",
    }, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
    if err != nil { t.Fatalf("UpdateEquipment: %v", err) }
}

func TestSetDistributionDateRejectsMediaFromAnyStage(t *testing.T) {
    for _, stage := range []string{"mesin", "dokumen", "penyerahan"} {
        t.Run(stage, func(t *testing.T) {
            pool := distributionIntegrationPool(t)
            f := seedRevisionFixture(t, pool, "linked")
            insertAcceptedMedia(t, pool, f.DocumentationSlotIDs[stage])
            _, err := NewRepository(pool).SetDistributionDate(context.Background(), auth.Principal{}, SetDistributionDateInput{
                ScheduleID: f.ScheduleID, SlotNumber: 1, DistributionDate: "2026-10-08",
            }, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
            if !errors.Is(err, ErrDistributionDateLocked) { t.Fatalf("err=%v", err) }
        })
    }
}
```

Untuk `TestReopenCompletedSlotResetsCompletionAndInvalidatesBA`, lengkapi fixture dengan allocation `distributed`, slot `completed`, snapshot equipment, BA individual `final`, bundle `active`, dan aggregate docs `active`. Panggil `ReopenSlot`, lalu query setiap row dan assert slot `linked`, allocation `ready`, completion timestamps NULL, snapshot `{}`, `needs_recompletion=true`, BA terkait `stale`, dan audit `distribution.slot_reopened` tepat satu. Untuk concurrency, jalankan dua goroutine terhadap slot yang sama, kumpulkan dua error, dan assert tepat satu sukses serta satu `ErrRevisionNotCompleted`.

- [ ] **Step 2: Run repository tests and verify failure**

Run: `go test ./internal/distribution -run 'Test(UpdateEquipmentAllowsMachineMedia|SetDistributionDateRejectsMediaFromAnyStage|ReopenCompletedSlot|ConcurrentReopen)' -count=1`

Expected: FAIL pada equipment lock dan method reopen yang belum ada.

- [ ] **Step 3: Simplify slot creation and expand slot reads**

Ubah INSERT create menjadi hanya schedule, slot number, dan date. Perluas `getSlotByID`/search query untuk membaca recipient contact/sector fields serta semua metadata revisi. Jangan ubah resolver folder Drive; `media/gdrive.go` sudah idempotent dan pengujian existing harus tetap hijau.

- [ ] **Step 4: Remove equipment media lock while retaining row lock**

`UpdateEquipment` tetap mengunci baris slot dan scope kabupaten, tetapi hapus query/branch `hasMesinMedia` dan `ErrEquipmentLocked`. Tolak update langsung saat `status='completed'` dengan `ErrAlreadyCompleted`; perubahan baru dapat dilakukan setelah reopen.

- [ ] **Step 5: Implement transactional reopen and invalidation**

Di dalam satu transaksi:

```sql
SELECT ds.id, ds.allocation_id, ds.distribution_date
FROM distribution_slots ds
JOIN program_schedules ps ON ps.id=ds.schedule_id
WHERE ds.schedule_id=$1 AND ds.slot_number=$2 AND (...scope...)
FOR UPDATE OF ds;

UPDATE package_allocations SET status='ready', updated_at=now() WHERE id=$allocation_id;
UPDATE distribution_slots
SET status='linked', verification_snapshot_json='{}'::jsonb,
    distributed_at=NULL, distributed_by=NULL, completed_at=NULL,
    needs_recompletion=true, reopened_at=now(), reopened_by=$actor,
    reopened_stage=$stage, revision_reason=$reason, updated_at=now()
WHERE id=$slot_id AND status='completed';
```

Sebelum status BA individual diubah, ambil ID final yang terkait slot untuk menandai `bast_daily_bundles` yang memuatnya sebagai `stale`; lalu ubah BA individual menjadi `stale`. Tandai seluruh `bast_aggregate_documents` aktif untuk schedule tersebut sebagai `stale` karena DP3/closing kabupaten tidak semuanya dibatasi tanggal. Catat audit dengan stage, reason, allocation, dan tanggal lama.

Jika slot bukan `completed`, kembalikan `ErrRevisionNotCompleted`; row lock memastikan dua reopen bersamaan tidak sama-sama berhasil.

- [ ] **Step 6: Ensure recompletion clears revision flag**

Pada `CompleteSlot`, pertahankan metadata riwayat reopen tetapi set `needs_recompletion=false`, bangun ulang `verification_snapshot_json`, dan set status/alokasi/timestamp selesai seperti sebelumnya.

- [ ] **Step 7: Run repository and Drive regression tests**

Run: `go test ./internal/distribution ./internal/media -count=1`

Expected: PASS, termasuk test reuse folder yang sudah ada.

- [ ] **Step 8: Commit**

```powershell
git add internal/distribution/repository.go internal/distribution/distribution_date_test.go internal/distribution/equipment_snapshot_test.go internal/distribution/repository_integration_test.go
git commit -m "feat: reopen completed distribution slots"
```

### Task 4: Atomic recipient editing and replacement

**Files:**
- Modify: `internal/distribution/repository.go`
- Modify: `internal/distribution/pos_dokumen_test.go`
- Modify: `internal/distribution/repository_integration_test.go`

**Interfaces:**
- Consumes: `UpdateRecipientInput` dan `ReplaceRecipientInput` dari Task 2.
- Produces: repository methods `UpdateRecipient(...) (DistributionSlot, error)` dan `ReplaceRecipient(...) (DistributionSlot, error)`.

- [ ] **Step 1: Write failing atomicity tests**

Tambahkan helper file-local `seedLinkedRecipientFixture(t, pool) recipientFixture` yang membuat slot linked, allocation lama `ready`, kandidat baru `eligible`, dan kandidat konflik yang sudah linked di slot lain. Gunakan pola assertion konkret berikut untuk rollback:

```go
before := readRecipientLinkState(t, pool, fixture.SlotID, fixture.OldAllocationID)
_, err := repo.ReplaceRecipient(ctx, actor, ReplaceRecipientInput{
    ScheduleID: fixture.ScheduleID, SlotNumber: 1, NIK: fixture.ConflictNIK,
}, meta, auth.RegencyScope{Unrestricted: true})
if !errors.Is(err, ErrCandidateNotFound) && !errors.Is(err, ErrPreviouslyReceived) {
    t.Fatalf("err=%v", err)
}
after := readRecipientLinkState(t, pool, fixture.SlotID, fixture.OldAllocationID)
if !reflect.DeepEqual(before, after) { t.Fatalf("rollback mismatch: before=%+v after=%+v", before, after) }
```

Definisikan `recipientLinkState` di file test dengan field primitive untuk slot allocation/person/status dan allocation lama distribution number/status, sehingga `readRecipientLinkState` hanya melakukan SELECT dan tidak menyembunyikan mutasi. Tests sukses mengassert UpdateRecipient mengubah seluruh field kecuali NIK/person ID, ReplaceRecipient memulihkan allocation lama lalu mengaitkan yang baru, dan kedua method mengembalikan `ErrAlreadyCompleted` tanpa perubahan bila slot completed.

Conflict test mengambil snapshot old allocation/slot/person sebelum panggilan, mencoba NIK kandidat yang sudah linked, lalu membandingkan semua row setelah error `ErrCandidateNotFound` atau `ErrPreviouslyReceived`.

- [ ] **Step 2: Run tests and verify failure**

Run: `go test ./internal/distribution -run 'Test(UpdateRecipient|ReplaceRecipient|RecipientMutation)' -count=1`

Expected: FAIL karena method belum diimplementasikan.

- [ ] **Step 3: Implement UpdateRecipient**

Lock slot + allocation + person dalam transaksi, require status `linked`, update hanya address/village/district/phone pada `people` dan sector identifier pada tabel sektor yang sama dengan alur `LinkSlot`, pertahankan NIK/person ID, lalu audit `distribution.recipient_updated`.

- [ ] **Step 4: Implement ReplaceRecipient atomically**

Lock slot dan allocation lama; validasi slot `linked`; pilih kandidat baru memakai syarat yang sama dengan `SearchCandidate`/`LinkSlot`; lock allocation baru; kembalikan allocation lama ke status sebelum link (`eligible`, `distribution_number=NULL`), set allocation baru `ready` dengan nomor slot, ubah `allocation_id` dan `recipient_person_id` slot, update field penerima baru, lalu audit `distribution.recipient_replaced` dengan old/new allocation IDs. Semua operasi berada dalam satu transaksi.

- [ ] **Step 5: Run focused and full distribution tests**

Run: `go test ./internal/distribution -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add internal/distribution/repository.go internal/distribution/pos_dokumen_test.go internal/distribution/repository_integration_test.go
git commit -m "feat: edit and replace distribution recipients"
```

### Task 5: API routes and stage-aware media authorization

**Files:**
- Modify: `internal/api/handler.go`
- Modify: `internal/api/distribution_routes.go`
- Modify: `internal/api/handler_test.go`
- Modify: `internal/api/routes.go`

**Interfaces:**
- Consumes: service methods Tasks 2–4.
- Produces:
  - `PATCH /api/v1/distribution/slots/{number}/recipient?schedule_id=...`
  - `POST /api/v1/distribution/slots/{number}/replace-recipient?schedule_id=...`
  - `POST /api/v1/distribution/slots/{number}/reopen?schedule_id=...` body `{ "stage": "dokumen", "reason": "..." }`
  - media upload/delete authorization derived from stored documentation stage.

- [ ] **Step 1: Write failing route and permission tests**

Tambahkan table-driven tests yang membuktikan:

```go
tests := []struct{ stage, permission string }{
    {"mesin", "distribution.pos_mesin"},
    {"dokumen", "distribution.pos_dokumen"},
    {"penyerahan", "distribution.pos_penyerahan"},
}
```

Untuk setiap stage, permission yang cocok mendapat 201/204 dan dua permission POS lain mendapat 403. Tambahkan kasus pengguna hanya memiliki `documentation.manage` mendapat 403. Test route equipment harus mengizinkan `distribution.pos_dokumen` dan menolak `distribution.pos_mesin` saja.

- [ ] **Step 2: Run API tests and verify failure**

Run: `go test ./internal/api -run 'TestDistribution.*(Recipient|Reopen|Equipment|MediaPermission)' -count=1`

Expected: FAIL karena routes belum ada dan media masih memakai `documentation.manage` global.

- [ ] **Step 3: Extend DistributionService and route dispatch**

Tambahkan method signature yang sama dengan service Task 2. Dispatch `recipient`, `replace-recipient`, dan `reopen` pada `handleDistributionSlot`. Equipment memakai `distribution.pos_dokumen`.

- [ ] **Step 4: Authorize reopen from requested stage**

Decode body terlebih dahulu, map stage melalui helper tertutup:

```go
func distributionStagePermission(stage string) (string, bool) {
    permissions := map[string]string{
        "mesin": "distribution.pos_mesin",
        "dokumen": "distribution.pos_dokumen",
        "penyerahan": "distribution.pos_penyerahan",
    }
    permission, ok := permissions[stage]
    return permission, ok
}
```

Stage invalid menghasilkan 422 melalui service error mapping; stage valid harus lolos `authorize` permission yang sesuai.

- [ ] **Step 5: Resolve stored media stage before mutation**

Untuk upload, panggil `DocumentationSlotStage(ctx, documentationSlotID, scope)` sebelum membuka multipart besar; untuk delete, panggil `MediaStage(ctx, mediaID, scope)` sebelum `DeleteMedia`. Map hasil ke permission POS dengan helper yang sama. `documentation.manage` tidak lagi menjadi satu-satunya gate endpoint distribusi.

- [ ] **Step 6: Map new errors to stable HTTP responses**

`ErrRevisionReasonRequired`/`ErrRevisionStageInvalid` → 422, `ErrRevisionNotCompleted`/`ErrAlreadyCompleted` → 409, not found → 404. Pertahankan struktur error JSON yang dipakai frontend.

- [ ] **Step 7: Run API suite**

Run: `go test ./internal/api -count=1`

Expected: PASS.

- [ ] **Step 8: Commit**

```powershell
git add internal/api/handler.go internal/api/distribution_routes.go internal/api/handler_test.go internal/api/routes.go
git commit -m "feat: enforce distribution POS permissions"
```

### Task 6: BA stale lifecycle and history presentation

**Files:**
- Modify: `internal/bast/models.go`
- Modify: `internal/bast/repository.go`
- Modify: `internal/bast/aggregate_repository.go`
- Modify: `internal/bast/repository_integration_test.go`
- Modify: `internal/bast/aggregate_repository_integration_test.go`
- Modify: `frontend/src/features/berita-acara/BAIndividualPanel.tsx`
- Modify: `frontend/src/features/berita-acara/DP3Panel.tsx`
- Modify: `frontend/src/features/berita-acara/DailyRecapPanel.tsx`
- Modify: `frontend/src/features/berita-acara/ClosingTitikSerahPanel.tsx`
- Modify: `frontend/src/features/berita-acara/ClosingKabupatenPanel.tsx`
- Modify: adjacent `*.test.tsx` files for those panels.

**Interfaces:**
- Consumes: `stale` rows produced when reopening in Task 3.
- Produces: finalization that creates a new active/final version when only stale history exists; UI history list that labels stale documents `Perlu dibuat ulang`.

- [ ] **Step 1: Write failing BA repository tests**

Test bahwa `SaveFinalDocument` membuat revision berikutnya bila revision lama `stale`; `ActivateAggregate` membuat active version berikutnya bila active lama sudah `stale`; dokumen stale masih dapat dibaca melalui endpoint content berdasarkan ID.

- [ ] **Step 2: Run BA tests and verify failure**

Run: `go test ./internal/bast -run 'Test.*Stale.*' -count=1`

Expected: FAIL sampai model/scanner/list semantics mengenali stale.

- [ ] **Step 3: Update BA repositories without deleting storage**

Query current tetap hanya mencari `final`/`active`, sedangkan query history menyertakan seluruh status. Jangan memanggil cleanup storage untuk dokumen yang menjadi stale; `storage_key` lama harus tetap dapat dibuka. Versi berikutnya tetap memakai `max(version/revision)+1` sehingga nomor tidak berulang.

- [ ] **Step 4: Add frontend history tests**

Pada setiap panel agregat, fixture mengandung `{status:'stale', version:1}` tanpa active doc. Assert teks `Perlu dibuat ulang`, tombol unduh riwayat, dan tombol finalisasi tetap tersedia bagi `bast.manage`. Pada BA individual, assert stale tidak dianggap dokumen final terkini.

- [ ] **Step 5: Implement stale presentation**

Gunakan label konsisten `Perlu dibuat ulang`; active doc tetap menjadi CTA utama, sedangkan stale/superseded masuk daftar riwayat. Jangan menyembunyikan tombol unduh stale.

- [ ] **Step 6: Run backend and frontend BA tests**

Run: `go test ./internal/bast -count=1`

Run: `npm.cmd test -- --run src/features/berita-acara` (working directory `frontend`)

Expected: PASS.

- [ ] **Step 7: Commit**

```powershell
git add internal/bast frontend/src/features/berita-acara
git commit -m "feat: preserve stale BA revision history"
```

### Task 7: Frontend revision primitives and stage-owned media

**Files:**
- Modify: `frontend/src/features/distribution/types.ts`
- Create: `frontend/src/features/distribution/RevisionDialog.tsx`
- Create: `frontend/src/features/distribution/RevisionDialog.test.tsx`
- Modify: `frontend/src/features/distribution/DocumentationSlot.tsx`
- Modify: `frontend/src/features/distribution/DocumentationSlot.test.tsx`

**Interfaces:**
- Consumes: reopen API Task 5 and revision fields Task 2.
- Produces: `RevisionDialog({slot, stage, open, onOpenChange, onReopened})`; `DocumentationSlot` prop `canManage: boolean`.

- [ ] **Step 1: Write failing component tests**

Test dialog menolak alasan whitespace, mengirim `{stage,reason}` ke URL reopen yang benar, lalu memanggil `onReopened`. Test `DocumentationSlot` menyembunyikan upload/delete saat `canManage=false` walaupun provider mempunyai `documentation.manage`.

- [ ] **Step 2: Run focused tests and verify failure**

Run: `npm.cmd test -- --run src/features/distribution/RevisionDialog.test.tsx src/features/distribution/DocumentationSlot.test.tsx` (working directory `frontend`)

Expected: FAIL karena dialog/prop belum tersedia.

- [ ] **Step 3: Extend TypeScript types**

Tambahkan recipient fields, `needs_recompletion`, `reopened_at`, `reopened_stage`, `revision_reason`, dan input API yang sama dengan backend. `CreateSlotInput` hanya berisi schedule, optional slot number, dan date.

- [ ] **Step 4: Implement RevisionDialog**

Gunakan komponen dialog/form existing. Tombol submit disabled bila `reason.trim()===''`. Mutation melakukan POST dan menampilkan `ApiError.message` pada `role='alert'`. Copy utama: `Buka revisi`, `Alasan revisi`, `Slot harus diselesaikan ulang setelah perubahan.`

- [ ] **Step 5: Make media ownership explicit**

Hapus `useCan('documentation.manage')` dari `DocumentationSlot`; gunakan boolean `canManage` dari parent untuk upload, drag/drop, dan delete. Parent section bertanggung jawab mengirim permission POS tahapnya dan `false` ketika slot completed.

- [ ] **Step 6: Run focused frontend tests**

Run: `npm.cmd test -- --run src/features/distribution/RevisionDialog.test.tsx src/features/distribution/DocumentationSlot.test.tsx` (working directory `frontend`)

Expected: PASS.

- [ ] **Step 7: Commit**

```powershell
git add frontend/src/features/distribution/types.ts frontend/src/features/distribution/RevisionDialog.tsx frontend/src/features/distribution/RevisionDialog.test.tsx frontend/src/features/distribution/DocumentationSlot.tsx frontend/src/features/distribution/DocumentationSlot.test.tsx
git commit -m "feat: add distribution revision controls"
```

### Task 8: POS Mesin and POS Dokumen frontend workflow

**Files:**
- Modify: `frontend/src/features/distribution/SlotMesinCreate.tsx`
- Modify: `frontend/src/features/distribution/SlotMesinCreate.test.tsx`
- Modify: `frontend/src/features/distribution/SlotMesinSection.tsx`
- Modify: `frontend/src/features/distribution/SlotMesinSection.test.tsx`
- Modify: `frontend/src/features/distribution/SlotDokumenSection.tsx`
- Modify: `frontend/src/features/distribution/SlotDokumenSection.test.tsx`

**Interfaces:**
- Consumes: `RevisionDialog`, stage-owned `DocumentationSlot`, recipient/equipment APIs.
- Produces: user-facing ownership split specified in the design.

- [ ] **Step 1: Write failing POS Mesin tests**

Assert create form hanya menampilkan tanggal dan mengirim payload date+slot; existing slot menampilkan tanggal dan docs mesin tetapi tidak menampilkan label/serial peralatan; completed slot menampilkan `Buka revisi`; locked date menjelaskan `Hapus seluruh media terlebih dahulu`.

- [ ] **Step 2: Write failing POS Dokumen tests**

Assert section menampilkan form peralatan; equipment PATCH berjalan dengan `distribution.pos_dokumen`; linked slot menyediakan `Edit data penerima` dan `Ganti penerima`; completed slot menyembunyikan mutasi sampai reopen sukses; docs dokumen menerima `canManage` yang benar.

- [ ] **Step 3: Run POS tests and verify failure**

Run: `npm.cmd test -- --run src/features/distribution/SlotMesinCreate.test.tsx src/features/distribution/SlotMesinSection.test.tsx src/features/distribution/SlotDokumenSection.test.tsx` (working directory `frontend`)

Expected: FAIL terhadap susunan lama.

- [ ] **Step 4: Simplify POS Mesin**

Hapus state, selectors, scanner, dan validation peralatan dari create/section mesin. Pertahankan date picker, date-lock computation across all docs, media stage mesin, dan revision dialog stage `mesin`.

- [ ] **Step 5: Build POS Dokumen forms**

Pindahkan selectors/options dan barcode scanner mesin+converter ke section Dokumen. Gunakan endpoint equipment yang sama. Tambahkan dialog edit detail penerima (tanpa NIK editable) dan dialog ganti penerima yang memakai candidate search + confirmation; invalidasi local slot via `onChanged(data)` setelah sukses.

- [ ] **Step 6: Run POS tests**

Run: `npm.cmd test -- --run src/features/distribution/SlotMesinCreate.test.tsx src/features/distribution/SlotMesinSection.test.tsx src/features/distribution/SlotDokumenSection.test.tsx` (working directory `frontend`)

Expected: PASS.

- [ ] **Step 7: Commit**

```powershell
git add frontend/src/features/distribution/SlotMesinCreate.tsx frontend/src/features/distribution/SlotMesinCreate.test.tsx frontend/src/features/distribution/SlotMesinSection.tsx frontend/src/features/distribution/SlotMesinSection.test.tsx frontend/src/features/distribution/SlotDokumenSection.tsx frontend/src/features/distribution/SlotDokumenSection.test.tsx
git commit -m "feat: separate machine and document POS duties"
```

### Task 9: POS Penyerahan, catalog revision badge, and integration verification

**Files:**
- Modify: `frontend/src/features/distribution/SlotPenyerahanSection.tsx`
- Modify: `frontend/src/features/distribution/SlotPenyerahanSection.test.tsx`
- Modify: `frontend/src/features/distribution/SlotCatalogGrid.tsx`
- Modify: `frontend/src/features/distribution/SlotCatalogGrid.test.tsx`
- Modify: `frontend/src/features/distribution/DistributionPage.test.tsx`

**Interfaces:**
- Consumes: complete/reopen behavior and `needs_recompletion` from prior tasks.
- Produces: completion/recompletion UX and catalog-level revision visibility.

- [ ] **Step 1: Write failing handover and catalog tests**

Completed slot harus menawarkan `Buka revisi` bagi POS Penyerahan; reopened linked slot harus menampilkan badge `Perlu diselesaikan ulang` dan dapat diselesaikan; catalog entry dengan flag revisi harus berwarna perhatian dan memiliki accessible label yang sama.

- [ ] **Step 2: Run focused tests and verify failure**

Run: `npm.cmd test -- --run src/features/distribution/SlotPenyerahanSection.test.tsx src/features/distribution/SlotCatalogGrid.test.tsx src/features/distribution/DistributionPage.test.tsx` (working directory `frontend`)

Expected: FAIL karena revision state belum dirender.

- [ ] **Step 3: Implement handover revision and recompletion copy**

Pass `canManage={canComplete && slot.status !== 'completed'}` ke docs penyerahan. Pada completed slot render riwayat docs read-only dan RevisionDialog stage penyerahan. Pada linked+`needs_recompletion`, title/copy menjelaskan penyelesaian ulang; complete mutation tetap memakai endpoint existing.

- [ ] **Step 4: Extend catalog response and badge**

Tambahkan `needs_recompletion` pada `SlotCatalogEntry` backend query/model dan TypeScript. Prioritaskan badge revisi di atas badge linked biasa tanpa mengubah completed green state.

- [ ] **Step 5: Run complete automated verification**

Run: `go test ./... -count=1`

Run: `npm.cmd test -- --run` (working directory `frontend`)

Run: `npm.cmd run typecheck` (working directory `frontend`)

Run: `npm.cmd run build` (working directory `frontend`)

Expected: seluruh command exit 0; build boleh mempertahankan warning ukuran chunk yang sudah dikenal tetapi tidak boleh menambah error.

- [ ] **Step 6: Inspect final diff and migration order**

Run: `git status --short`

Run: `git diff --check HEAD~8..HEAD`

Run: `git log --oneline -10`

Expected: hanya file scope plan berubah, tidak ada whitespace error, `00043` adalah migrasi terbaru, dan commit TDD tersusun per task.

- [ ] **Step 7: Commit final integration**

```powershell
git add internal/distribution/models.go internal/distribution/repository.go frontend/src/features/distribution/SlotPenyerahanSection.tsx frontend/src/features/distribution/SlotPenyerahanSection.test.tsx frontend/src/features/distribution/SlotCatalogGrid.tsx frontend/src/features/distribution/SlotCatalogGrid.test.tsx frontend/src/features/distribution/DistributionPage.test.tsx
git commit -m "feat: complete editable distribution workflow"
```

## Deployment and Manual Smoke Test

Deployment hanya dilakukan setelah seluruh automated verification lulus dan pengguna meminta/menyetujui deploy staging.

- [ ] Jalankan migrasi `00043` melalui prosedur deploy existing dan verifikasi health endpoint.
- [ ] Buat slot baru hanya dengan tanggal; pastikan folder tanggal/nomor yang sudah ada dipakai ulang saat upload.
- [ ] Upload media mesin, lalu isi/edit peralatan di POS Dokumen; pastikan tidak ada equipment lock.
- [ ] Kaitkan penerima, edit detailnya, dan uji ganti penerima dengan kandidat lain.
- [ ] Lengkapi dokumentasi lalu selesaikan slot.
- [ ] Buka revisi dari masing-masing role POS pada tiga slot uji terpisah; pastikan permission silang ditolak.
- [ ] Pastikan BA lama bertanda perlu dibuat ulang, selesaikan ulang slot, lalu finalisasi versi BA baru.
- [ ] Pastikan file BA lama dan media lama yang tidak dihapus tetap dapat diunduh.
