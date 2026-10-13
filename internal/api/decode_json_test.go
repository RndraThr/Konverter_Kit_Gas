package api

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Gambar base64 di atas 1 MB (pindaian tanda tangan basah, stempel) harus
// diterima route dengan batasnya sendiri, dan body yang melebihi batas
// dijawab 413, bukan "JSON tidak valid".
func TestDecodeJSONLimitAcceptsLargeImagesAndReportsTooLarge(t *testing.T) {
	image := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 2<<20))
	body := []byte(`{"image_png":"` + image + `"}`)
	request := func() *http.Request {
		r := httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		return r
	}
	var input struct {
		ImagePNG []byte `json:"image_png"`
	}

	recorder := httptest.NewRecorder()
	if !decodeJSONLimit(recorder, request(), &input, 12<<20) || len(input.ImagePNG) != 2<<20 {
		t.Fatalf("large image rejected: status=%d len=%d", recorder.Code, len(input.ImagePNG))
	}
	recorder = httptest.NewRecorder()
	if decodeJSON(recorder, request(), &input) || recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("default limit status=%d, want 413", recorder.Code)
	}
}
