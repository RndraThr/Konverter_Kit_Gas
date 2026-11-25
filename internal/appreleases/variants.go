package appreleases

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"konkit/internal/audit"
	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
)

// PrimaryABI is the phone type of a release's own APK.
const PrimaryABI = "arm64-v8a"

// variantABIs are the other phone types a release can carry an APK for.
var variantABIs = []string{"armeabi-v7a", "x86_64"}

var (
	// ErrVariantABI: the APK has no native code for a variant phone type (or
	// it is the 64-bit APK, which is the release's own).
	ErrVariantABI = errors.New("APK is not a 32-bit (armeabi-v7a) or x86_64 build")
)

// Variant is a release's APK for another phone type.
type Variant struct {
	ABI        string `json:"abi"`
	SizeBytes  int64  `json:"size_bytes"`
	Checksum   string `json:"checksum"`
	storageKey string
}

// apkABI is the one variant phone type an APK is built for, read from its
// native libraries (lib/<abi>/...). An APK that also holds arm64-v8a code
// (the 64-bit or a universal build) is not a variant.
func apkABI(data []byte) (string, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", ErrNotAnAPK
	}
	found := map[string]bool{}
	for _, file := range archive.File {
		if rest, ok := strings.CutPrefix(file.Name, "lib/"); ok {
			if abi, _, ok := strings.Cut(rest, "/"); ok {
				found[abi] = true
			}
		}
	}
	if found[PrimaryABI] {
		return "", ErrVariantABI
	}
	var abis []string
	for _, abi := range variantABIs {
		if found[abi] {
			abis = append(abis, abi)
		}
	}
	if len(abis) != 1 {
		return "", ErrVariantABI
	}
	return abis[0], nil
}

// AddVariant stores the APK of release id for another phone type, found from
// the APK itself. A later upload for the same type replaces it.
func (s *Service) AddVariant(ctx context.Context, actor auth.Principal, id string, apk io.Reader, meta auth.ClientMeta) (Variant, error) {
	if s.storage == nil {
		return Variant{}, errMissingStorage
	}
	release, err := s.get(ctx, id)
	if err != nil {
		return Variant{}, err
	}
	data, err := io.ReadAll(io.LimitReader(apk, MaxAPKBytes+1))
	if err != nil {
		return Variant{}, err
	}
	if int64(len(data)) > MaxAPKBytes {
		return Variant{}, ErrFileTooLarge
	}
	if !bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		return Variant{}, ErrNotAnAPK
	}
	abi, err := apkABI(data)
	if err != nil {
		return Variant{}, err
	}
	key := fmt.Sprintf("apk-android-%d-%s-%d", release.VersionCode, abi, time.Now().UnixNano())
	storageKey, size, checksum, err := s.storage.Put(ctx, key, nil, bytes.NewReader(data))
	if err != nil {
		return Variant{}, fmt.Errorf("store APK: %w", err)
	}
	var oldKey string
	_ = s.pool.QueryRow(ctx, `SELECT storage_key FROM app_release_variants WHERE release_id::text = $1 AND abi = $2`, id, abi).Scan(&oldKey)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		_ = s.storage.Delete(ctx, storageKey)
		return Variant{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `INSERT INTO app_release_variants (release_id, abi, storage_key, size_bytes, checksum, uploaded_by)
		VALUES ($1::uuid, $2, $3, $4, $5, NULLIF($6, '')::uuid)
		ON CONFLICT (release_id, abi) DO UPDATE SET storage_key = EXCLUDED.storage_key, size_bytes = EXCLUDED.size_bytes,
			checksum = EXCLUDED.checksum, uploaded_by = EXCLUDED.uploaded_by, created_at = now()`,
		id, abi, storageKey, size, checksum, actor.UserID); err != nil {
		_ = s.storage.Delete(ctx, storageKey)
		return Variant{}, fmt.Errorf("insert app release variant: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "app_release.variant_uploaded", ResourceType: "app_release", ResourceID: id,
		Metadata:  map[string]any{"version_name": release.VersionName, "version_code": release.VersionCode, "abi": abi, "size_bytes": size},
		IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		_ = s.storage.Delete(ctx, storageKey)
		return Variant{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		_ = s.storage.Delete(ctx, storageKey)
		return Variant{}, err
	}
	if oldKey != "" && oldKey != storageKey {
		_ = s.storage.Delete(ctx, oldKey)
	}
	return Variant{ABI: abi, SizeBytes: size, Checksum: checksum, storageKey: storageKey}, nil
}

// variantABI is the variant type for a phone supporting abis (its preferred
// first), or "" when the phone runs the release's own 64-bit APK. Phones that
// do not say (older app versions) get the 64-bit APK.
func variantABI(abis []string) string {
	if len(abis) == 0 || slices.Contains(abis, PrimaryABI) {
		return ""
	}
	for _, abi := range abis {
		if slices.Contains(variantABIs, abi) {
			return abi
		}
	}
	return abis[0]
}

// LatestFor is Latest for a phone supporting abis: a phone that cannot run
// the 64-bit APK is offered the newest release that has an APK for it, and
// downloads that APK whole.
func (s *Service) LatestFor(ctx context.Context, platform string, currentCode int, abis []string) (*Update, error) {
	abi := variantABI(abis)
	if abi == "" {
		return s.Latest(ctx, platform, currentCode)
	}
	var variant Variant
	release, err := scanReleaseWith(s.pool.QueryRow(ctx, `SELECT `+releaseColumns+`, v.size_bytes, v.checksum FROM app_releases r
		JOIN app_release_variants v ON v.release_id = r.id AND v.abi = $2
		LEFT JOIN users u ON u.id = r.uploaded_by
		WHERE r.platform = $1 AND `+releaseReady+` ORDER BY r.version_code DESC LIMIT 1`, platform, abi), &variant.SizeBytes, &variant.Checksum)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("latest app release for %s: %w", abi, err)
	}
	if release.VersionCode <= currentCode {
		return nil, nil
	}
	var mandatory bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM app_releases WHERE platform = $1 AND version_code > $2 AND version_code <= $3 AND mandatory)`,
		platform, currentCode, release.VersionCode).Scan(&mandatory); err != nil {
		return nil, fmt.Errorf("mandatory app release: %w", err)
	}
	release.SizeBytes, release.Checksum = variant.SizeBytes, variant.Checksum
	return &Update{Release: release, Mandatory: mandatory, DownloadPath: "/api/v1/app/releases/" + release.ID + "/apk?abi=" + abi}, nil
}

func scanReleaseWith(row pgx.Row, extra ...any) (Release, error) {
	var r Release
	err := row.Scan(append([]any{&r.ID, &r.Platform, &r.VersionName, &r.VersionCode, &r.Notes, &r.Mandatory, &r.SizeBytes, &r.Checksum, &r.UploadedBy, &r.CreatedAt, &r.storageKey}, extra...)...)
	return r, err
}

// OpenVariant streams release id's APK for abi ("" or arm64-v8a: its own).
func (s *Service) OpenVariant(ctx context.Context, id, abi string) (Content, error) {
	if abi == "" || abi == PrimaryABI {
		return s.Open(ctx, id)
	}
	if s.storage == nil {
		return Content{}, errMissingStorage
	}
	var key, name string
	var size int64
	err := s.pool.QueryRow(ctx, `SELECT v.storage_key, v.size_bytes, r.version_name FROM app_release_variants v
		JOIN app_releases r ON r.id = v.release_id WHERE v.release_id::text = $1 AND v.abi = $2`, id, abi).Scan(&key, &size, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return Content{}, ErrNotFound
	}
	if err != nil {
		return Content{}, fmt.Errorf("get app release variant: %w", err)
	}
	reader, err := s.storage.Open(ctx, key)
	if err != nil {
		return Content{}, fmt.Errorf("open APK: %w", err)
	}
	return Content{Reader: reader, Size: size, Filename: fmt.Sprintf("konkit-%s-%s.apk", name, abi)}, nil
}

// variantsByRelease lists every release's variants (website list).
func (s *Service) variantsByRelease(ctx context.Context) (map[string][]Variant, error) {
	rows, err := s.pool.Query(ctx, `SELECT release_id::text, abi, size_bytes, checksum, storage_key FROM app_release_variants ORDER BY abi`)
	if err != nil {
		return nil, fmt.Errorf("list app release variants: %w", err)
	}
	defer rows.Close()
	result := map[string][]Variant{}
	for rows.Next() {
		var id string
		var v Variant
		if err := rows.Scan(&id, &v.ABI, &v.SizeBytes, &v.Checksum, &v.storageKey); err != nil {
			return nil, err
		}
		result[id] = append(result[id], v)
	}
	return result, rows.Err()
}
