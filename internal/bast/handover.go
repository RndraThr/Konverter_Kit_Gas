package bast

import (
	"context"
	"errors"
	"sort"
	"strings"
)

// Titik serah BA Serah Terima (Closing Titik Serah). Petugas POS Dokumen
// mencatat titik serah setiap nomor bagi saat distribusi; daftar lokasinya
// dipakai bersama satu kabupaten dan tumbuh dari lapangan. Admin merapikan
// varian nama (ubah nama, gabungkan) dan mengoreksi lokasi satu penerima.

var (
	ErrHandoverLocationInvalid = errors.New("handover location is invalid")
	// ErrHandoverLocationDuplicate: nama baru sama dengan lokasi lain di
	// kabupaten itu (gunakan gabungkan).
	ErrHandoverLocationDuplicate = errors.New("handover location name already used")
	// ErrHandoverLocationInUse: lokasi masih dipakai penerima.
	ErrHandoverLocationInUse = errors.New("handover location still has recipients")
	// ErrHandoverUnassigned: masih ada penerima selesai distribusi yang
	// belum punya titik serah.
	ErrHandoverUnassigned = errors.New("recipients without handover location")
	// ErrHandoverClosingIncomplete: Closing Kabupaten membutuhkan BA Serah
	// Terima final untuk setiap titik serah yang punya penerima.
	ErrHandoverClosingIncomplete = errors.New("closing titik serah of every handover location must be final")
)

const (
	closingDocumentPrefix = AggregateDocumentClosingTitikSerah + ":"
	closingWetKeyPrefix   = "closing-titik-serah@"
)

// Status BA Serah Terima satu titik serah.
const (
	HandoverStatusFinal = "final" // dokumen final aktif
	HandoverStatusStale = "stale" // data berubah setelah final
	HandoverStatusNone  = "none"  // belum dibuat
	HandoverStatusEmpty = "empty" // belum ada penerima selesai distribusi
)

// HandoverLocation adalah satu titik serah kabupaten.
type HandoverLocation struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// handoverRecipient adalah satu nomor bagi berpenerima pada jadwal.
type handoverRecipient struct {
	SlotID     string
	SlotNumber int
	Name       string
	District   string
	Village    string
	LocationID string
	Completed  bool
	LocalDate  string
	Snapshot   []byte
}

// HandoverRecipientView adalah penerima selesai distribusi tanpa titik serah
// (perlu dikoreksi admin).
type HandoverRecipientView struct {
	SlotID     string `json:"slot_id"`
	SlotNumber int    `json:"slot_number"`
	Name       string `json:"name"`
	District   string `json:"district"`
	Village    string `json:"village"`
	LocalDate  string `json:"local_date,omitempty"`
}

// HandoverLocationSummary adalah rekap satu titik serah pada satu jadwal.
type HandoverLocationSummary struct {
	HandoverLocation
	// Recipients: penerima selesai distribusi; Assigned: seluruh penerima
	// jadwal ini yang memilih lokasi ini (termasuk yang belum selesai).
	Recipients int      `json:"recipients"`
	Assigned   int      `json:"assigned"`
	Dates      []string `json:"dates"`
	Status     string   `json:"status"`
	// DocumentDate: tanggal BA final aktif terbaru.
	DocumentDate string `json:"document_date,omitempty"`
}

// HandoverView adalah rekap titik serah satu jadwal.
type HandoverView struct {
	ScheduleID string                    `json:"schedule_id"`
	Locations  []HandoverLocationSummary `json:"locations"`
	// Unassigned: penerima selesai distribusi tanpa titik serah.
	Unassigned []HandoverRecipientView `json:"unassigned"`
}

// handoverDocumentStatus adalah status BA Serah Terima satu lokasi.
type handoverDocumentStatus struct {
	Active     bool
	ActiveDate string
	Stale      bool
}

// locationOf mengembalikan titik serah penerima bila lokasinya ada di daftar.
func locationOf(locations []HandoverLocation, recipient handoverRecipient) string {
	for _, location := range locations {
		if location.ID == recipient.LocationID {
			return location.ID
		}
	}
	return ""
}

// buildHandoverView menghitung rekap per lokasi. Lokasi kabupaten tanpa
// penerima pada jadwal ini tetap tampil (status empty) agar bisa dirapikan.
func buildHandoverView(scheduleID string, locations []HandoverLocation, recipients []handoverRecipient, statuses map[string]handoverDocumentStatus) HandoverView {
	view := HandoverView{ScheduleID: scheduleID, Locations: []HandoverLocationSummary{}, Unassigned: []HandoverRecipientView{}}
	index := map[string]int{}
	dates := map[string]map[string]bool{}
	for i, location := range locations {
		index[location.ID] = i
		dates[location.ID] = map[string]bool{}
		view.Locations = append(view.Locations, HandoverLocationSummary{HandoverLocation: location, Dates: []string{}, Status: HandoverStatusEmpty})
	}
	for _, recipient := range recipients {
		locationID := locationOf(locations, recipient)
		if locationID == "" {
			if recipient.Completed {
				view.Unassigned = append(view.Unassigned, HandoverRecipientView{
					SlotID: recipient.SlotID, SlotNumber: recipient.SlotNumber, Name: recipient.Name,
					District: recipient.District, Village: recipient.Village, LocalDate: recipient.LocalDate,
				})
			}
			continue
		}
		summary := &view.Locations[index[locationID]]
		summary.Assigned++
		if recipient.Completed {
			summary.Recipients++
			if recipient.LocalDate != "" {
				dates[locationID][recipient.LocalDate] = true
			}
		}
	}
	for i := range view.Locations {
		summary := &view.Locations[i]
		for date := range dates[summary.ID] {
			summary.Dates = append(summary.Dates, date)
		}
		sort.Strings(summary.Dates)
		status := statuses[summary.ID]
		switch {
		case status.Active:
			summary.Status, summary.DocumentDate = HandoverStatusFinal, status.ActiveDate
		case status.Stale:
			summary.Status = HandoverStatusStale
		case summary.Recipients > 0:
			summary.Status = HandoverStatusNone
		}
	}
	// Lokasi berpenerima dulu, lalu menurut nama.
	sort.SliceStable(view.Locations, func(i, j int) bool {
		a, b := view.Locations[i], view.Locations[j]
		if (a.Assigned > 0) != (b.Assigned > 0) {
			return a.Assigned > 0
		}
		return strings.ToUpper(a.Name) < strings.ToUpper(b.Name)
	})
	return view
}

func closingDocumentType(locationID string) string {
	return closingDocumentPrefix + locationID
}

func closingWetKey(locationID string) string {
	return closingWetKeyPrefix + locationID
}

// wetBaseKey mengembalikan kunci BA tanda tangan basah tanpa lokasi titik
// serah (aturan tanda tangan berlaku untuk seluruh lokasi).
func wetBaseKey(documentKey string) string {
	if base, _, found := strings.Cut(documentKey, "@"); found {
		return base
	}
	return documentKey
}

type handoverLocationKey struct{}

// WithHandoverLocation membawa titik serah yang dipilih ke Preview BA
// Serah Terima (dipakai juga oleh cetak/unggah tanda tangan basah).
func WithHandoverLocation(ctx context.Context, locationID string) context.Context {
	return context.WithValue(ctx, handoverLocationKey{}, strings.TrimSpace(locationID))
}

func handoverLocationFrom(ctx context.Context) string {
	value, _ := ctx.Value(handoverLocationKey{}).(string)
	return value
}

// pickHandoverLocation memilih lokasi dengan ID; ID kosong hanya sah bila
// hanya ada satu titik serah.
func pickHandoverLocation(locations []HandoverLocation, locationID string) (HandoverLocation, error) {
	locationID = strings.TrimSpace(locationID)
	if locationID == "" {
		if len(locations) == 1 {
			return locations[0], nil
		}
		return HandoverLocation{}, ErrHandoverLocationRequired
	}
	for _, location := range locations {
		if location.ID == locationID {
			return location, nil
		}
	}
	return HandoverLocation{}, ErrHandoverLocationRequired
}

// closingRawRowsFor mengambil distribusi selesai milik satu titik serah.
func closingRawRowsFor(recipients []handoverRecipient, locationID string) []closingRawRow {
	raw := []closingRawRow{}
	for _, recipient := range recipients {
		if recipient.Completed && recipient.LocationID == locationID {
			raw = append(raw, closingRawRow{LocalDate: recipient.LocalDate, VerificationSnapshot: recipient.Snapshot})
		}
	}
	sort.SliceStable(raw, func(i, j int) bool { return raw[i].LocalDate < raw[j].LocalDate })
	return raw
}

// closingKabupatenRowsFromHandover menyusun baris Closing Kabupaten dari BA
// Serah Terima final tiap titik serah (satu baris per lokasi dan varian
// mesin). Tanpa strict, lokasi yang belum final memakai data distribusi
// berjalan agar pratinjau tetap terisi. Dengan strict, setiap penerima
// selesai harus punya titik serah dan setiap lokasi berpenerima harus punya
// BA Serah Terima final yang memuat seluruh penerimanya.
func closingKabupatenRowsFromHandover(locations []HandoverLocation, recipients []handoverRecipient, snapshots map[string]ClosingTitikSerahSnapshot, strict bool) ([]ClosingKabupatenRow, error) {
	completed := map[string]int{}
	for _, recipient := range recipients {
		if !recipient.Completed {
			continue
		}
		locationID := locationOf(locations, recipient)
		if locationID == "" && strict {
			return nil, ErrHandoverUnassigned
		}
		completed[locationID]++
	}
	ordered := append([]HandoverLocation(nil), locations...)
	sort.SliceStable(ordered, func(i, j int) bool { return strings.ToUpper(ordered[i].Name) < strings.ToUpper(ordered[j].Name) })
	rows := []ClosingKabupatenRow{}
	for _, location := range ordered {
		count := completed[location.ID]
		if count == 0 {
			continue
		}
		snapshot, final := snapshots[location.ID]
		final = final && snapshot.GrandTotal == count
		if !final {
			if strict {
				return nil, ErrHandoverClosingIncomplete
			}
			for _, row := range groupClosingRows(closingRawRowsFor(recipients, location.ID)) {
				rows = appendKabupatenRow(rows, location.Name, row.MachineBrand, row.MachineType, row.Count, false)
			}
			continue
		}
		name := snapshot.HandoverLocation
		if strings.TrimSpace(name) == "" {
			name = location.Name
		}
		for _, row := range snapshot.Rows {
			rows = appendKabupatenRow(rows, name, row.MachineBrand, row.MachineType, row.Count, true)
		}
	}
	return rows, nil
}

// appendKabupatenRow menjumlahkan baris (lokasi, merek, tipe) yang sama dan
// menjaga urutan kemunculan pertama.
func appendKabupatenRow(rows []ClosingKabupatenRow, location, brand, typ string, count int, final bool) []ClosingKabupatenRow {
	for i := range rows {
		if rows[i].Location == location && rows[i].MachineBrand == brand && rows[i].MachineType == typ {
			rows[i].Count += count
			return rows
		}
	}
	return append(rows, ClosingKabupatenRow{Location: location, MachineBrand: brand, MachineType: typ, Count: count, Final: final})
}
