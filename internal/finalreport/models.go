package finalreport

import (
	"errors"
	"io"
	"time"
)

// Validation sentinels surface verbatim as the API's 400 message, so they
// are worded in Bahasa Indonesia.
var (
	ErrProgramRequired   = errors.New("Program wajib dipilih")
	ErrRegencyRequired   = errors.New("Kabupaten wajib dipilih")
	ErrNotFound          = errors.New("final report resource not found")
	ErrCountNegative     = errors.New("Jumlah anggota tim tidak boleh negatif")
	ErrTooManyPicks      = errors.New("Jumlah foto melebihi slot yang tersedia pada slide ini")
	ErrDuplicatePick     = errors.New("Foto yang sama dipilih lebih dari sekali")
	ErrSlideKeyInvalid   = errors.New("Slide tidak dikenali")
	ErrPhotoOutOfScope   = errors.New("Foto bukan milik program/kabupaten ini")
	ErrPhotoTypeInvalid  = errors.New("Hanya foto JPEG atau PNG yang dapat dipilih")
	ErrRegencyOutOfScope = errors.New("regency is outside the caller's assigned scope")
)

// DocumentContent is an opened generated PPTX, ready to stream to a client.
type DocumentContent struct {
	Filename string
	Reader   io.ReadCloser
}

const (
	SlideKeyPengirimanPenerimaan = "pengiriman_penerimaan"
	SlideKeyRapatKoordinasi      = "rapat_koordinasi"
	SlideKeySosialisasi          = "sosialisasi"
)

var validSlideKeys = map[string]bool{
	SlideKeyPengirimanPenerimaan: true,
	SlideKeyRapatKoordinasi:      true,
	SlideKeySosialisasi:          true,
}

// Settings is the per-program-per-regency input that has no other home in
// the system (susunan tim, delivery dates, machine manufacturer name).
type Settings struct {
	ProgramID                string    `json:"program_id"`
	RegencyID                string    `json:"regency_id"`
	LeaderCount              int       `json:"leader_count"`
	AdminCount               int       `json:"admin_count"`
	TechnicianCount          int       `json:"technician_count"`
	HelperCount              int       `json:"helper_count"`
	MachineManufacturerName  string    `json:"machine_manufacturer_name"`
	DeliveryHoseDate         string    `json:"delivery_hose_date"`
	DeliveryMachineGroupDate string    `json:"delivery_machine_group_date"`
	DeliveryGasCylinderDate  string    `json:"delivery_gas_cylinder_date"`
	UpdatedAt                time.Time `json:"updated_at"`
}

// PhotoPick is one chosen photo for one documentation slide.
type PhotoPick struct {
	ID              string `json:"id"`
	ProgramID       string `json:"program_id"`
	RegencyID       string `json:"regency_id"`
	SlideKey        string `json:"slide_key"`
	ActivityMediaID string `json:"activity_media_id"`
	SortOrder       int    `json:"sort_order"`
	DisplayName     string `json:"display_name"`
	ContentURL      string `json:"content_url"`
	// StorageKey is the underlying activity_media.storage_key, not exposed
	// in the JSON the frontend sees (`json:"-"`) — it exists only so
	// Task 10's Generate can open the photo's bytes via media.Storage.Open.
	StorageKey string    `json:"-"`
	CreatedAt  time.Time `json:"created_at"`
}

// Document is one generated PPTX version, mirroring bast_aggregate_documents.
type Document struct {
	ID          string    `json:"id"`
	ProgramID   string    `json:"program_id"`
	RegencyID   string    `json:"regency_id"`
	Version     int       `json:"version"`
	Filename    string    `json:"filename"`
	Checksum    string    `json:"checksum"`
	StorageKey  string    `json:"storage_key"`
	Status      string    `json:"status"`
	FinalizedAt time.Time `json:"finalized_at"`
}
