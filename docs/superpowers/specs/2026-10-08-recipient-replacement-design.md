# Rancangan Mekanisme Penerima Pengganti dan Penyaringan Kelayakan Kandidat

Versi: 1.0
Tanggal: 2026-10-08
Status: Menunggu review pengguna
Terkait: `2026-09-03-dcp3-distribution-documentation-design.md` §10 dan §11, `2026-10-07-staged-distribution-media-design.md`, `2026-09-12-data-penerima-master-design.md`

## 1. Latar Belakang

Kasus lapangan: calon penerima A tidak dapat hadir, sehingga paket diberikan kepada orang lain (B). Alur hari ini sudah memiliki aksi **Ganti penerima** di POS Dokumen, tetapi:

1. jejak penggantian hanya tersimpan di audit log sehingga tidak dapat ditanyakan kembali;
2. pengganti wajib sudah memiliki alokasi pada jadwal yang sama dan belum menerima, sehingga orang yang belum terdaftar (mis. kerabat) tidak dapat menggantikan;
3. alasan penggantian tidak pernah diminta, padahal perubahan identitas penerima adalah perubahan data sensitif;
4. rancangan `2026-09-03` §10 sudah menetapkan tabel `recipient_replacements`, status alokasi `replaced`, dan penyimpanan alasan — tidak satu pun diimplementasikan.

Spec ini menambahkan mekanisme penerima pengganti yang lengkap tanpa mengubah alur POS yang sudah berjalan.

## 2. Keadaan Saat Ini

`ReplaceRecipient` (`internal/distribution/repository.go`) berjalan dalam satu transaksi:

1. slot harus `linked`; slot `completed` ditolak `ErrAlreadyCompleted`;
2. mencari `package_allocations` pada jadwal yang sama dengan `p.nik = NIK pengganti AND distribution_number IS NULL`;
3. menolak pengganti yang sudah pernah menerima (`ErrPreviouslyReceived`);
4. memperbarui data penerima pengganti (alamat, desa, kecamatan, telepon, identitas sektor);
5. melepas alokasi lama: `distribution_number=NULL, status='candidate', actual_recipient_person_id=NULL`;
6. memasang alokasi baru: `distribution_number=slot_number, status='ready', actual_recipient_person_id=pengganti`;
7. menukar `distribution_slots.allocation_id` dan `recipient_person_id`;
8. mengantre ulang pemindahan media (`queueMediaMovesForSlot`);
9. mencatat audit `distribution.recipient_replaced`.

Status alokasi `replaced` sudah ada pada CHECK constraint sejak migrasi `00004` dan sudah dihitung pada laporan (`internal/reports/repository.go`), tetapi belum pernah dipakai.

## 3. Keputusan

| Topik | Keputusan |
|---|---|
| Cakupan pengganti | Dua-duanya: pengganti yang sudah punya alokasi di jadwal ini memakai alokasi itu; pengganti yang belum punya alokasi dibuatkan data penerima dan alokasi baru |
| Kelengkapan jejak | Alasan wajib + tabel `recipient_replacements`; tanpa alur persetujuan dan tanpa lampiran |
| Nasib alokasi A | Ditandai `replaced` |
| Tampilan riwayat | Data Penerima dan POS Dokumen |
| Kandidat `needs_review` | Disembunyikan dari pencarian dan ditolak backend; peninjauan calon menjadi fitur terpisah |
| Cakupan penyaringan kelayakan | Berlaku untuk kedua alur, Hubungkan maupun Ganti Penerima |

## 4. Prinsip: Slot Selalu Menunjuk Alokasi Penerima Sebenarnya

Seluruh dokumen dan laporan yang ada membaca identitas penerima dari **nominasi alokasi milik slot**, bukan dari `actual_recipient_person_id`:

| Pembaca | Sumber identitas |
|---|---|
| DP3 (`internal/bast/dp3_repository.go`) | `JOIN people p ON p.id = cn.person_id` |
| BA Perorangan (`internal/bast/repository.go`) | `JOIN people person ON person.id = ds.recipient_person_id` |
| Rekap harian (`internal/bast/daily_recap_repository.go`) | `JOIN people p ON p.id = ds.recipient_person_id` |
| Daftar Data Penerima (`internal/recipients/repository.go`) | `COALESCE(pa.actual_recipient_person_id, pa.intended_person_id, cn.person_id)` |
| Laporan (`internal/reports/repository.go`) | `COALESCE(a.actual_recipient_person_id, a.intended_person_id, n.person_id)` |

DP3 secara khusus menyaring `pa.status IN ('ready','distributed')`. Karena itu rancangan ini menetapkan satu aturan agar semua pembaca tetap benar tanpa diubah:

> Setelah penggantian, `distribution_slots.allocation_id` selalu menunjuk alokasi milik penerima sebenarnya. Alokasi A dilepas dari slot dan ditandai `replaced`.

Konsekuensinya pengganti **selalu** berakhir memiliki alokasi sendiri — baik alokasi yang sudah ada, maupun alokasi baru yang dibuatkan. Ini juga sebabnya pengganti yang belum terdaftar harus dibuatkan alokasi, bukan sekadar dicatat pada `actual_recipient_person_id`.

Alternatif "slot tetap memegang alokasi A, penerima sebenarnya hanya di `actual_recipient_person_id`" **ditolak** karena akan membuat DP3 mencetak A (nama calon awal) sebagai penerima, dan menandai alokasi A `replaced` akan mengeluarkan nomor bagi tersebut dari DP3 sama sekali.

## 5. Penyaringan Kelayakan Kandidat

Pencarian kandidat dipakai bersama oleh mode **Hubungkan** dan mode **Ganti Penerima** di POS Dokumen (`candidatePicker(false)` dan `candidatePicker(true)` memanggil endpoint yang sama), sehingga kelayakan kandidat harus diperbaiki sekali untuk kedua alur.

### 5.1 Masalah yang Ditemukan

`SuggestCandidates` (`internal/distribution/repository.go`) hanya menyaring `pa.distribution_number IS NULL` tanpa memeriksa `pa.status`. Akibatnya alokasi berstatus `needs_review`, `cancelled`, dan `replaced` tetap muncul pada daftar saran.

Lebih berbahaya, `LinkSlot` menulis `status='ready'` tanpa syarat:

```sql
UPDATE package_allocations SET distribution_number=$2, status='ready' WHERE id=$1
```

`ReplaceRecipient` melakukan hal yang sama pada alokasi pengganti. Akibatnya memilih kandidat yang sudah dibatalkan atau ditandai perlu ditinjau akan **menghapus sinyal peringatannya secara diam-diam** — pembatalan menjadi batal tanpa jejak, dan penanda `needs_review` dari DCP3 (konflik identitas atau pernah menerima paket) hilang.

### 5.2 Aturan

Kandidat dinyatakan **dapat menerima** hanya bila seluruhnya terpenuhi:

1. berada pada jadwal yang diminta dan di dalam cakupan kabupaten pengguna;
2. `distribution_number IS NULL` — belum dipasangkan ke nomor bagi mana pun;
3. `status IN ('candidate','ready')` — status yang jelas dapat menerima;
4. orangnya belum pernah menerima paket (`distribution_slots.recipient_person_id AND status='completed'`).

Status `needs_review`, `cancelled`, `replaced`, dan `distributed` **tidak dapat menerima**.

### 5.3 Penegakan

- **Pencarian** (`SuggestCandidates`) menambahkan `AND pa.status IN ('candidate','ready')`, sehingga daftar saran hanya berisi kandidat yang dapat menerima.
- **Mutasi** (`LinkSlot` dan `ReplaceRecipient`) memeriksa aturan §5.2 di dalam transaksi sebelum menimpa status. Backend adalah batas keamanan; penyaringan antarmuka bukan pengganti.
- Penulisan status menjadi `ready` hanya sah bila status sebelumnya `candidate` atau `ready`. Status lain tidak pernah ditimpa.
- Galat dibedakan agar petugas tahu sebabnya, bukan menerima `ErrCandidateNotFound` yang menyesatkan:

| Keadaan kandidat | Galat | HTTP | Kode |
|---|---|---|---|
| `needs_review` | `ErrCandidateNeedsReview` | 409 | `candidate_needs_review` |
| `cancelled` | `ErrCandidateNotAvailable` | 409 | `candidate_not_available` |
| `replaced` | `ErrCandidateNotAvailable` | 409 | `candidate_not_available` |
| Alokasi sudah punya nomor bagi | `ErrCandidateAlreadyAssigned` | 409 | `candidate_already_assigned` |
| NIK tidak ada di jadwal ini | `ErrCandidateNotFound` | 404 | `candidate_not_found` |

`ErrCandidateAlreadyAssigned` dan `ErrCandidateNotAvailable` sudah diperkenalkan pada §7 untuk jalur penggantian; §5 memperluas pemakaiannya ke jalur Hubungkan.

### 5.4 Penjelasan saat Pencarian Kosong

Pencarian yang kosong memiliki dua sebab yang sangat berbeda akibatnya bagi petugas:

- NIK belum terdaftar → tawarkan pembuatan data penerima baru;
- NIK terdaftar tetapi belum dapat menerima → tampilkan alasannya dan larang pembuatan data baru.

Tanpa pembedaan ini, petugas akan menyangka orangnya belum terdaftar lalu mencoba membuat data baru, padahal NIK tersebut sudah ada dan akan ditolak `people_nik_uq` (`ErrNIKInUse`). Karena itu ditambahkan endpoint pencarian NIK eksak:

`GET /api/v1/distribution/candidate-lookup?schedule_id=...&nik=...`

Responsnya menyatakan salah satu keadaan: `receivable`, `needs_review`, `not_available` (beserta status alokasinya), `already_assigned` (beserta nomor baginya), `not_registered`, atau `previously_received`. Antarmuka memakai keadaan ini untuk memilih panel yang tepat, bukan menebak dari daftar saran yang kosong.

## 6. Perubahan Database

Migrasi baru `00046`:

```sql
CREATE TABLE recipient_replacements (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    distribution_slot_id uuid NOT NULL REFERENCES distribution_slots(id) ON DELETE CASCADE,
    old_allocation_id uuid NOT NULL REFERENCES package_allocations(id),
    old_person_id uuid NOT NULL REFERENCES people(id),
    new_allocation_id uuid NOT NULL REFERENCES package_allocations(id),
    new_person_id uuid NOT NULL REFERENCES people(id),
    origin text NOT NULL CHECK (origin IN ('existing_allocation', 'new_allocation')),
    reason text NOT NULL CHECK (btrim(reason) <> ''),
    replaced_by uuid REFERENCES users(id),
    replaced_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX recipient_replacements_slot_idx ON recipient_replacements (distribution_slot_id, replaced_at DESC);
CREATE INDEX recipient_replacements_old_person_idx ON recipient_replacements (old_person_id);
CREATE INDEX recipient_replacements_new_person_idx ON recipient_replacements (new_person_id);
```

Satu slot boleh memiliki lebih dari satu baris: penggantian dapat berantai (A→B, lalu B→C). Baris bersifat append-only dan tidak pernah diubah atau dihapus. `new_allocation_id` wajib terisi karena §4 menjamin pengganti selalu memiliki alokasi.

Migrasi Down menghapus tabel saja; tidak menyentuh `package_allocations` maupun berkas Google Drive.

## 7. Alur Penggantian

`ReplaceRecipientInput` bertambah `Reason string` dan `FullName string`.

Seluruh langkah berjalan dalam satu transaksi:

1. Kunci slot dan validasi cakupan kabupaten. Slot `completed` → `ErrAlreadyCompleted` (buka revisi lebih dahulu). Slot bukan `linked` → `ErrRecipientNotLinked`.
2. Validasi `Reason` tidak kosong → galat baru `ErrReplacementReasonRequired`.
3. Tentukan alokasi pengganti berdasarkan keadaan NIK pengganti. Pencarian alokasi **tidak membedakan asal data**: alokasi hasil import DCP3 dan alokasi hasil **Tambah penerima** di Data Penerima diperlakukan sama, karena keduanya berbentuk sama (`package_allocations` pada jadwal ini dengan `distribution_number IS NULL`).

   | Keadaan NIK pengganti | Tindakan | `origin` |
   |---|---|---|
   | Punya alokasi di jadwal ini, `distribution_number IS NULL`, status `candidate`/`ready`/`needs_review` | Pakai alokasi itu | `existing_allocation` |
   | Tidak punya alokasi di jadwal ini (mis. baru terdaftar di jadwal lain, atau sama sekali baru) | Pakai ulang baris `people` bila ada; bila belum ada buat baru. Lalu buat `candidate_nominations` + `package_allocations` dengan `distribution_number=slot_number`, `status='ready'`, `intended_person_id=penerima` | `new_allocation` |
   | Punya alokasi di jadwal ini tetapi `distribution_number` sudah terisi | **Tolak** `ErrCandidateAlreadyAssigned` — pengganti sudah terpasang pada nomor bagi lain | — |
   | Punya alokasi di jadwal ini berstatus `cancelled` atau `replaced` | **Tolak** `ErrCandidateNotAvailable` — arahkan petugas ke Data Penerima untuk memulihkan (`restore`) lebih dahulu | — |

   Pembuatan orang baru memakai validasi yang sama seperti `recipients.Create`: NIK tepat 16 digit, NIK belum dipakai (`ErrNIKInUse`), identitas sektor belum dipakai (`ErrSectorIdentifierInUse`), nama wajib terisi. `candidate_nominations` dibuat tanpa `batch_id` dan `import_row_id`, persis sebagaimana `recipients.Create` melakukannya. Bila NIK sudah ada pada `people`, baris tersebut dipakai ulang dan tidak pernah dibuat ganda — `people_nik_uq` menjaminnya.

   Bila ditemukan lebih dari satu alokasi yang layak untuk NIK yang sama pada jadwal ini (mungkin terjadi karena import DCP3 ganda), dipilih yang paling lama secara deterministik (`ORDER BY created_at, id`) dan kejadian itu dicatat pada audit sebagai sinyal kualitas data.

   Dua penolakan di atas penting: tanpa keduanya, alur "buat alokasi baru" dapat memberi satu orang dua alokasi pada jadwal yang sama, karena `UNIQUE (nomination_id)` hanya membatasi per nominasi, bukan per orang.
4. Tolak pengganti yang sudah pernah menerima: `ErrPreviouslyReceived`.
5. Perbarui data penerima pengganti (alamat, desa, kecamatan, telepon, identitas sektor) dengan normalisasi business-uppercase yang berlaku.
6. Lepas alokasi A: `distribution_number=NULL, status='replaced', actual_recipient_person_id=NULL`.
7. Pasang alokasi pengganti: `distribution_number=slot_number, status='ready', actual_recipient_person_id=pengganti`.
8. Tukar `distribution_slots.allocation_id` dan `recipient_person_id`.
9. Sisipkan baris `recipient_replacements` berisi alokasi/orang lama dan baru, `origin`, `reason`, dan `replaced_by` dari aktor.
10. `queueMediaMovesForSlot` agar berkas media dipindahkan dan dinamai ulang ke nama pengganti.
11. Audit `distribution.recipient_replaced` dengan metadata alokasi/orang lama dan baru, `origin`, serta `reason`.

Poin 6 dan 10 memastikan dua integrasi yang sudah ada tetap benar: status alokasi A terlihat sebagai `replaced` pada filter Data Penerima dan laporan, dan nama berkas di Google Drive mengikuti nama pengganti karena nama final media memuat nama penerima slot.

### 7.1 Penerima yang Berasal dari Tambah Penerima

Data penerima dapat masuk melalui dua jalur, dan keduanya sudah menghasilkan bentuk alokasi yang sama sehingga **penggantian tidak perlu membedakannya**:

| Jalur | Yang dibuat |
|---|---|
| Import DCP3 (`internal/dcp3/repository.go`) | `people` (via `matchOrCreatePerson`) → `candidate_nominations` (dengan `batch_id`/`import_row_id`) → `package_allocations` dengan nomor bagi bila mapping menyediakannya |
| **Tambah penerima** (`internal/recipients/repository.go` `Create`) | `people` → `person_sector_identifiers` → `candidate_nominations` tanpa batch → `package_allocations` `status='ready'` dan `distribution_number` NULL |

Karena penerima hasil Tambah Penerima sudah memiliki `package_allocations` pada jadwalnya dengan `distribution_number IS NULL`, mereka **sudah dapat dipakai sebagai pengganti pada alur hari ini**, lewat cabang `existing_allocation`. Yang berubah hanyalah penamaannya: cabang ini sebelumnya disebut "kandidat DCP3", padahal kueri `ReplaceRecipient` memang tidak pernah memeriksa asal data.

Konsekuensi yang perlu diperhatikan:

- Penerima hasil Tambah Penerima yang alokasinya sudah terpasang pada nomor bagi lain ditolak `ErrCandidateAlreadyAssigned`, bukan dibuatkan alokasi kedua.
- Tambah Penerima tidak dapat membuat orang yang NIK-nya sudah terdaftar (`people_nik_uq` → `ErrNIKInUse`), sehingga tidak ada risiko orang ganda dari jalur ini.
- Alokasi penerima hasil Tambah Penerima tidak memiliki `batch_id`, sehingga `origin` pada riwayat penggantian **tidak dapat disimpulkan dari keberadaan batch**. Karena itu `origin` hanya mencatat `existing_allocation` atau `new_allocation` (apakah alokasi pengganti sudah ada atau baru dibuat saat penggantian), bukan asal daftarnya.
- Bila penerima hasil Tambah Penerima ditolak sebagai pengganti karena statusnya `cancelled`, pemulihannya dilakukan lebih dahulu lewat aksi **Pulihkan** di Data Penerima, bukan dibuatkan alokasi baru.

## 8. API

| Endpoint | Perubahan |
|---|---|
| `POST /api/v1/distribution/slots/{number}/replace-recipient?schedule_id=...` | Body bertambah `reason` (wajib) dan `full_name` (wajib hanya bila penerima belum terdaftar) |
| `GET /api/v1/distribution/slots/{number}/replacements?schedule_id=...` | Baru — riwayat penggantian satu slot untuk POS Dokumen |
| `GET /api/v1/distribution/candidate-suggestions?schedule_id=...&nik_prefix=...` | Hanya mengembalikan kandidat yang dapat menerima (§5.2) |
| `GET /api/v1/distribution/candidate-lookup?schedule_id=...&nik=...` | Baru — keadaan NIK eksak agar antarmuka dapat menjelaskan sebab pencarian kosong |
| `GET /api/v1/recipients` | Respons item bertambah `replaced_by` dan `replaces` |

Permission tidak berubah: `distribution.pos_dokumen`. Galat baru beserta pemetaannya:

| Galat | HTTP | Kode |
|---|---|---|
| `ErrReplacementReasonRequired`, `ErrReplacementNameRequired` | 422 | `validation_failed` (field `replacement`), mengikuti pola alasan revisi yang sudah ada |
| `ErrCandidateNeedsReview` | 409 | `candidate_needs_review` |
| `ErrCandidateNotAvailable` | 409 | `candidate_not_available` |
| `ErrCandidateAlreadyAssigned` | 409 | `candidate_already_assigned` |
| `ErrReplacementSameRecipient` | 409 | `replacement_same_recipient` |

Catatan urutan pemeriksaan pada saat implementasi: status alokasi diperiksa **sebelum** nomor bagi. Alokasi yang dibatalkan di Data Penerima tetapi masih menyimpan nomor bagi lama dijelaskan sebagai "tidak dapat menerima", bukan "sudah terpasang di nomor lain", karena pesan pertama yang lebih tepat untuk petugas. Penggantian dengan NIK penerima saat ini juga diperiksa lebih dahulu agar tidak dilaporkan sebagai `candidate_already_assigned`.

## 9. Antarmuka

### 9.1 POS Dokumen

- Mode ganti penerima menampilkan kolom **Alasan penggantian** yang wajib diisi sebelum tombol **Pasang penerima pengganti** aktif.
- Bila pencarian NIK tidak menemukan penerima terdaftar, tampil panel **Pengganti belum terdaftar** dengan field nama lengkap dan alamat, disertai penjelasan bahwa data penerima dan alokasi baru akan dibuat. Petugas tidak dibiarkan menemui galat buntu.
- Bila pengganti ditemukan tetapi alokasinya sudah terpasang pada nomor bagi lain, panel menampilkan pesan backend `candidate_already_assigned` beserta nomor bagi yang bersangkutan, bukan sekadar galat umum.
- Bila pengganti ditemukan tetapi alokasinya `cancelled` atau `replaced`, panel menampilkan pesan `candidate_not_available` dan mengarahkan petugas memulihkan data tersebut lewat Data Penerima.
- Setelah penggantian, kartu slot menampilkan penanda **Penerima digantikan dari {A}** beserta alasan dan waktu, dan menampilkan tautan ke riwayat lengkap bila penggantian lebih dari satu kali.

### 9.2 Data Penerima

- Baris dengan alokasi `replaced` menampilkan badge **Digantikan**.
- Detail penerima A menampilkan **Digantikan oleh {B} pada {tanggal}** serta alasan.
- Detail penerima B menampilkan **Menggantikan {A}** pada nomor bagi yang sama.
- Filter `allocation_status=replaced` sudah didukung backend; hanya perlu opsi pada antarmuka.

## 10. Dampak terhadap Fitur yang Sudah Berjalan

| Fitur | Dampak |
|---|---|
| DP3 | Tetap benar tanpa perubahan query: alokasi slot adalah milik penerima sebenarnya |
| BA Perorangan dan rekap harian | Tetap benar: membaca `distribution_slots.recipient_person_id` |
| Laporan | Bucket `replaced` sudah dihitung; A tidak lagi muncul sebagai penerima nomor bagi tersebut |
| Pemindahan dan penamaan media | Nama berkas final memuat nama penerima, sehingga penggantian mengantre ulang dan mengganti nama berkas otomatis |
| Pencegahan penerimaan ganda | Berbasis orang (`distribution_slots.recipient_person_id`), sehingga pengganti terlindungi dan A tetap bebas menerima pada jadwal lain |
| Revisi slot | Tidak berubah: slot `completed` tetap wajib dibuka revisi sebelum diganti |
| Data Penerima | Baris A tetap tampil dengan status `replaced` sebagai riwayat, bukan dihapus |
| Penerima hasil Tambah Penerima | Tidak berubah bentuknya: alokasinya sudah `ready` tanpa nomor bagi, sehingga langsung dapat dipakai sebagai pengganti |
| Tambah Penerima (pembuatan orang) | Tidak berubah: `people_nik_uq` tetap mencegah orang ganda, dan penggantian memakai ulang baris `people` yang ada |
| Alur Hubungkan penerima | Ikut berubah: kandidat `needs_review`/`cancelled`/`replaced` tidak lagi tampil, dan status kandidat tidak lagi tertimpa `ready` tanpa syarat |
| Data DCP3 | Tidak berubah: penanda `needs_review` dari import kini bertahan sampai ditinjau, tidak lagi hilang saat dipasang di POS Dokumen |

## 11. Kriteria Penerimaan

1. Pengganti yang sudah memiliki alokasi pada jadwal yang sama memakai alokasi itu, dan alokasi A menjadi `replaced`.
2. Pengganti yang belum memiliki alokasi pada jadwal tersebut dapat dipasang, memperoleh data penerima dan alokasi baru, dan alokasi A menjadi `replaced`.
3. Penerima hasil **Tambah penerima** dapat dipasang sebagai pengganti tanpa perlakuan khusus, karena alokasinya sudah berbentuk sama.
4. Pengganti yang alokasinya sudah terpasang pada nomor bagi lain ditolak `ErrCandidateAlreadyAssigned`; pengganti dengan alokasi `cancelled`/`replaced` ditolak `ErrCandidateNotAvailable`.
5. Satu orang tidak pernah memiliki dua alokasi terpakai pada satu jadwal akibat penggantian.
6. NIK pengganti yang sudah terdaftar tidak membuat baris `people` kedua.
7. Penggantian tanpa alasan ditolak backend dan tidak dapat dikirim dari antarmuka.
8. Riwayat penggantian dapat dibaca kembali per slot maupun per orang.
9. Penggantian berantai (A→B→C) menyimpan tiga baris riwayat dan hanya alokasi terakhir yang terpasang pada slot.
10. DP3, BA Perorangan, rekap harian, dan laporan menampilkan pengganti sebagai penerima nomor bagi tersebut.
11. Berkas media slot dipindahkan dan dinamai ulang ke nama pengganti.
12. Pengganti yang sudah pernah menerima ditolak; A tetap dapat menerima pada jadwal lain.
13. Slot `completed` tidak dapat diganti sebelum dibuka revisi.
14. Pencarian kandidat tidak pernah menampilkan alokasi berstatus `needs_review`, `cancelled`, `replaced`, atau `distributed`.
15. `LinkSlot` maupun `ReplaceRecipient` menolak kandidat yang tidak memenuhi §5.2 dengan galat yang membedakan sebabnya, dan tidak pernah menimpa status selain `candidate`/`ready`.
16. Memasang kandidat yang sebelumnya `needs_review` atau `cancelled` tidak lagi mengubah statusnya menjadi `ready` tanpa jejak.
17. Pencarian NIK eksak membedakan "belum terdaftar" dari "terdaftar tetapi belum dapat menerima", sehingga petugas tidak diarahkan membuat data penerima ganda.
18. Seluruh tes Go, tes frontend, typecheck, dan build lulus.

## 12. Di Luar Cakupan

- Alur persetujuan pusat dan status menunggu persetujuan (`2026-09-03` §11).
- Unggah lampiran pendukung (surat kuasa, foto KTP) ke Google Drive.
- Pemeriksaan kelayakan pengganti dan pencocokan DCP3 untuk pengganti yang belum terdaftar.
- Menampilkan kolom "calon awal" pada DP3/BAST (butuh perubahan query dokumen).
- Pembatalan penggantian (undo).
- Perubahan kuota jadwal akibat penggantian.
- Pembersihan alokasi ganda yang sudah ada akibat import DCP3 berulang; spec ini hanya memilih satu secara deterministik dan mencatatnya pada audit.
- Antarmuka peninjauan calon berstatus `needs_review` beserta perubahan statusnya menjadi `ready`; spec ini hanya menolak dan menjelaskan, tidak menyediakan jalur peninjauan.
- Pemeriksaan kelayakan otomatis penuh (`eligibility_checks`) dan override pusat `2026-09-03` §11.
