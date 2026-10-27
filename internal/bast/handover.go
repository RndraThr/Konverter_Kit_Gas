package bast

import (
	"context"
	"errors"
	"sort"
	"strings"
)

// Titik serah BA Serah Terima (Closing Titik Serah). Satu jadwal dapat punya
// beberapa lokasi titik serah. Setiap lokasi mencakup kecamatan/desa
// penerima; penerima masuk ke lokasi menurut urutan: pindah manual, desa,
// kecamatan, lalu lokasi tunggal bila jadwal hanya punya satu lokasi.

var (
	ErrHandoverLocationInvalid = errors.New("handover location is invalid")
	// ErrHandoverUnassigned: masih ada penerima selesai distribusi yang
	// belum masuk ke titik serah mana pun.
	ErrHandoverUnassigned = errors.New("recipients without handover location")
	// ErrHandoverClosingIncomplete: Closing Kabupaten membutuhkan BA Serah
	// Terima final untuk setiap titik serah yang punya penerima.
	ErrHandoverClosingIncomplete = errors.New("closing titik serah of every handover location must be final")
)

const (
	closingDocumentPrefix = AggregateDocumentClosingTitikSerah + ":"
	closingWetKeyPrefix   = "closing-titik-serah@"
)

// Asal penempatan penerima ke titik serah.
const (
	HandoverByOverride = "override"
	HandoverByVillage  = "village"
	HandoverByDistrict = "district"
	HandoverBySingle   = "single"
)

// Status BA Serah Terima satu titik serah.
const (
	HandoverStatusFinal = "final" // dokumen final aktif
	HandoverStatusStale = "stale" // data berubah setelah final
	HandoverStatusNone  = "none"  // belum dibuat
	HandoverStatusEmpty = "empty" // belum ada penerima selesai distribusi
)

// HandoverArea adalah cakupan wilayah satu titik serah; Village kosong berarti
// seluruh kecamatan.
type HandoverArea struct {
	District string `json:"district"`
	Village  string `json:"village,omitempty"`
}

type HandoverLocation struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Areas     []HandoverArea `json:"areas"`
	SortOrder int            `json:"sort_order"`
}

// HandoverLocationInput adalah satu lokasi pada penyimpanan daftar titik
// serah; ID kosong berarti lokasi baru.
type HandoverLocationInput struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Areas []HandoverArea `json:"areas"`
}

// handoverRecipient adalah satu nomor bagi berpenerima pada jadwal.
type handoverRecipient struct {
	SlotID     string
	SlotNumber int
	Name       string
	District   string
	Village    string
	Override   string
	Completed  bool
	LocalDate  string
	Snapshot   []byte
}

// HandoverRecipientView adalah penerima pada pengaturan titik serah.
type HandoverRecipientView struct {
	SlotID     string `json:"slot_id"`
	SlotNumber int    `json:"slot_number"`
	Name       string `json:"name"`
	District   string `json:"district"`
	Village    string `json:"village"`
	LocationID string `json:"location_id,omitempty"`
	Source     string `json:"source,omitempty"`
	Completed  bool   `json:"completed"`
	LocalDate  string `json:"local_date,omitempty"`
}

// HandoverAreaOption adalah satu desa penerima jadwal untuk dipilih sebagai
// cakupan titik serah.
type HandoverAreaOption struct {
	District   string `json:"district"`
	Village    string `json:"village"`
	Recipients int    `json:"recipients"`
}

// HandoverLocationSummary adalah rekap satu titik serah.
type HandoverLocationSummary struct {
	HandoverLocation
	// Recipients: penerima selesai distribusi; Assigned: seluruh penerima
	// (termasuk yang belum selesai).
	Recipients int      `json:"recipients"`
	Assigned   int      `json:"assigned"`
	Dates      []string `json:"dates"`
	Status     string   `json:"status"`
	// DocumentDate: tanggal BA final aktif terbaru.
	DocumentDate string `json:"document_date,omitempty"`
}

// HandoverView adalah pengaturan dan rekap titik serah satu jadwal.
type HandoverView struct {
	ScheduleID string                    `json:"schedule_id"`
	Locations  []HandoverLocationSummary `json:"locations"`
	Areas      []HandoverAreaOption      `json:"areas"`
	Recipients []HandoverRecipientView   `json:"recipients"`
	// Unassigned: penerima selesai distribusi tanpa titik serah.
	Unassigned      int `json:"unassigned"`
	UnassignedTotal int `json:"unassigned_total"`
}

func normalizeArea(value string) string {
	return strings.ToUpper(strings.Join(strings.Fields(value), " "))
}

// assignHandover menentukan titik serah satu penerima.
func assignHandover(locations []HandoverLocation, recipient handoverRecipient) (string, string) {
	if recipient.Override != "" {
		for _, location := range locations {
			if location.ID == recipient.Override {
				return location.ID, HandoverByOverride
			}
		}
	}
	district, village := normalizeArea(recipient.District), normalizeArea(recipient.Village)
	if village != "" {
		for _, location := range locations {
			for _, area := range location.Areas {
				if normalizeArea(area.Village) == village && normalizeArea(area.District) == district {
					return location.ID, HandoverByVillage
				}
			}
		}
	}
	if district != "" {
		for _, location := range locations {
			for _, area := range location.Areas {
				if strings.TrimSpace(area.Village) == "" && normalizeArea(area.District) == district {
					return location.ID, HandoverByDistrict
				}
			}
		}
	}
	if len(locations) == 1 {
		return locations[0].ID, HandoverBySingle
	}
	return "", ""
}

// normalizeHandoverInputs merapikan nama dan cakupan, menolak nama kosong
// atau ganda.
func normalizeHandoverInputs(inputs []HandoverLocationInput) ([]HandoverLocationInput, error) {
	result := make([]HandoverLocationInput, 0, len(inputs))
	names := map[string]bool{}
	for _, input := range inputs {
		name := strings.Join(strings.Fields(input.Name), " ")
		key := strings.ToUpper(name)
		if name == "" || len(name) > 200 || names[key] {
			return nil, ErrHandoverLocationInvalid
		}
		names[key] = true
		areas := []HandoverArea{}
		seen := map[HandoverArea]bool{}
		for _, area := range input.Areas {
			item := HandoverArea{District: normalizeArea(area.District), Village: normalizeArea(area.Village)}
			if item.District == "" || seen[item] {
				continue
			}
			seen[item] = true
			areas = append(areas, item)
		}
		result = append(result, HandoverLocationInput{ID: strings.TrimSpace(input.ID), Name: name, Areas: areas})
	}
	return result, nil
}

// buildHandoverView menghitung penempatan, rekap per lokasi, dan pilihan
// cakupan wilayah.
func buildHandoverView(scheduleID string, locations []HandoverLocation, recipients []handoverRecipient, statuses map[string]handoverDocumentStatus) HandoverView {
	view := HandoverView{ScheduleID: scheduleID, Locations: []HandoverLocationSummary{}, Areas: []HandoverAreaOption{}, Recipients: []HandoverRecipientView{}}
	index := map[string]int{}
	dates := map[string]map[string]bool{}
	for i, location := range locations {
		index[location.ID] = i
		dates[location.ID] = map[string]bool{}
		view.Locations = append(view.Locations, HandoverLocationSummary{HandoverLocation: location, Dates: []string{}, Status: HandoverStatusEmpty})
	}
	type areaKey struct{ district, village string }
	areaOrder := []areaKey{}
	areaCounts := map[areaKey]int{}
	for _, recipient := range recipients {
		locationID, source := assignHandover(locations, recipient)
		view.Recipients = append(view.Recipients, HandoverRecipientView{
			SlotID: recipient.SlotID, SlotNumber: recipient.SlotNumber, Name: recipient.Name,
			District: recipient.District, Village: recipient.Village, LocationID: locationID, Source: source,
			Completed: recipient.Completed, LocalDate: recipient.LocalDate,
		})
		key := areaKey{normalizeArea(recipient.District), normalizeArea(recipient.Village)}
		if key.district != "" {
			if _, exists := areaCounts[key]; !exists {
				areaOrder = append(areaOrder, key)
			}
			areaCounts[key]++
		}
		if locationID == "" {
			view.UnassignedTotal++
			if recipient.Completed {
				view.Unassigned++
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
	sort.SliceStable(areaOrder, func(i, j int) bool {
		if areaOrder[i].district != areaOrder[j].district {
			return areaOrder[i].district < areaOrder[j].district
		}
		return areaOrder[i].village < areaOrder[j].village
	})
	for _, key := range areaOrder {
		view.Areas = append(view.Areas, HandoverAreaOption{District: key.district, Village: key.village, Recipients: areaCounts[key]})
	}
	return view
}

// handoverDocumentStatus adalah status BA Serah Terima satu lokasi.
type handoverDocumentStatus struct {
	Active     bool
	ActiveDate string
	Stale      bool
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
// jadwal punya tepat satu titik serah.
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
func closingRawRowsFor(locations []HandoverLocation, recipients []handoverRecipient, locationID string) []closingRawRow {
	raw := []closingRawRow{}
	for _, recipient := range recipients {
		if !recipient.Completed {
			continue
		}
		if assigned, _ := assignHandover(locations, recipient); assigned == locationID {
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
		locationID, _ := assignHandover(locations, recipient)
		if locationID == "" && strict {
			return nil, ErrHandoverUnassigned
		}
		completed[locationID]++
	}
	rows := []ClosingKabupatenRow{}
	for _, location := range locations {
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
			for _, row := range groupClosingRows(closingRawRowsFor(locations, recipients, location.ID)) {
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
