package distribution

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"strings"
	"time"

	"konkit/internal/audit"
	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
)

// Recipient signature taken in POS Dokumen (optional): a small transparent PNG
// drawn on the phone or in the browser. It belongs to the recipient linked at
// signing time; after a replacement it no longer counts (see
// signedRecipientJoin). Without it, Berita Acara leaves the signature box
// empty for a wet signature.

var ErrSignatureInvalid = errors.New("recipient signature must be a PNG image")

const (
	maxSignatureBytes = 512 << 10
	maxSignatureSide  = 4000
	minSignatureSide  = 40
)

// signedRecipientJoin keeps only a signature of the slot's current recipient.
const signedRecipientJoin = `JOIN distribution_slots sig_ds ON sig_ds.id = sig.distribution_slot_id AND sig_ds.recipient_person_id = sig.person_id`

type SaveSignatureInput struct {
	ScheduleID string
	SlotNumber int
	ImagePNG   []byte
}

type SignatureContent struct {
	ImagePNG []byte
	SignedAt time.Time
}

type signatureRepository interface {
	SaveRecipientSignature(ctx context.Context, actor auth.Principal, input SaveSignatureInput, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionSlot, error)
	DeleteRecipientSignature(ctx context.Context, actor auth.Principal, scheduleID string, slotNumber int, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionSlot, error)
	OpenRecipientSignature(ctx context.Context, scheduleID string, slotNumber int, scope auth.RegencyScope) (SignatureContent, error)
}

func (s *Service) SaveRecipientSignature(ctx context.Context, actor auth.Principal, input SaveSignatureInput, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionSlot, error) {
	input.ScheduleID = strings.TrimSpace(input.ScheduleID)
	if input.ScheduleID == "" {
		return DistributionSlot{}, ErrScheduleRequired
	}
	if input.SlotNumber < 1 {
		return DistributionSlot{}, ErrSlotNumberRequired
	}
	if err := validateSignaturePNG(input.ImagePNG); err != nil {
		return DistributionSlot{}, err
	}
	if s.signatureRepository == nil {
		return DistributionSlot{}, ErrSlotNotFound
	}
	return s.signatureRepository.SaveRecipientSignature(ctx, actor, input, meta, scope)
}

func (s *Service) DeleteRecipientSignature(ctx context.Context, actor auth.Principal, scheduleID string, slotNumber int, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionSlot, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if scheduleID == "" {
		return DistributionSlot{}, ErrScheduleRequired
	}
	if slotNumber < 1 {
		return DistributionSlot{}, ErrSlotNumberRequired
	}
	if s.signatureRepository == nil {
		return DistributionSlot{}, ErrSlotNotFound
	}
	return s.signatureRepository.DeleteRecipientSignature(ctx, actor, scheduleID, slotNumber, meta, scope)
}

func (s *Service) OpenRecipientSignature(ctx context.Context, scheduleID string, slotNumber int, scope auth.RegencyScope) (SignatureContent, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if scheduleID == "" {
		return SignatureContent{}, ErrScheduleRequired
	}
	if s.signatureRepository == nil {
		return SignatureContent{}, ErrSlotNotFound
	}
	return s.signatureRepository.OpenRecipientSignature(ctx, scheduleID, slotNumber, scope)
}

func validateSignaturePNG(data []byte) error {
	if len(data) == 0 || len(data) > maxSignatureBytes {
		return ErrSignatureInvalid
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return ErrSignatureInvalid
	}
	if config.Width < minSignatureSide || config.Height < minSignatureSide || config.Width > maxSignatureSide || config.Height > maxSignatureSide {
		return ErrSignatureInvalid
	}
	return nil
}

// lockLinkedSlot locks a slot that may still be edited in POS Dokumen and
// returns its id and current recipient.
func lockLinkedSlot(ctx context.Context, tx pgx.Tx, scheduleID string, slotNumber int, scope auth.RegencyScope) (string, string, error) {
	var slotID, status string
	var personID *string
	err := tx.QueryRow(ctx, `
		SELECT ds.id::text, ds.status, ds.recipient_person_id::text
		FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id=ds.schedule_id
		WHERE ds.schedule_id=$1 AND ds.slot_number=$2 AND ($3 OR ps.regency_id::text=ANY($4))
		FOR UPDATE OF ds
	`, scheduleID, slotNumber, scope.Unrestricted, scope.RegencyIDs).Scan(&slotID, &status, &personID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrSlotNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("lock signature slot: %w", err)
	}
	if status == "completed" {
		return "", "", ErrAlreadyCompleted
	}
	if status != "linked" || personID == nil {
		return "", "", ErrRecipientNotLinked
	}
	return slotID, *personID, nil
}

func (r *Repository) SaveRecipientSignature(ctx context.Context, actor auth.Principal, input SaveSignatureInput, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionSlot, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("begin signature save: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	slotID, personID, err := lockLinkedSlot(ctx, tx, input.ScheduleID, input.SlotNumber, scope)
	if err != nil {
		return DistributionSlot{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO distribution_slot_signatures(distribution_slot_id,person_id,image_png,byte_size,signed_by,signed_at)
		VALUES($1,$2,$3,$4,NULLIF($5,'')::uuid,now())
		ON CONFLICT (distribution_slot_id) DO UPDATE SET person_id=EXCLUDED.person_id,image_png=EXCLUDED.image_png,
			byte_size=EXCLUDED.byte_size,signed_by=EXCLUDED.signed_by,signed_at=now()
	`, slotID, personID, input.ImagePNG, len(input.ImagePNG), actor.UserID); err != nil {
		return DistributionSlot{}, fmt.Errorf("save recipient signature: %w", err)
	}
	// Bumps the slot for delta sync and the realtime signal.
	if _, err := tx.Exec(ctx, `UPDATE distribution_slots SET updated_at=now() WHERE id=$1`, slotID); err != nil {
		return DistributionSlot{}, fmt.Errorf("touch signed slot: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "distribution.signature_saved", ResourceType: "distribution_slot", ResourceID: slotID, Metadata: map[string]any{"slot_number": input.SlotNumber, "person_id": personID, "byte_size": len(input.ImagePNG)}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return DistributionSlot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DistributionSlot{}, fmt.Errorf("commit signature save: %w", err)
	}
	return r.getSlotByID(ctx, slotID)
}

func (r *Repository) DeleteRecipientSignature(ctx context.Context, actor auth.Principal, scheduleID string, slotNumber int, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionSlot, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("begin signature delete: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	slotID, personID, err := lockLinkedSlot(ctx, tx, scheduleID, slotNumber, scope)
	if err != nil {
		return DistributionSlot{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM distribution_slot_signatures WHERE distribution_slot_id=$1`, slotID); err != nil {
		return DistributionSlot{}, fmt.Errorf("delete recipient signature: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE distribution_slots SET updated_at=now() WHERE id=$1`, slotID); err != nil {
		return DistributionSlot{}, fmt.Errorf("touch unsigned slot: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "distribution.signature_deleted", ResourceType: "distribution_slot", ResourceID: slotID, Metadata: map[string]any{"slot_number": slotNumber, "person_id": personID}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return DistributionSlot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DistributionSlot{}, fmt.Errorf("commit signature delete: %w", err)
	}
	return r.getSlotByID(ctx, slotID)
}

func (r *Repository) OpenRecipientSignature(ctx context.Context, scheduleID string, slotNumber int, scope auth.RegencyScope) (SignatureContent, error) {
	var content SignatureContent
	err := r.pool.QueryRow(ctx, `
		SELECT sig.image_png, sig.signed_at
		FROM distribution_slot_signatures sig
		`+signedRecipientJoin+`
		JOIN program_schedules ps ON ps.id=sig_ds.schedule_id
		WHERE sig_ds.schedule_id=$1 AND sig_ds.slot_number=$2 AND ($3 OR ps.regency_id::text=ANY($4))
	`, scheduleID, slotNumber, scope.Unrestricted, scope.RegencyIDs).Scan(&content.ImagePNG, &content.SignedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return SignatureContent{}, ErrSlotNotFound
	}
	if err != nil {
		return SignatureContent{}, fmt.Errorf("open recipient signature: %w", err)
	}
	return content, nil
}

// recipientSignedAt is when the current recipient signed, or nil.
func (r *Repository) recipientSignedAt(ctx context.Context, slotID string) (*time.Time, error) {
	var signedAt time.Time
	err := r.pool.QueryRow(ctx, `SELECT sig.signed_at FROM distribution_slot_signatures sig `+signedRecipientJoin+` WHERE sig.distribution_slot_id=$1`, slotID).Scan(&signedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read recipient signature: %w", err)
	}
	return &signedAt, nil
}
