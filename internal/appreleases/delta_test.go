package appreleases

import (
	"bytes"
	"math/rand"
	"os"
	"testing"
	"time"
)

func roundTrip(t *testing.T, oldData, newData []byte) []byte {
	t.Helper()
	var patch bytes.Buffer
	if err := Diff(oldData, newData, &patch); err != nil {
		t.Fatalf("diff: %v", err)
	}
	result := append([]byte{}, patch.Bytes()...)
	got, err := ApplyPatch(oldData, &patch)
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if !bytes.Equal(got, newData) {
		t.Fatalf("patched output differs (got %d bytes, want %d)", len(got), len(newData))
	}
	return result
}

func TestDeltaRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	random := func(n int) []byte {
		b := make([]byte, n)
		rng.Read(b)
		return b
	}
	base := random(200_000)
	// Edits, an insertion, a deletion and a moved block.
	edited := append([]byte{}, base[:50_000]...)
	edited = append(edited, random(3_000)...)
	edited = append(edited, base[60_000:120_000]...)
	edited = append(edited, base[150_000:]...)
	edited = append(edited, base[10_000:20_000]...)
	for i := 0; i < 200; i++ {
		edited[rng.Intn(len(edited))] ^= byte(rng.Intn(255) + 1)
	}
	cases := []struct {
		name     string
		old, new []byte
	}{
		{"empty", nil, nil},
		{"from empty", nil, random(1000)},
		{"to empty", random(1000), nil},
		{"same", base, base},
		{"edited", base, edited},
		{"unrelated", random(5000), random(7000)},
		{"zeros", make([]byte, 100_000), append(make([]byte, 50_000), random(100)...)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			size := len(roundTrip(t, c.old, c.new))
			if c.name == "edited" && size > 30_000 {
				t.Fatalf("patch for a lightly edited file is %d bytes", size)
			}
		})
	}
}

func TestPatchRejectsGarbage(t *testing.T) {
	if _, err := ApplyPatch([]byte("old"), bytes.NewReader([]byte("not a patch at all"))); err == nil {
		t.Fatal("garbage was accepted")
	}
}

// KONKIT_DELTA_OLD / KONKIT_DELTA_NEW: two real APKs, to measure a patch
// (KONKIT_DELTA_OUT: also save it, e.g. to check the app applies it).
func TestDeltaRealAPKs(t *testing.T) {
	oldPath, newPath := os.Getenv("KONKIT_DELTA_OLD"), os.Getenv("KONKIT_DELTA_NEW")
	if oldPath == "" || newPath == "" {
		t.Skip("set KONKIT_DELTA_OLD and KONKIT_DELTA_NEW")
	}
	oldData, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	newData, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	patch := roundTrip(t, oldData, newData)
	size := len(patch)
	if out := os.Getenv("KONKIT_DELTA_OUT"); out != "" {
		if err := os.WriteFile(out, patch, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("old %d, new %d, patch %d bytes (%.1f%%) in %s", len(oldData), len(newData), size, 100*float64(size)/float64(len(newData)), time.Since(start))
}
