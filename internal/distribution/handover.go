package distribution

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"konkit/internal/audit"
	"konkit/internal/auth"
	"konkit/internal/textnorm"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Titik serah: petugas POS Dokumen mencatat lokasi penyerahan setiap nomor
// bagi. Daftar lokasi dipakai bersama satu kabupaten; lokasi baru dibuat dari
// lapangan (juga dari HP offline dengan ID buatan HP), dan nama yang sama
// (beda huruf besar/kecil, spasi, tanda baca) memakai lokasi yang sudah ada.

var ErrHandoverLocationRequired = errors.New("handover location is required")

// HandoverLocation adalah satu titik serah kabupaten.
type HandoverLocation struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SetHandoverLocationInput memilih titik serah nomor bagi: LocationID lokasi
// yang ada, atau LocationName untuk lokasi baru (LocationID boleh ID buatan
// HP agar perubahan offline berikutnya merujuk lokasi yang sama).
type SetHandoverLocationInput struct {
	ScheduleID   string `json:"schedule_id"`
	SlotNumber   int    `json:"slot_number"`
	LocationID   string `json:"handover_location_id"`
	LocationName string `json:"handover_location_name"`
}

type handoverRepository interface {
	ListHandoverLocations(context.Context, string, auth.RegencyScope) ([]HandoverLocation, error)
	SetHandoverLocation(context.Context, auth.Principal, SetHandoverLocationInput, auth.ClientMeta, auth.RegencyScope) (DistributionSlot, error)
}

func (s *Service) ListHandoverLocations(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]HandoverLocation, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if scheduleID == "" {
		return nil, ErrScheduleRequired
	}
	repository, ok := s.posDokumenRepository.(handoverRepository)
	if !ok {
		return nil, ErrSlotNotFound
	}
	return repository.ListHandoverLocations(ctx, scheduleID, scope)
}

func (s *Service) SetHandoverLocation(ctx context.Context, actor auth.Principal, input SetHandoverLocationInput, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionSlot, error) {
	input.ScheduleID = strings.TrimSpace(input.ScheduleID)
	input.LocationID = strings.TrimSpace(input.LocationID)
	input.LocationName = textnorm.PlaceName(input.LocationName)
	if input.ScheduleID == "" {
		return DistributionSlot{}, ErrScheduleRequired
	}
	if input.SlotNumber < 1 {
		return DistributionSlot{}, ErrSlotNumberRequired
	}
	if input.LocationID != "" {
		if _, err := uuid.Parse(input.LocationID); err != nil {
			return DistributionSlot{}, ErrHandoverLocationRequired
		}
	}
	if input.LocationID == "" && textnorm.PlaceKey(input.LocationName) == "" || len(input.LocationName) > 200 {
		return DistributionSlot{}, ErrHandoverLocationRequired
	}
	repository, ok := s.posDokumenRepository.(handoverRepository)
	if !ok {
		return DistributionSlot{}, ErrSlotNotFound
	}
	return repository.SetHandoverLocation(ctx, actor, input, meta, scope)
}

// ListHandoverLocations mengembalikan titik serah kabupaten jadwal, urut
// nama. Kabupaten yang belum punya daftar memakai titik serah pada
// pengaturan BA jadwal sebagai lokasi pertamanya.
func (r *Repository) ListHandoverLocations(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]HandoverLocation, error) {
	var regencyID, seed string
	err := r.pool.QueryRow(ctx, `
		SELECT ps.regency_id::text, COALESCE(st.handover_location,'')
		FROM program_schedules ps LEFT JOIN bast_schedule_settings st ON st.schedule_id = ps.id
		WHERE ps.id::text=$1 AND ($2 OR ps.regency_id::text = ANY($3))
	`, scheduleID, scope.Unrestricted, scope.RegencyIDs).Scan(&regencyID, &seed)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSlotNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("handover schedule: %w", err)
	}
	if name := textnorm.PlaceName(seed); textnorm.PlaceKey(name) != "" {
		if _, err := r.pool.Exec(ctx, `
			INSERT INTO bast_handover_locations (regency_id, name, name_key)
			SELECT $1::uuid, $2, $3 WHERE NOT EXISTS (SELECT 1 FROM bast_handover_locations WHERE regency_id::text=$1)
			ON CONFLICT DO NOTHING
		`, regencyID, name, textnorm.PlaceKey(name)); err != nil {
			return nil, fmt.Errorf("seed handover location: %w", err)
		}
	}
	rows, err := r.pool.Query(ctx, `SELECT id::text, name FROM bast_handover_locations WHERE regency_id::text=$1 ORDER BY upper(name), id`, regencyID)
	if err != nil {
		return nil, fmt.Errorf("list handover locations: %w", err)
	}
	defer rows.Close()
	items := []HandoverLocation{}
	for rows.Next() {
		var item HandoverLocation
		if err := rows.Scan(&item.ID, &item.Name); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// resolveHandoverLocation mengembalikan lokasi kabupaten untuk pilihan
// petugas: ID yang ada, nama yang sudah ada, atau lokasi baru.
func resolveHandoverLocation(ctx context.Context, tx pgx.Tx, actor auth.Principal, regencyID string, input SetHandoverLocationInput) (string, bool, error) {
	var id string
	if input.LocationID != "" {
		err := tx.QueryRow(ctx, `SELECT id::text FROM bast_handover_locations WHERE id::text=$1 AND regency_id::text=$2`, input.LocationID, regencyID).Scan(&id)
		if err == nil {
			return id, false, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", false, fmt.Errorf("find handover location: %w", err)
		}
	}
	key := textnorm.PlaceKey(input.LocationName)
	if key == "" {
		return "", false, ErrHandoverLocationRequired
	}
	newID := input.LocationID
	if newID == "" {
		newID = uuid.NewString()
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO bast_handover_locations (id, regency_id, name, name_key, created_by)
		VALUES ($1::uuid, $2::uuid, $3, $4, NULLIF($5,'')::uuid)
		ON CONFLICT DO NOTHING
	`, newID, regencyID, input.LocationName, key, actor.UserID)
	if err != nil {
		return "", false, fmt.Errorf("create handover location: %w", err)
	}
	if err := tx.QueryRow(ctx, `SELECT id::text FROM bast_handover_locations WHERE regency_id::text=$1 AND name_key=$2`, regencyID, key).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, ErrHandoverLocationRequired
		}
		return "", false, fmt.Errorf("find handover location by name: %w", err)
	}
	return id, tag.RowsAffected() > 0, nil
}

// SetHandoverLocation mencatat titik serah nomor bagi (sebelum diselesaikan;
// koreksi setelahnya lewat BA Closing Titik Serah).
func (r *Repository) SetHandoverLocation(ctx context.Context, actor auth.Principal, input SetHandoverLocationInput, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionSlot, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("begin handover location: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var slotID, status, regencyID string
	err = tx.QueryRow(ctx, `
		SELECT ds.id::text, ds.status, ps.regency_id::text
		FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id=ds.schedule_id
		WHERE ds.schedule_id::text=$1 AND ds.slot_number=$2 AND ($3 OR ps.regency_id::text = ANY($4))
		FOR UPDATE OF ds
	`, input.ScheduleID, input.SlotNumber, scope.Unrestricted, scope.RegencyIDs).Scan(&slotID, &status, &regencyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return DistributionSlot{}, ErrSlotNotFound
	}
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("lock handover slot: %w", err)
	}
	if status == "completed" {
		return DistributionSlot{}, ErrAlreadyCompleted
	}
	locationID, created, err := resolveHandoverLocation(ctx, tx, actor, regencyID, input)
	if err != nil {
		return DistributionSlot{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE distribution_slots SET handover_location_id=$2::uuid, updated_at=now() WHERE id=$1`, slotID, locationID); err != nil {
		return DistributionSlot{}, fmt.Errorf("update handover location: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "distribution.handover_location_updated", ResourceType: "distribution_slot", ResourceID: slotID, Metadata: map[string]any{"location_id": locationID, "created": created, "name": input.LocationName}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return DistributionSlot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DistributionSlot{}, fmt.Errorf("commit handover location: %w", err)
	}
	return r.getSlotByID(ctx, slotID)
}

// defaultHandoverLocation mengisi titik serah nomor bagi yang diselesaikan
// tanpa pilihan bila kabupatennya hanya punya satu titik serah.
func defaultHandoverLocation(ctx context.Context, tx pgx.Tx, slotID string) error {
	_, err := tx.Exec(ctx, `
		UPDATE distribution_slots ds SET handover_location_id = (
			SELECT l.id FROM bast_handover_locations l JOIN program_schedules ps ON ps.regency_id = l.regency_id WHERE ps.id = ds.schedule_id)
		WHERE ds.id::text=$1 AND ds.handover_location_id IS NULL
		  AND (SELECT count(*) FROM bast_handover_locations l JOIN program_schedules ps ON ps.regency_id = l.regency_id WHERE ps.id = ds.schedule_id) = 1
	`, slotID)
	if err != nil {
		return fmt.Errorf("default handover location: %w", err)
	}
	return nil
}
