package media

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"

	pdfapi "github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// ErrRotateUnsupported: the file kind cannot be rotated (video, WebP, ...).
var ErrRotateUnsupported = errors.New("media rotation unsupported for this file type")

// ErrRotateDegrees: only quarter turns are allowed.
var ErrRotateDegrees = errors.New("media rotation must be 90, 180 or 270 degrees")

// NormalizeDegrees turns any quarter turn (also negative) into 90, 180 or 270.
// Zero means nothing to do.
func NormalizeDegrees(degrees int) (int, error) {
	if degrees%90 != 0 {
		return 0, ErrRotateDegrees
	}
	return ((degrees % 360) + 360) % 360, nil
}

// RotateFile rotates an image or a PDF clockwise by degrees (90, 180, 270) and
// returns the new content. JPEG keeps JPEG (its EXIF orientation applied first,
// so the result is what viewers showed, turned), PNG stays lossless, and every
// page of a PDF gets the turn added to its page rotation.
func RotateFile(data []byte, mimeType string, degrees int, maxBytes int64) ([]byte, error) {
	switch mimeType {
	case "image/jpeg", "image/png":
		return rotateImage(data, mimeType, degrees, maxBytes)
	case "application/pdf":
		return rotatePDF(data, degrees)
	default:
		return nil, ErrRotateUnsupported
	}
}

func rotateImage(data []byte, mimeType string, degrees int, maxBytes int64) ([]byte, error) {
	var source image.Image
	var err error
	if mimeType == "image/png" {
		source, err = png.Decode(bytes.NewReader(data))
	} else {
		source, err = jpeg.Decode(bytes.NewReader(data))
		if err == nil {
			source = applyOrientation(source, jpegOrientation(data))
		}
	}
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	rotated := rotateClockwise(source, degrees)
	var out bytes.Buffer
	if mimeType == "image/png" {
		if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&out, rotated); err != nil {
			return nil, fmt.Errorf("encode png: %w", err)
		}
		return out.Bytes(), nil
	}
	// A high quality first; lower only when the limit of the table needs it.
	for _, quality := range []int{92, 85, 75} {
		out.Reset()
		if err := jpeg.Encode(&out, rotated, &jpeg.Options{Quality: quality}); err != nil {
			return nil, fmt.Errorf("encode jpeg: %w", err)
		}
		if maxBytes <= 0 || int64(out.Len()) <= maxBytes {
			break
		}
	}
	return out.Bytes(), nil
}

func rotatePDF(data []byte, degrees int) ([]byte, error) {
	var out bytes.Buffer
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	if err := pdfapi.Rotate(bytes.NewReader(data), &out, degrees, nil, conf); err != nil {
		return nil, fmt.Errorf("rotate pdf: %w", err)
	}
	return out.Bytes(), nil
}

// rotateClockwise turns img by 90, 180 or 270 degrees.
func rotateClockwise(img image.Image, degrees int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	src := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(src, src.Bounds(), img, b.Min, draw.Src)
	var dst *image.RGBA
	if degrees == 180 {
		dst = image.NewRGBA(image.Rect(0, 0, w, h))
	} else {
		dst = image.NewRGBA(image.Rect(0, 0, h, w))
	}
	for y := 0; y < h; y++ {
		row := src.Pix[y*src.Stride : y*src.Stride+w*4]
		for x := 0; x < w; x++ {
			var dx, dy int
			switch degrees {
			case 90:
				dx, dy = h-1-y, x
			case 180:
				dx, dy = w-1-x, h-1-y
			default: // 270
				dx, dy = y, w-1-x
			}
			o := dy*dst.Stride + dx*4
			copy(dst.Pix[o:o+4], row[x*4:x*4+4])
		}
	}
	return dst
}

// applyOrientation makes the pixels upright as EXIF orientation says.
func applyOrientation(img image.Image, orientation int) image.Image {
	switch orientation {
	case 3:
		return rotateClockwise(img, 180)
	case 6:
		return rotateClockwise(img, 90)
	case 8:
		return rotateClockwise(img, 270)
	case 2, 4, 5, 7:
		// Mirrored orientations: mirror horizontally, then turn.
		mirrored := mirror(img)
		switch orientation {
		case 4:
			return rotateClockwise(mirrored, 180)
		case 5:
			return rotateClockwise(mirrored, 270)
		case 7:
			return rotateClockwise(mirrored, 90)
		}
		return mirrored
	}
	return img
}

func mirror(img image.Image) image.Image {
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.Set(b.Dx()-1-x, y, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return out
}

// jpegOrientation reads the EXIF orientation (1-8) of a JPEG; 1 when absent.
func jpegOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 1
	}
	i := 2
	for i+4 <= len(data) {
		if data[i] != 0xFF {
			return 1
		}
		marker := data[i+1]
		if marker == 0xDA || marker == 0xD9 { // image data starts: no EXIF before it
			return 1
		}
		size := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
		if size < 2 || i+2+size > len(data) {
			return 1
		}
		segment := data[i+4 : i+2+size]
		if marker == 0xE1 && len(segment) > 14 && string(segment[:6]) == "Exif\x00\x00" {
			return tiffOrientation(segment[6:])
		}
		i += 2 + size
	}
	return 1
}

func tiffOrientation(tiff []byte) int {
	if len(tiff) < 8 {
		return 1
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	offset := int(order.Uint32(tiff[4:8]))
	if offset+2 > len(tiff) {
		return 1
	}
	count := int(order.Uint16(tiff[offset : offset+2]))
	for n := 0; n < count; n++ {
		entry := offset + 2 + n*12
		if entry+12 > len(tiff) {
			return 1
		}
		if order.Uint16(tiff[entry:entry+2]) == 0x0112 {
			value := int(order.Uint16(tiff[entry+8 : entry+10]))
			if value >= 1 && value <= 8 {
				return value
			}
			return 1
		}
	}
	return 1
}
