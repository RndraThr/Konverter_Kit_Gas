package bast

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"konkit/internal/auth"
)

type signatoryRepositoryStub struct {
	saved    SignatureRules
	override bool
	asset    []byte
	name     string
	nip      string
}

func (s *signatoryRepositoryStub) GetSignatureSettings(context.Context, string, string, auth.RegencyScope) (SignatureSettingsView, error) {
	return SignatureSettingsView{Override: s.override, Rules: s.saved}, nil
}
func (s *signatoryRepositoryStub) SaveSignatureRules(_ context.Context, _ auth.Principal, _, _ string, override bool, rules SignatureRules, _ bool, _ auth.RegencyScope, _ auth.ClientMeta) error {
	s.saved, s.override = rules, override
	return nil
}
func (s *signatoryRepositoryStub) SaveSignatoryName(_ context.Context, _ auth.Principal, _, _, name, nip string, _ auth.RegencyScope, _ auth.ClientMeta) error {
	s.name, s.nip = name, nip
	return nil
}
func (s *signatoryRepositoryStub) PutSignatoryAsset(_ context.Context, _ auth.Principal, _, _, _ string, image []byte, _ auth.RegencyScope, _ auth.ClientMeta) error {
	s.asset = image
	return nil
}
func (s *signatoryRepositoryStub) DeleteSignatoryAsset(context.Context, auth.Principal, string, string, string, auth.RegencyScope, auth.ClientMeta) error {
	return nil
}
func (s *signatoryRepositoryStub) OpenSignatoryAsset(context.Context, string, string, string, auth.RegencyScope) ([]byte, error) {
	return s.asset, nil
}

func TestSignatoryServiceValidatesInput(t *testing.T) {
	repository := &signatoryRepositoryStub{}
	service := NewSignatoryService(repository)
	ctx, scope, actor, meta := context.Background(), auth.RegencyScope{Unrestricted: true}, auth.Principal{}, auth.ClientMeta{}

	if _, err := service.Settings(ctx, "schedule-1", "rakorda", scope); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("RAKORDA has no signature table, err=%v", err)
	}
	if _, err := service.SaveRules(ctx, actor, "schedule-1", "dp3", true, SignatureRules{"penerima": {Signature: true}}, false, scope, meta); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown role err=%v", err)
	}
	if _, err := service.SaveRules(ctx, actor, "schedule-1", "dp3", true, SignatureRules{SignatoryInstaller: {Signature: true, Stamp: true}}, false, scope, meta); err != nil || !repository.override || !repository.saved[SignatoryInstaller].Stamp {
		t.Fatalf("save rules err=%v override=%v rules=%v", err, repository.override, repository.saved)
	}
	if err := service.SaveName(ctx, actor, "schedule-1", SignatoryAgricultureOffice, "  ir.  haryanto ", " 1972 ", scope, meta); err != nil || repository.name != "IR. HARYANTO" || repository.nip != "1972" {
		t.Fatalf("save name err=%v name=%q nip=%q", err, repository.name, repository.nip)
	}
	if err := service.PutAsset(ctx, actor, "schedule-1", SignatoryInstaller, SignatoryKindStamp, []byte("not a png"), scope, meta); !errors.Is(err, ErrSignatoryImageInvalid) {
		t.Fatalf("non-PNG err=%v", err)
	}
	if err := service.PutAsset(ctx, actor, "schedule-1", "penerima", SignatoryKindStamp, testPNG(t), scope, meta); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown role err=%v", err)
	}
}

// PNG 16-bit (umum dari aplikasi desain) disimpan ulang 8-bit agar fpdf dapat
// memuatnya; tanpa ini stempel terlewat diam-diam saat dicetak.
func TestPutAssetStoresEightBitPNG(t *testing.T) {
	repository := &signatoryRepositoryStub{}
	service := NewSignatoryService(repository)
	deep := image.NewRGBA64(image.Rect(0, 0, 8, 8))
	deep.Set(2, 2, color.RGBA64{R: 0xffff, A: 0xffff})
	var source bytes.Buffer
	if err := png.Encode(&source, deep); err != nil {
		t.Fatal(err)
	}
	if err := service.PutAsset(context.Background(), auth.Principal{}, "schedule-1", SignatoryInstaller, SignatoryKindStamp, source.Bytes(), auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{}); err != nil {
		t.Fatal(err)
	}
	stored, err := png.Decode(bytes.NewReader(repository.asset))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := stored.(*image.NRGBA); !ok {
		t.Fatalf("stored image type %T, want 8-bit NRGBA", stored)
	}
}

// Tanda tangan dan stempel ikut tercetak di kotak tanda tangan; stempel
// memakai mode campur Multiply dan diputar.
func TestRenderClosingDrawsSignatoryMarks(t *testing.T) {
	images := map[string][]byte{
		"logo-1": testPNG(t),
		signatoryImageKey(SignatoryInstaller, SignatoryKindSignature): testPNG(t),
		signatoryImageKey(SignatoryInstaller, SignatoryKindStamp):     testPNG(t),
	}
	plain, err := RenderClosingTitikSerah(sampleClosingSnapshot(), images)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := sampleClosingSnapshot()
	snapshot.SignatureMarks = SignatureMarks{SignatoryInstaller: {Signature: true, Stamp: true}}
	marked, err := RenderClosingTitikSerah(snapshot, images)
	if err != nil {
		t.Fatal(err)
	}
	plainText, markedText := decodedPDFStreams(t, plain.PDF), decodedPDFStreams(t, marked.PDF)
	// decodedPDFStreams memuat tiga salinan teks stream, jadi dua gambar
	// tambahan (tanda tangan + stempel) menambah enam operator Do.
	if got := strings.Count(markedText, " Do") - strings.Count(plainText, " Do"); got != 6 {
		t.Fatalf("extra image draws=%d, want 6 (signature + stamp)", got)
	}
	if !strings.Contains(string(marked.PDF), "/Multiply") {
		t.Fatal("stamp must use the Multiply blend mode")
	}
}

func TestDefaultSignatureRulesKeepExternalOfficialsOff(t *testing.T) {
	rules := defaultSignatureRules()
	if rules[SignatoryAgricultureOffice].Signature || rules[SignatoryPertamina].Signature || !rules[SignatoryInstaller].Signature {
		t.Fatalf("rules=%+v", rules)
	}
	if signatureDocuments["dp3"][0] != SignatoryAgricultureOffice || len(signatureDocuments["servis-berkala"]) != 2 {
		t.Fatal("unexpected document roles")
	}
}
