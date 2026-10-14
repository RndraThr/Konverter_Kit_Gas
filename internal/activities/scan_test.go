package activities

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"strings"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/programs"
)

func fakeJPEGPage(t *testing.T) *bytes.Reader {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 100, 140))
	for y := 0; y < 140; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.White)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode fake jpeg: %v", err)
	}
	return bytes.NewReader(buf.Bytes())
}

func TestScanUploadRejectsZeroPages(t *testing.T) {
	service := NewService(&repositoryStub{}, &storageStub{})
	_, err := service.ScanUpload(context.Background(), auth.Principal{}, ScanUploadInput{
		ProgramID: "program-1", RegencyID: "regency-1", SlotID: "slot-1", Source: "camera", Pages: nil,
	}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if !errors.Is(err, ErrScanPagesRequired) {
		t.Fatalf("err = %v, want ErrScanPagesRequired", err)
	}
}

func TestScanUploadRejectsTooManyPages(t *testing.T) {
	service := NewService(&repositoryStub{}, &storageStub{})
	pages := make([]io.Reader, 0, maxScanPages+1)
	for i := 0; i < maxScanPages+1; i++ {
		pages = append(pages, fakeJPEGPage(t))
	}
	_, err := service.ScanUpload(context.Background(), auth.Principal{}, ScanUploadInput{
		ProgramID: "program-1", RegencyID: "regency-1", SlotID: "slot-1", Source: "camera", Pages: pages,
	}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if !errors.Is(err, ErrScanTooManyPages) {
		t.Fatalf("err = %v, want ErrScanTooManyPages", err)
	}
}

func TestScanUploadAssemblesPagesIntoOnePDF(t *testing.T) {
	repository := &repositoryStub{
		regency:     regencyInfo{ID: "regency-1", Name: "Wajo", DocumentCode: "WJO"},
		slotsResult: []SupportingDocumentSlot{{ID: "slot-1", RegencyID: "regency-1", Name: "Surat Tugas", IsActive: true}},
	}
	storage := &storageStub{}
	resolver := &programContextResolverStub{
		context: programs.StorageContext{ProgramID: "program-1", ProgramType: programs.ProgramFarmer, ZoneName: "Zona 1", RegencyID: "regency-1", RegencyName: "Wajo"},
	}
	service := NewService(repository, storage, resolver)

	pages := []io.Reader{fakeJPEGPage(t), fakeJPEGPage(t), fakeJPEGPage(t)}
	_, err := service.ScanUpload(context.Background(), auth.Principal{}, ScanUploadInput{
		ProgramID: "program-1", RegencyID: "regency-1", SlotID: "slot-1", Source: "camera", Pages: pages,
	}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if repository.insertInput.MediaType != "document" {
		t.Fatalf("MediaType = %q, want document", repository.insertInput.MediaType)
	}
	if repository.insertInput.MimeType != "application/pdf" {
		t.Fatalf("MimeType = %q, want application/pdf", repository.insertInput.MimeType)
	}
	if repository.insertInput.ByteSize <= 0 {
		t.Fatalf("ByteSize = %d, want > 0", repository.insertInput.ByteSize)
	}
	if repository.insertInput.ActivityType != "dokumen_pendukung" {
		t.Fatalf("ActivityType = %q, want dokumen_pendukung", repository.insertInput.ActivityType)
	}
	if !strings.Contains(repository.insertInput.DisplayName, "SURAT-TUGAS") {
		t.Fatalf("DisplayName = %q, want to contain SURAT-TUGAS", repository.insertInput.DisplayName)
	}
}

func TestScanUploadAcceptsCustomNameWithoutASlot(t *testing.T) {
	repository := &repositoryStub{regency: regencyInfo{ID: "regency-1", Name: "Wajo", DocumentCode: "WJO"}}
	storage := &storageStub{}
	resolver := &programContextResolverStub{
		context: programs.StorageContext{ProgramID: "program-1", ProgramType: programs.ProgramFarmer, ZoneName: "Zona 1", RegencyID: "regency-1", RegencyName: "Wajo"},
	}
	service := NewService(repository, storage, resolver)

	_, err := service.ScanUpload(context.Background(), auth.Principal{}, ScanUploadInput{
		ProgramID: "program-1", RegencyID: "regency-1", CustomName: "Surat Keterangan Domisili", Source: "camera",
		Pages: []io.Reader{fakeJPEGPage(t)},
	}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(repository.insertInput.DisplayName, "SURAT-KETERANGAN-DOMISILI") {
		t.Fatalf("DisplayName = %q, want to contain SURAT-KETERANGAN-DOMISILI", repository.insertInput.DisplayName)
	}
}

func TestScanUploadRejectsWhenNeitherSlotNorCustomNameGiven(t *testing.T) {
	repository := &repositoryStub{regency: regencyInfo{ID: "regency-1", Name: "Wajo", DocumentCode: "WJO"}}
	storage := &storageStub{}
	resolver := &programContextResolverStub{
		context: programs.StorageContext{ProgramID: "program-1", ProgramType: programs.ProgramFarmer, ZoneName: "Zona 1", RegencyID: "regency-1", RegencyName: "Wajo"},
	}
	service := NewService(repository, storage, resolver)

	_, err := service.ScanUpload(context.Background(), auth.Principal{}, ScanUploadInput{
		ProgramID: "program-1", RegencyID: "regency-1", Source: "camera", Pages: []io.Reader{fakeJPEGPage(t)},
	}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if !errors.Is(err, ErrDocumentNameRequired) {
		t.Fatalf("err = %v, want ErrDocumentNameRequired", err)
	}
}

func TestScanUploadRejectsInvalidSlot(t *testing.T) {
	repository := &repositoryStub{
		regency:     regencyInfo{ID: "regency-1", Name: "Wajo", DocumentCode: "WJO"},
		slotsResult: []SupportingDocumentSlot{{ID: "slot-1", RegencyID: "regency-1", Name: "Surat Tugas", IsActive: true}},
	}
	storage := &storageStub{}
	resolver := &programContextResolverStub{
		context: programs.StorageContext{ProgramID: "program-1", ProgramType: programs.ProgramFarmer, ZoneName: "Zona 1", RegencyID: "regency-1", RegencyName: "Wajo"},
	}
	service := NewService(repository, storage, resolver)

	_, err := service.ScanUpload(context.Background(), auth.Principal{}, ScanUploadInput{
		ProgramID: "program-1", RegencyID: "regency-1", SlotID: "no-such-slot", Source: "camera", Pages: []io.Reader{fakeJPEGPage(t)},
	}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if !errors.Is(err, ErrSlotInvalid) {
		t.Fatalf("err = %v, want ErrSlotInvalid", err)
	}
}

func TestScanUploadRejectsNonImagePage(t *testing.T) {
	repository := &repositoryStub{
		regency:     regencyInfo{ID: "regency-1", Name: "Wajo", DocumentCode: "WJO"},
		slotsResult: []SupportingDocumentSlot{{ID: "slot-1", RegencyID: "regency-1", Name: "Surat Tugas", IsActive: true}},
	}
	storage := &storageStub{}
	resolver := &programContextResolverStub{
		context: programs.StorageContext{ProgramID: "program-1", ProgramType: programs.ProgramFarmer, ZoneName: "Zona 1", RegencyID: "regency-1", RegencyName: "Wajo"},
	}
	service := NewService(repository, storage, resolver)

	pdfPage := bytes.NewReader([]byte("%PDF-1.4\n%fake pdf content for test\n"))
	_, err := service.ScanUpload(context.Background(), auth.Principal{}, ScanUploadInput{
		ProgramID: "program-1", RegencyID: "regency-1", SlotID: "slot-1", Source: "camera", Pages: []io.Reader{pdfPage},
	}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if !errors.Is(err, ErrMediaTypeInvalid) {
		t.Fatalf("err = %v, want ErrMediaTypeInvalid", err)
	}
}
