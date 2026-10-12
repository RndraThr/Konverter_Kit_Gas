package distribution

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"konkit/internal/auth"
)

func signaturePNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for x := 5; x < width-5; x++ {
		img.Set(x, height/2, color.NRGBA{R: 20, G: 30, B: 90, A: 255})
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestValidateSignaturePNG(t *testing.T) {
	if err := validateSignaturePNG(signaturePNG(t, 600, 240)); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	for name, data := range map[string][]byte{
		"empty":     nil,
		"not a png": []byte("\xff\xd8\xff\xe0 jpeg"),
		"too small": signaturePNG(t, 20, 20),
		"too large": make([]byte, maxSignatureBytes+1),
	} {
		if err := validateSignaturePNG(data); !errors.Is(err, ErrSignatureInvalid) {
			t.Errorf("%s: err=%v, want ErrSignatureInvalid", name, err)
		}
	}
}

// The recipient signs in POS Dokumen; completion waits for it, and a signature
// of a replaced recipient no longer counts.
func TestIntegrationRecipientSignature(t *testing.T) {
	pool := distributionIntegrationPool(t)
	ctx := context.Background()
	fixture := seedMediaFixture(t, pool)
	nik := addCandidateForMediaFixture(t, pool, fixture)
	repo := NewRepository(pool)
	service := NewService(repo)
	scope := auth.RegencyScope{Unrestricted: true}
	image := signaturePNG(t, 600, 240)
	input := SaveSignatureInput{ScheduleID: fixture.scheduleID, SlotNumber: fixture.slotNumber, ImagePNG: image}

	if _, err := service.SaveRecipientSignature(ctx, auth.Principal{}, input, auth.ClientMeta{}, scope); !errors.Is(err, ErrRecipientNotLinked) {
		t.Fatalf("open slot err=%v, want ErrRecipientNotLinked", err)
	}
	if _, err := repo.SetDistributionDate(ctx, auth.Principal{}, SetDistributionDateInput{ScheduleID: fixture.scheduleID, SlotNumber: fixture.slotNumber, DistributionDate: "2026-10-20"}, auth.ClientMeta{}, scope); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.LinkSlot(ctx, auth.Principal{}, LinkSlotInput{ScheduleID: fixture.scheduleID, SlotNumber: fixture.slotNumber, NIK: nik, SectorIdentifier: fmt.Sprintf("KPSIG%d", time.Now().UnixNano())}, auth.ClientMeta{}, scope); err != nil {
		t.Fatal(err)
	}
	// The photo already sits in its final folder (no move worker in tests).
	must(t, func() error {
		_, err := pool.Exec(ctx, `UPDATE media_files SET storage_state='final' WHERE id=$1`, fixture.mediaID)
		return err
	}())
	if _, err := repo.CompleteSlot(ctx, auth.Principal{}, CompleteSlotInput{ScheduleID: fixture.scheduleID, SlotNumber: fixture.slotNumber}, auth.ClientMeta{}, scope); !errors.Is(err, ErrSignatureRequired) {
		t.Fatalf("unsigned completion err=%v, want ErrSignatureRequired", err)
	}

	slot, err := service.SaveRecipientSignature(ctx, auth.Principal{}, input, auth.ClientMeta{}, scope)
	if err != nil {
		t.Fatal(err)
	}
	if slot.RecipientSignedAt == nil {
		t.Fatal("slot does not report the signature")
	}
	content, err := service.OpenRecipientSignature(ctx, fixture.scheduleID, fixture.slotNumber, scope)
	if err != nil || !bytes.Equal(content.ImagePNG, image) {
		t.Fatalf("open signature err=%v same=%v", err, bytes.Equal(content.ImagePNG, image))
	}
	if _, err := repo.CompleteSlot(ctx, auth.Principal{}, CompleteSlotInput{ScheduleID: fixture.scheduleID, SlotNumber: fixture.slotNumber}, auth.ClientMeta{}, scope); errors.Is(err, ErrSignatureRequired) {
		t.Fatal("signed slot still reports a missing signature")
	}

	// Another person on the slot: the old signature is ignored.
	must(t, func() error {
		_, err := pool.Exec(ctx, `UPDATE distribution_slots SET recipient_person_id=(SELECT id FROM people WHERE id<>recipient_person_id LIMIT 1) WHERE id=$1`, fixture.distributionSlotID)
		return err
	}())
	if slot, err = repo.getSlotByID(ctx, fixture.distributionSlotID); err != nil || slot.RecipientSignedAt != nil {
		t.Fatalf("replaced recipient still signed: err=%v signedAt=%v", err, slot.RecipientSignedAt)
	}
	if _, err := service.OpenRecipientSignature(ctx, fixture.scheduleID, fixture.slotNumber, scope); !errors.Is(err, ErrSlotNotFound) {
		t.Fatalf("old signature still served: err=%v", err)
	}
}
