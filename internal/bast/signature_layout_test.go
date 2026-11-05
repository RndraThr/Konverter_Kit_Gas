package bast

import (
	"context"
	"errors"
	"math"
	"testing"

	"konkit/internal/auth"
)

// Editor tanda tangan menerima letak keempat kotak tanda tangan Closing pada
// halaman tanda tangan, dan PDF-nya tanpa gambar tanda tangan/stempel.
func TestSignatureLayoutRecordsBoxesWithoutMarks(t *testing.T) {
	images := map[string][]byte{"logo-1": testPNG(t)}
	previewer := PreviewFunc(func(ctx context.Context, scheduleID, date string, _ auth.RegencyScope) (AggregatePreview, error) {
		snapshot := sampleClosingSnapshot()
		marks, _, err := loadSignatoryMarks(ctx, nil, scheduleID, "closing-titik-serah")
		if err != nil {
			return AggregatePreview{}, err
		}
		snapshot.SignatureMarks = marks
		rendered, err := RenderClosingTitikSerah(snapshot, images)
		return AggregatePreview{PDF: rendered.PDF, PageCount: rendered.PageCount}, err
	})
	service := NewSignatureLayoutService(map[string]WetPreviewer{"closing-titik-serah": previewer})
	layout, err := service.Layout(context.Background(), "closing-titik-serah", "schedule-1", "2026-10-12", auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(layout.Boxes) != 4 || layout.Boxes[0].Role != SignatoryAgricultureOffice || layout.Boxes[3].Role != SignatoryPertamina {
		t.Fatalf("boxes=%+v", layout.Boxes)
	}
	if layout.Page != 2 || math.Abs(layout.PageWidth-210) > 0.5 || len(layout.PDF) == 0 {
		t.Fatalf("page=%d width=%v", layout.Page, layout.PageWidth)
	}
	first := layout.Boxes[0]
	if first.H != activitySignatureSpace || math.Abs(layout.Boxes[1].X-(first.X+first.W)) > 0.01 {
		t.Fatalf("unexpected geometry %+v", layout.Boxes)
	}
	if _, err := service.Layout(context.Background(), "rakorda", "schedule-1", "2026-10-12", auth.RegencyScope{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unsupported document err=%v", err)
	}
}

func TestPlacedMarkHasTheSameSizeInEveryBA(t *testing.T) {
	stamp := MarkPlacement{X: -0.4, Y: 0.1, Size: 0.78}
	// Kotak pendek (15,5 mm) dan BA lain (kotak 28 mm), kolom sama lebar.
	smallW, smallH, smallX, smallY := placedGeometry(stamp, 1, 100, 50, 64, 15.5, markUnitMM)
	tallW, tallH, tallX, tallY := placedGeometry(stamp, 1, 100, 50, 64, 28, markUnitMM)
	if smallW != tallW || smallH != tallH || math.Abs(smallW-0.78*markUnitMM) > 1e-9 {
		t.Fatalf("size %v x %v vs %v x %v", smallW, smallH, tallW, tallH)
	}
	if smallX != tallX {
		t.Fatalf("x %v vs %v", smallX, tallX)
	}
	if math.Abs((smallY-(50+7.75))-(tallY-(50+14))) > 1e-9 {
		t.Fatalf("y offset %v vs %v", smallY-(50+7.75), tallY-(50+14))
	}
	// The centre stays in the box.
	if _, _, _, cy := placedGeometry(MarkPlacement{Y: 0.5, Size: 0.5}, 1, 0, 0, 60, 15.5, markUnitMM); cy != 15.5 {
		t.Fatalf("cy=%v", cy)
	}
}

func TestBAPeroranganPrintsMarksSlightlySmaller(t *testing.T) {
	stamp := MarkPlacement{Size: 0.78}
	other, _, _, _ := placedGeometry(stamp, 1, 0, 0, 64, 28, markUnitMM)
	perorangan, _, _, _ := placedGeometry(stamp, 1, 0, 0, 64, 28, markUnitMM*peroranganMarkScale)
	if math.Abs(perorangan-other*peroranganMarkScale) > 1e-9 || perorangan >= other {
		t.Fatalf("perorangan %v vs other %v", perorangan, other)
	}
}
