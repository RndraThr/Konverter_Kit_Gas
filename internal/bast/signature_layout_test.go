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
