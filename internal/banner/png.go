package banner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ErrImageUnavailable means no PDF rasterizer (pdftoppm or Ghostscript) is
// installed, so a banner can only be downloaded as PDF.
var ErrImageUnavailable = errors.New("banner image export is unavailable")

// imageDPI is the PNG resolution: the large banners at 72 dpi (one pixel per
// point, the usual resolution for printed banners of several metres), the
// 60 x 40 cm plakat at 300 dpi.
func imageDPI(kind Kind) int {
	if kind == KindPlakat {
		return 300
	}
	return 72
}

// ImageFilename is the PNG download name, e.g. "PLAKAT PETANI KABUPATEN BONE.png".
func ImageFilename(kind Kind, regencyName string) string {
	return strings.TrimSuffix(Filename(kind, regencyName), ".pdf") + ".png"
}

// rasterSlots lets one banner be rasterized at a time: a seremoni banner is
// an 80 megapixel image.
var rasterSlots = make(chan struct{}, 1)

// RenderPNG turns a banner PDF into a PNG, for printers or phones that cannot
// open the full-size PDF.
func RenderPNG(ctx context.Context, kind Kind, pdf []byte) ([]byte, error) {
	if _, ok := layouts[kind]; !ok {
		return nil, ErrUnknownKind
	}
	select {
	case rasterSlots <- struct{}{}:
		defer func() { <-rasterSlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	dir, err := os.MkdirTemp("", "konkit-banner-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	input, output := filepath.Join(dir, "banner.pdf"), filepath.Join(dir, "banner.png")
	if err := os.WriteFile(input, pdf, 0o600); err != nil {
		return nil, err
	}
	dpi := strconv.Itoa(imageDPI(kind))
	var cmd *exec.Cmd
	if path, err := exec.LookPath("pdftoppm"); err == nil {
		cmd = exec.CommandContext(ctx, path, "-png", "-r", dpi, "-singlefile", input, strings.TrimSuffix(output, ".png"))
	} else if path := ghostscript(); path != "" {
		cmd = exec.CommandContext(ctx, path, "-q", "-dSAFER", "-dBATCH", "-dNOPAUSE", "-sDEVICE=png16m", "-r"+dpi,
			"-dTextAlphaBits=4", "-dGraphicsAlphaBits=4", "-sOutputFile="+output, input)
	} else {
		return nil, ErrImageUnavailable
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("rasterize banner: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return os.ReadFile(output)
}

func ghostscript() string {
	for _, name := range []string{"gs", "gswin64c", "gswin32c"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}
