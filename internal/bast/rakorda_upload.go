package bast

import (
	"io"
	"time"
)

const maxRakordaUploadBytes = 20 << 20

// RakordaKind membedakan jenis daftar hadir yang berbagi alur cetak kosong
// lalu unggah hasil terisi (RAKORDA, Sosialisasi, Training 10%/100%).
type RakordaKind string

const (
	RakordaKindRakorda     RakordaKind = "rakorda"
	RakordaKindSosialisasi RakordaKind = "sosialisasi"
	RakordaKindTraining10  RakordaKind = "training_10"
	RakordaKindTraining100 RakordaKind = "training_100"
)

type RakordaUpload struct {
	ID           string      `json:"id"`
	ScheduleID   string      `json:"schedule_id"`
	DocumentKind RakordaKind `json:"document_kind"`
	EventDate    string      `json:"event_date"`
	OriginalName string      `json:"original_name"`
	MimeType     string      `json:"mime_type"`
	ByteSize     int64       `json:"byte_size"`
	CreatedAt    time.Time   `json:"created_at"`
	StorageKey   string      `json:"-"`
}

type RakordaUploadInput struct {
	ScheduleID   string
	EventDate    string
	OriginalName string
	Data         []byte
}

type RakordaContent struct {
	Reader   io.ReadCloser
	Filename string
	MimeType string
}
