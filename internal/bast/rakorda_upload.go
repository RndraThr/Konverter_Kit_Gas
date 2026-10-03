package bast

import (
	"io"
	"time"
)

const maxRakordaUploadBytes = 20 << 20

type RakordaUpload struct {
	ID           string    `json:"id"`
	ScheduleID   string    `json:"schedule_id"`
	EventDate    string    `json:"event_date"`
	OriginalName string    `json:"original_name"`
	MimeType     string    `json:"mime_type"`
	ByteSize     int64     `json:"byte_size"`
	CreatedAt    time.Time `json:"created_at"`
	StorageKey   string    `json:"-"`
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
