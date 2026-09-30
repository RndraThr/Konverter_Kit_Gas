package media

import (
	"errors"
	"testing"
)

func TestBuildFolderPathFarmerPhotosExample(t *testing.T) {
	got, err := BuildFolderPath(FolderPathInput{
		ProgramType: "farmer",
		ZoneName:    "Zona 1",
		RegencyName: "Kabupaten Wajo",
		Category:    FolderPhotos,
		Child:       "RAKOR",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"PETANI", "ZONA 1", "KABUPATEN WAJO", "DOKUMENTASI (FOTO)", "RAKOR"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("segment %d: got %q, want %q (full: got=%v want=%v)", i, got[i], want[i], got, want)
		}
	}
}

func TestBuildFolderPathFishermanMapsToNelayan(t *testing.T) {
	got, err := BuildFolderPath(FolderPathInput{
		ProgramType: "fisherman",
		ZoneName:    "Zona 2",
		RegencyName: "Kabupaten Bone",
		Category:    FolderBA,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"NELAYAN", "ZONA 2", "KABUPATEN BONE", "BERITA ACARA (BA)"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("segment %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestBuildFolderPathOmitsChildWhenEmpty(t *testing.T) {
	got, err := BuildFolderPath(FolderPathInput{
		ProgramType: "farmer",
		ZoneName:    "Zona 1",
		RegencyName: "Wajo",
		Category:    FolderSupporting,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"PETANI", "ZONA 1", "WAJO", "DOKUMEN PENDUKUNG"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestBuildFolderPathRejectsUnknownProgramType(t *testing.T) {
	_, err := BuildFolderPath(FolderPathInput{
		ProgramType: "other",
		ZoneName:    "Zona 1",
		RegencyName: "Wajo",
		Category:    FolderPhotos,
	})
	if !errors.Is(err, ErrInvalidFolderPath) {
		t.Fatalf("err=%v, want ErrInvalidFolderPath", err)
	}
}

func TestBuildFolderPathRejectsEmptyRequiredFields(t *testing.T) {
	base := FolderPathInput{ProgramType: "farmer", ZoneName: "Zona 1", RegencyName: "Wajo", Category: FolderPhotos}

	cases := []FolderPathInput{
		{ProgramType: "", ZoneName: base.ZoneName, RegencyName: base.RegencyName, Category: base.Category},
		{ProgramType: base.ProgramType, ZoneName: "", RegencyName: base.RegencyName, Category: base.Category},
		{ProgramType: base.ProgramType, ZoneName: base.ZoneName, RegencyName: "", Category: base.Category},
		{ProgramType: base.ProgramType, ZoneName: base.ZoneName, RegencyName: base.RegencyName, Category: ""},
		{ProgramType: "   ", ZoneName: base.ZoneName, RegencyName: base.RegencyName, Category: base.Category},
	}
	for i, c := range cases {
		if _, err := BuildFolderPath(c); !errors.Is(err, ErrInvalidFolderPath) {
			t.Fatalf("case %d: err=%v, want ErrInvalidFolderPath", i, err)
		}
	}
}

func TestBuildFolderPathRejectsPathSeparatorsInsteadOfSplitting(t *testing.T) {
	cases := []FolderPathInput{
		{ProgramType: "farmer", ZoneName: `Zona / Timur\A`, RegencyName: "Wajo", Category: FolderPhotos},
		{ProgramType: "farmer", ZoneName: "Zona 1", RegencyName: `Kabupaten\Wajo`, Category: FolderPhotos},
		{ProgramType: "farmer", ZoneName: "Zona 1", RegencyName: "Wajo", Category: FolderPhotos, Child: "Rakor/Foto"},
		{ProgramType: "farmer", ZoneName: "Zona 1", RegencyName: "Wajo", Category: FolderPhotos, Child: "Rakor\x00Foto"},
	}
	for i, c := range cases {
		if _, err := BuildFolderPath(c); !errors.Is(err, ErrInvalidFolderPath) {
			t.Fatalf("case %d: err=%v, want ErrInvalidFolderPath", i, err)
		}
	}
}

func TestBuildFolderPathTrimsAndCollapsesWhitespace(t *testing.T) {
	got, err := BuildFolderPath(FolderPathInput{
		ProgramType: "farmer",
		ZoneName:    "  Zona   1  ",
		RegencyName: "Kabupaten   Wajo",
		Category:    FolderPhotos,
		Child:       "  RAKOR  Umum ",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"PETANI", "ZONA 1", "KABUPATEN WAJO", "DOKUMENTASI (FOTO)", "RAKOR UMUM"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("segment %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestBuildFolderPathPreservesParenthesesAndSpacesInCategory(t *testing.T) {
	got, err := BuildFolderPath(FolderPathInput{
		ProgramType: "farmer",
		ZoneName:    "Zona 1",
		RegencyName: "Wajo",
		Category:    FolderBA,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got[len(got)-1] != "BERITA ACARA (BA)" {
		t.Fatalf("category segment = %q, want %q", got[len(got)-1], "BERITA ACARA (BA)")
	}
}
