package activities

import "testing"

// Every documentation type except the supporting documents has its own Drive folder.
func TestEveryActivityTypeHasAFolder(t *testing.T) {
	for code := range activityTypeCodes {
		if code == "dokumen_pendukung" {
			continue
		}
		if activityTypeFolderNames[code] == "" {
			t.Errorf("%s has no folder name", code)
		}
	}
	for _, code := range []string{"kunjungan_dinas", "gudang_titik_serah", "perakitan_mesin", "pemeriksaan_barang", "kegiatan_distribusi", "closing_kabupaten"} {
		if !isValidActivityType(code) {
			t.Errorf("%s is not accepted", code)
		}
	}
}
