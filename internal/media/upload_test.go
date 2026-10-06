package media

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"testing"
)

type shortReader struct {
	data  []byte
	off   int
	reads int
}

func (r *shortReader) Read(p []byte) (int, error) {
	r.reads++
	if r.off == len(r.data) {
		return 0, io.EOF
	}
	if len(p) > 37 {
		p = p[:37]
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	return n, nil
}

func TestDetectUploadRecognizesContentAndPreservesEveryByte(t *testing.T) {
	tests := []struct {
		name, filename, mime, extension string
		kind                            Kind
		data                            []byte
	}{
		{name: "jpeg", filename: "camera.jpg", mime: "image/jpeg", extension: ".jpg", kind: KindImage, data: append([]byte{0xff, 0xd8, 0xff, 0xdb}, bytes.Repeat([]byte{0x01}, 900)...)},
		{name: "png", filename: "camera.png", mime: "image/png", extension: ".png", kind: KindImage, data: append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0x01}, 900)...)},
		{name: "webp", filename: "camera.webp", mime: "image/webp", extension: ".webp", kind: KindImage, data: append([]byte("RIFF\x10\x00\x00\x00WEBPVP8 "), bytes.Repeat([]byte{0x01}, 900)...)},
		{name: "mp4", filename: "proof.mp4", mime: "video/mp4", extension: ".mp4", kind: KindVideo, data: append([]byte("\x00\x00\x00\x18ftypisom"), bytes.Repeat([]byte{0x01}, 900)...)},
		{name: "webm fallback", filename: "proof.webm", mime: "video/webm", extension: ".webm", kind: KindVideo, data: append([]byte{0x1a, 0x45, 0xdf, 0xa3, 0xff}, bytes.Repeat([]byte{0xff}, 900)...)},
		{name: "mov fallback", filename: "proof.mov", mime: "video/quicktime", extension: ".mov", kind: KindVideo, data: append([]byte{0x00, 0x00, 0x00, 0x14, 'f', 't', 'y', 'p', 'q', 't', ' ', ' '}, bytes.Repeat([]byte{0x00}, 900)...)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := &shortReader{data: tt.data}
			detected, err := DetectUpload(source, tt.filename)
			if err != nil {
				t.Fatal(err)
			}
			if detected.Kind != tt.kind || detected.MimeType != tt.mime || detected.Extension != tt.extension {
				t.Fatalf("detected=%+v", detected)
			}
			if source.off > 512 {
				t.Fatalf("detection read %d bytes, want at most 512", source.off)
			}
			got, err := io.ReadAll(detected.Reader)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, tt.data) {
				t.Fatal("detector consumed or changed upload bytes")
			}
		})
	}
}

func TestDetectUploadRejectsEmptyAndDisguisedFiles(t *testing.T) {
	for name, input := range map[string]struct {
		filename string
		data     []byte
	}{
		"empty":         {filename: "empty.jpg"},
		"text as video": {filename: "fake.mp4", data: []byte("this is ordinary text, not a video")},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := DetectUpload(bytes.NewReader(input.data), input.filename)
			if !errors.Is(err, ErrUnsupportedUpload) {
				t.Fatalf("err=%v, want ErrUnsupportedUpload", err)
			}
		})
	}
}

func TestPolicyAllows(t *testing.T) {
	for _, tt := range []struct {
		policy string
		kind   Kind
		want   bool
	}{
		{"image", KindImage, true}, {"image", KindVideo, false},
		{"video", KindImage, false}, {"video", KindVideo, true},
		{"image_video", KindImage, true}, {"image_video", KindVideo, true},
		{"unknown", KindImage, false},
	} {
		if got := PolicyAllows(tt.policy, tt.kind); got != tt.want {
			t.Fatalf("PolicyAllows(%q, %q)=%v, want %v", tt.policy, tt.kind, got, tt.want)
		}
	}
}

func TestVideoLimiterCapacityReleaseAndConcurrentAccess(t *testing.T) {
	limiter := NewVideoLimiter(3)
	releases := make([]func(), 0, 3)
	for range 3 {
		release, ok := limiter.TryAcquire()
		if !ok {
			t.Fatal("one of the first three acquisitions was rejected")
		}
		releases = append(releases, release)
	}
	if _, ok := limiter.TryAcquire(); ok {
		t.Fatal("fourth acquisition should be rejected")
	}
	releases[0]()
	releases[0]() // release is intentionally idempotent
	if _, ok := limiter.TryAcquire(); !ok {
		t.Fatal("permit was not reusable after release")
	}

	concurrent := NewVideoLimiter(3)
	start := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	successes := 0
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, ok := concurrent.TryAcquire(); ok {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()
	if successes != 3 {
		t.Fatalf("concurrent successes=%d, want 3", successes)
	}
}
