// Package appreleases serves mobile app updates outside the Play Store: the
// admin uploads a signed APK with its version, phones ask for the newest one,
// download it and install it (mandatory releases must be installed).
package appreleases

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"konkit/internal/audit"
	"konkit/internal/auth"
	"konkit/internal/media"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MaxAPKBytes caps an uploaded APK.
const MaxAPKBytes int64 = 200 << 20

var (
	ErrNotFound       = errors.New("app release not found")
	ErrInvalidInput   = errors.New("app release input is invalid")
	ErrNotAnAPK       = errors.New("uploaded file is not an APK")
	ErrVersionTooLow  = errors.New("version code must be higher than the latest release")
	ErrVersionExists  = errors.New("a release with this version code exists")
	ErrFileTooLarge   = errors.New("APK is too large")
	errMissingStorage = errors.New("app release storage is not configured")
)

type Release struct {
	ID          string    `json:"id"`
	Platform    string    `json:"platform"`
	VersionName string    `json:"version_name"`
	VersionCode int       `json:"version_code"`
	Notes       string    `json:"notes"`
	Mandatory   bool      `json:"mandatory"`
	SizeBytes   int64     `json:"size_bytes"`
	Checksum    string    `json:"checksum"`
	UploadedBy  string    `json:"uploaded_by_name,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	storageKey  string
}

// Update is what a phone on CurrentCode is offered: the newest release, and
// whether it must install it (some release newer than its own is mandatory).
type Update struct {
	Release
	Mandatory    bool   `json:"mandatory"`
	DownloadPath string `json:"download_path"`
}

type UploadInput struct {
	VersionName string
	VersionCode int
	Notes       string
	Mandatory   bool
}

type Content struct {
	Reader   io.ReadCloser
	Size     int64
	Filename string
}

type Service struct {
	pool    *pgxpool.Pool
	storage media.Storage
}

func NewService(pool *pgxpool.Pool, storage media.Storage) *Service {
	return &Service{pool: pool, storage: storage}
}

const releaseColumns = `r.id::text, r.platform, r.version_name, r.version_code, r.notes, r.mandatory, r.size_bytes, r.checksum,
	COALESCE(NULLIF(u.full_name, ''), u.username, ''), r.created_at, r.storage_key`

func scanRelease(row pgx.Row) (Release, error) {
	var r Release
	err := row.Scan(&r.ID, &r.Platform, &r.VersionName, &r.VersionCode, &r.Notes, &r.Mandatory, &r.SizeBytes, &r.Checksum, &r.UploadedBy, &r.CreatedAt, &r.storageKey)
	return r, err
}

// List returns every release, newest first.
func (s *Service) List(ctx context.Context) ([]Release, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+releaseColumns+` FROM app_releases r LEFT JOIN users u ON u.id = r.uploaded_by ORDER BY r.version_code DESC`)
	if err != nil {
		return nil, fmt.Errorf("list app releases: %w", err)
	}
	defer rows.Close()
	result := []Release{}
	for rows.Next() {
		release, err := scanRelease(rows)
		if err != nil {
			return nil, fmt.Errorf("scan app release: %w", err)
		}
		result = append(result, release)
	}
	return result, rows.Err()
}

// Latest is the update for a phone running currentCode, or nil when it is
// already on the newest release.
func (s *Service) Latest(ctx context.Context, platform string, currentCode int) (*Update, error) {
	release, err := scanRelease(s.pool.QueryRow(ctx, `SELECT `+releaseColumns+` FROM app_releases r LEFT JOIN users u ON u.id = r.uploaded_by
		WHERE r.platform = $1 ORDER BY r.version_code DESC LIMIT 1`, platform))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("latest app release: %w", err)
	}
	if release.VersionCode <= currentCode {
		return nil, nil
	}
	var mandatory bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM app_releases WHERE platform = $1 AND version_code > $2 AND mandatory)`, platform, currentCode).Scan(&mandatory); err != nil {
		return nil, fmt.Errorf("mandatory app release: %w", err)
	}
	return &Update{Release: release, Mandatory: mandatory, DownloadPath: "/api/v1/app/releases/" + release.ID + "/apk"}, nil
}

// Upload stores an APK as a new release. Its version code must be higher
// than every earlier release: phones only move forward.
func (s *Service) Upload(ctx context.Context, actor auth.Principal, input UploadInput, apk io.Reader, meta auth.ClientMeta) (Release, error) {
	if s.storage == nil {
		return Release{}, errMissingStorage
	}
	input.VersionName = strings.TrimSpace(input.VersionName)
	input.Notes = strings.TrimSpace(input.Notes)
	if input.VersionName == "" || len(input.VersionName) > 40 || input.VersionCode <= 0 || len(input.Notes) > 4000 {
		return Release{}, ErrInvalidInput
	}
	var latest int
	if err := s.pool.QueryRow(ctx, `SELECT COALESCE(MAX(version_code), 0) FROM app_releases WHERE platform = 'android'`).Scan(&latest); err != nil {
		return Release{}, fmt.Errorf("latest app release code: %w", err)
	}
	if input.VersionCode <= latest {
		return Release{}, ErrVersionTooLow
	}
	// An APK is a zip archive: it starts with "PK\x03\x04".
	buffered := bufio.NewReader(io.LimitReader(apk, MaxAPKBytes+1))
	if head, err := buffered.Peek(4); err != nil || !bytes.Equal(head, []byte("PK\x03\x04")) {
		return Release{}, ErrNotAnAPK
	}
	key := fmt.Sprintf("apk-android-%d-%d", input.VersionCode, time.Now().UnixNano())
	storageKey, size, checksum, err := s.storage.Put(ctx, key, nil, buffered)
	if err != nil {
		return Release{}, fmt.Errorf("store APK: %w", err)
	}
	if size > MaxAPKBytes {
		_ = s.storage.Delete(ctx, storageKey)
		return Release{}, ErrFileTooLarge
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		_ = s.storage.Delete(ctx, storageKey)
		return Release{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO app_releases (platform, version_name, version_code, notes, mandatory, storage_key, size_bytes, checksum, uploaded_by)
		VALUES ('android', $1, $2, $3, $4, $5, $6, $7, NULLIF($8, '')::uuid) RETURNING id::text`,
		input.VersionName, input.VersionCode, input.Notes, input.Mandatory, storageKey, size, checksum, actor.UserID).Scan(&id)
	if err != nil {
		_ = s.storage.Delete(ctx, storageKey)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Release{}, ErrVersionExists
		}
		return Release{}, fmt.Errorf("insert app release: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "app_release.uploaded", ResourceType: "app_release", ResourceID: id,
		Metadata:  map[string]any{"version_name": input.VersionName, "version_code": input.VersionCode, "mandatory": input.Mandatory, "size_bytes": size},
		IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		_ = s.storage.Delete(ctx, storageKey)
		return Release{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		_ = s.storage.Delete(ctx, storageKey)
		return Release{}, err
	}
	return s.get(ctx, id)
}

// SetMandatory turns a release's "must install" flag on or off.
func (s *Service) SetMandatory(ctx context.Context, actor auth.Principal, id string, mandatory bool, meta auth.ClientMeta) (Release, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Release{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `UPDATE app_releases SET mandatory = $2 WHERE id::text = $1`, id, mandatory)
	if err != nil {
		return Release{}, fmt.Errorf("update app release: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Release{}, ErrNotFound
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "app_release.updated", ResourceType: "app_release", ResourceID: id,
		Metadata: map[string]any{"mandatory": mandatory}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return Release{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Release{}, err
	}
	return s.get(ctx, id)
}

// Delete removes a release (a broken build); phones fall back to the
// newest remaining one.
func (s *Service) Delete(ctx context.Context, actor auth.Principal, id string, meta auth.ClientMeta) error {
	release, err := s.get(ctx, id)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM app_releases WHERE id::text = $1`, id); err != nil {
		return fmt.Errorf("delete app release: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "app_release.deleted", ResourceType: "app_release", ResourceID: id,
		Metadata: map[string]any{"version_name": release.VersionName, "version_code": release.VersionCode}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	_ = s.storage.Delete(ctx, release.storageKey)
	return nil
}

// Open streams a release's APK.
func (s *Service) Open(ctx context.Context, id string) (Content, error) {
	if s.storage == nil {
		return Content{}, errMissingStorage
	}
	release, err := s.get(ctx, id)
	if err != nil {
		return Content{}, err
	}
	reader, err := s.storage.Open(ctx, release.storageKey)
	if err != nil {
		return Content{}, fmt.Errorf("open APK: %w", err)
	}
	return Content{Reader: reader, Size: release.SizeBytes, Filename: fmt.Sprintf("konkit-%s.apk", release.VersionName)}, nil
}

func (s *Service) get(ctx context.Context, id string) (Release, error) {
	release, err := scanRelease(s.pool.QueryRow(ctx, `SELECT `+releaseColumns+` FROM app_releases r LEFT JOIN users u ON u.id = r.uploaded_by WHERE r.id::text = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Release{}, ErrNotFound
	}
	if err != nil {
		return Release{}, fmt.Errorf("get app release: %w", err)
	}
	return release, nil
}
