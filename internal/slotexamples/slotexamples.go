// Package slotexamples keeps one example photo per distribution
// documentation slot code, shown to field officers as a reference for the
// angle and framing of the photo they are about to take.
package slotexamples

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"konkit/internal/audit"
	"konkit/internal/auth"
	"konkit/internal/media"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MaxImageBytes caps an uploaded example photo.
const MaxImageBytes = 10 << 20

const maxCaptionRunes = 500

var (
	ErrNotFound       = errors.New("documentation example not found")
	ErrInvalidInput   = errors.New("documentation example input is invalid")
	ErrImageRequired  = errors.New("an example photo is required")
	ErrImageInvalid   = errors.New("example photo must be JPEG, PNG, or WebP")
	ErrImageTooLarge  = errors.New("example photo is too large")
	errMissingStorage = errors.New("documentation example storage is not configured")
)

var slotCodePattern = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)

// Slot is a documentation slot code found in the templates or in recorded
// distributions, with its example when one is set.
type Slot struct {
	Code    string   `json:"slot_code"`
	Label   string   `json:"label"`
	Stage   string   `json:"stage"`
	Example *Example `json:"example"`
}

type Example struct {
	Caption   string    `json:"caption"`
	ImageURL  string    `json:"image_url"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SaveInput sets a slot's caption and, when Image is not nil, its photo. A
// slot without an example needs a photo.
type SaveInput struct {
	SlotCode         string
	Caption          string
	Image            []byte
	OriginalFilename string
}

type Content struct {
	Reader   io.ReadCloser
	MimeType string
	Filename string
}

type Service struct {
	pool    *pgxpool.Pool
	storage media.Storage
}

func NewService(pool *pgxpool.Pool, storage media.Storage) *Service {
	return &Service{pool: pool, storage: storage}
}

// List returns every known slot code ordered by stage and position, each
// with its example when one is set. Labels come from the newest published
// template, then any template, then the recorded distributions (on-demand
// slots such as Surat Kuasa exist only there).
func (s *Service) List(ctx context.Context) ([]Slot, error) {
	rows, err := s.pool.Query(ctx, `
		WITH known AS (
			SELECT DISTINCT ON (slot_code) slot_code, label, stage, sort_order FROM (
				SELECT ts.slot_code, ts.label, ts.stage, ts.sort_order, 0 AS source, (v.status = 'published') AS published, v.version
				FROM documentation_template_slots ts JOIN documentation_template_versions v ON v.id = ts.template_version_id
				UNION ALL
				SELECT ds.slot_code, ds.label_snapshot, ds.stage, ds.sort_order, 1, false, 0 FROM documentation_slots ds
			) candidates
			ORDER BY slot_code, source, published DESC, version DESC
		)
		SELECT COALESCE(k.slot_code, e.slot_code), COALESCE(k.label, e.slot_code), COALESCE(k.stage, ''),
			e.caption, e.updated_at
		FROM known k FULL JOIN documentation_slot_examples e ON e.slot_code = k.slot_code
		ORDER BY array_position(ARRAY['mesin','dokumen','penyerahan'], k.stage) NULLS LAST, k.sort_order NULLS LAST, 1`)
	if err != nil {
		return nil, fmt.Errorf("list documentation examples: %w", err)
	}
	defer rows.Close()
	result := []Slot{}
	for rows.Next() {
		var slot Slot
		var caption *string
		var updatedAt *time.Time
		if err := rows.Scan(&slot.Code, &slot.Label, &slot.Stage, &caption, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan documentation example: %w", err)
		}
		if updatedAt != nil {
			slot.Example = &Example{Caption: *caption, ImageURL: imageURL(slot.Code, *updatedAt), UpdatedAt: *updatedAt}
		}
		result = append(result, slot)
	}
	return result, rows.Err()
}

// imageURL carries the update time so a replaced photo is not served from cache.
func imageURL(code string, updatedAt time.Time) string {
	return fmt.Sprintf("/api/v1/documentation-examples/%s/image?v=%d", code, updatedAt.Unix())
}

// Save creates or updates a slot's example.
func (s *Service) Save(ctx context.Context, actor auth.Principal, input SaveInput, meta auth.ClientMeta) (Example, error) {
	if s.storage == nil {
		return Example{}, errMissingStorage
	}
	input.SlotCode = strings.TrimSpace(input.SlotCode)
	input.Caption = strings.TrimSpace(input.Caption)
	if !slotCodePattern.MatchString(input.SlotCode) || utf8.RuneCountInString(input.Caption) > maxCaptionRunes {
		return Example{}, ErrInvalidInput
	}
	if len(input.Image) > MaxImageBytes {
		return Example{}, ErrImageTooLarge
	}
	var mimeType, storageKey, checksum string
	var size int64
	if input.Image != nil {
		mimeType = http.DetectContentType(input.Image[:min(len(input.Image), 512)])
		if mimeType != "image/jpeg" && mimeType != "image/png" && mimeType != "image/webp" {
			return Example{}, ErrImageInvalid
		}
		var err error
		storageKey, size, checksum, err = s.storage.Put(ctx, "slot-example-"+uuid.NewString(), nil, bytes.NewReader(input.Image))
		if err != nil {
			return Example{}, fmt.Errorf("store example photo: %w", err)
		}
	}
	example, oldKey, err := s.save(ctx, actor, input, storageKey, mimeType, size, checksum, meta)
	if err != nil {
		if storageKey != "" {
			_ = s.storage.Delete(context.Background(), storageKey)
		}
		return Example{}, err
	}
	if oldKey != "" && oldKey != storageKey {
		_ = s.storage.Delete(context.Background(), oldKey)
	}
	return example, nil
}

func (s *Service) save(ctx context.Context, actor auth.Principal, input SaveInput, storageKey, mimeType string, size int64, checksum string, meta auth.ClientMeta) (Example, string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Example{}, "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var oldKey string
	err = tx.QueryRow(ctx, `SELECT storage_key FROM documentation_slot_examples WHERE slot_code = $1 FOR UPDATE`, input.SlotCode).Scan(&oldKey)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Example{}, "", fmt.Errorf("lock documentation example: %w", err)
	}
	var updatedAt time.Time
	switch {
	case storageKey != "":
		err = tx.QueryRow(ctx, `
			INSERT INTO documentation_slot_examples (slot_code, caption, storage_key, original_filename, mime_type, byte_size, checksum, updated_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, '')::uuid)
			ON CONFLICT (slot_code) DO UPDATE SET caption = EXCLUDED.caption, storage_key = EXCLUDED.storage_key,
				original_filename = EXCLUDED.original_filename, mime_type = EXCLUDED.mime_type, byte_size = EXCLUDED.byte_size,
				checksum = EXCLUDED.checksum, updated_by = EXCLUDED.updated_by, updated_at = now()
			RETURNING updated_at`,
			input.SlotCode, input.Caption, storageKey, input.OriginalFilename, mimeType, size, checksum, actor.UserID).Scan(&updatedAt)
	case oldKey != "":
		err = tx.QueryRow(ctx, `UPDATE documentation_slot_examples SET caption = $2, updated_by = NULLIF($3, '')::uuid, updated_at = now() WHERE slot_code = $1 RETURNING updated_at`,
			input.SlotCode, input.Caption, actor.UserID).Scan(&updatedAt)
	default:
		return Example{}, "", ErrImageRequired
	}
	if err != nil {
		return Example{}, "", fmt.Errorf("save documentation example: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "documentation_example.saved", ResourceType: "documentation_slot_example", ResourceID: input.SlotCode,
		Metadata: map[string]any{"photo_replaced": storageKey != "", "caption": input.Caption}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return Example{}, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return Example{}, "", err
	}
	return Example{Caption: input.Caption, ImageURL: imageURL(input.SlotCode, updatedAt), UpdatedAt: updatedAt}, oldKey, nil
}

// Delete removes a slot's example.
func (s *Service) Delete(ctx context.Context, actor auth.Principal, slotCode string, meta auth.ClientMeta) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var storageKey string
	err = tx.QueryRow(ctx, `DELETE FROM documentation_slot_examples WHERE slot_code = $1 RETURNING storage_key`, slotCode).Scan(&storageKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("delete documentation example: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "documentation_example.deleted", ResourceType: "documentation_slot_example", ResourceID: slotCode,
		IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if s.storage != nil {
		_ = s.storage.Delete(context.Background(), storageKey)
	}
	return nil
}

// Open streams a slot's example photo.
func (s *Service) Open(ctx context.Context, slotCode string) (Content, error) {
	if s.storage == nil {
		return Content{}, errMissingStorage
	}
	var storageKey string
	var content Content
	err := s.pool.QueryRow(ctx, `SELECT storage_key, mime_type, original_filename FROM documentation_slot_examples WHERE slot_code = $1`, slotCode).
		Scan(&storageKey, &content.MimeType, &content.Filename)
	if errors.Is(err, pgx.ErrNoRows) {
		return Content{}, ErrNotFound
	}
	if err != nil {
		return Content{}, fmt.Errorf("get documentation example: %w", err)
	}
	if content.Reader, err = s.storage.Open(ctx, storageKey); err != nil {
		return Content{}, fmt.Errorf("open example photo: %w", err)
	}
	return content, nil
}
