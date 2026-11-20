// Package banner generates the printed banners of a kabupaten from the
// approved PDF templates: backdrop, pelatihan teknis, seremoni and plakat.
// A template is used as is (no recompression); a transparent layer with the
// kabupaten logo and the typed text is stamped on top of it.
package banner

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // logo formats
	_ "image/png"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	pdfapi "github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

//go:embed assets/*.pdf assets/*.ttf
var assets embed.FS

// Kind is one of the banner templates.
type Kind string

const (
	KindBackdrop  Kind = "backdrop"
	KindPelatihan Kind = "pelatihan"
	KindSeremoni  Kind = "seremoni"
	KindPlakat    Kind = "plakat"
)

// Kinds lists the banners in the order they are shown.
var Kinds = []Kind{KindBackdrop, KindPelatihan, KindSeremoni, KindPlakat}

var ErrUnknownKind = errors.New("unknown banner kind")

type box struct{ x0, y0, x1, y1 float64 }

func (b box) w() float64 { return b.x1 - b.x0 }
func (b box) h() float64 { return b.y1 - b.y0 }

// layout holds a template's page size and where the generated parts go, in
// points, measured on the template files.
type layout struct {
	template string
	title    string // file name prefix
	w, h     float64
	logo     box
}

var layouts = map[Kind]layout{
	KindBackdrop:  {template: "assets/backdrop-petani.pdf", title: "BANNER BACKDROP PETANI", w: 8503.9365, h: 5669.2915, logo: box{4930, 226, 5590, 917}},
	KindPelatihan: {template: "assets/pelatihan-petani.pdf", title: "BANNER PELATIHAN TEKNIS PETANI", w: 8503.9365, h: 5669.2915, logo: box{4920, 325, 5595, 1016}},
	KindSeremoni:  {template: "assets/seremoni-petani.pdf", title: "BANNER SEREMONI PETANI", w: 11338.583, h: 7086.6143, logo: box{6300, 520, 7180, 1380}},
	KindPlakat:    {template: "assets/plakat-petani.pdf", title: "PLAKAT PETANI", w: 1700.7874, h: 1133.8583, logo: box{962, 24, 1104, 166}},
}

// Text color of the template titles, used for the plakat's kabupaten line.
var inkColor = [3]int{0x22, 0x22, 0x22}

// TextStyle adjusts one typed text of a banner: its size as a percentage of
// the default and its color as #RRGGBB. Zero values keep the defaults.
type TextStyle struct {
	SizePercent int    `json:"size_percent,omitempty"`
	Color       string `json:"color,omitempty"`
}

// Styled texts and their default colors; sizes default to 100%.
const (
	StyleSeremoniName    = "seremoni_name"
	StylePlakatDate      = "plakat_date"
	StylePlakatName      = "plakat_name"
	StyleBackdropService = "backdrop_service"
	MinSizePercent       = 60
	MaxSizePercent       = 140
)

var defaultStyleColors = map[string]string{
	StyleSeremoniName:    "#232C7A",
	StylePlakatDate:      "#222222",
	StylePlakatName:      "#232C7A",
	StyleBackdropService: "#1D1819",
}

// DefaultStyleColors lists the styled texts with their default colors.
func DefaultStyleColors() map[string]string {
	out := make(map[string]string, len(defaultStyleColors))
	for key, color := range defaultStyleColors {
		out[key] = color
	}
	return out
}

// resolve gives the scale and RGB color of a styled text.
func resolveStyle(styles map[string]TextStyle, key string) (float64, [3]int) {
	style := styles[key]
	scale := 1.0
	if style.SizePercent >= MinSizePercent && style.SizePercent <= MaxSizePercent {
		scale = float64(style.SizePercent) / 100
	}
	color, ok := parseHexColor(style.Color)
	if !ok {
		color, _ = parseHexColor(defaultStyleColors[key])
	}
	return scale, color
}

func parseHexColor(value string) ([3]int, bool) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(value) != 6 {
		return [3]int{}, false
	}
	var rgb [3]int
	for i := range 3 {
		var n int
		if _, err := fmt.Sscanf(value[i*2:i*2+2], "%02x", &n); err != nil {
			return [3]int{}, false
		}
		rgb[i] = n
	}
	return rgb, true
}

// ValidHexColor reports whether value is #RRGGBB.
func ValidHexColor(value string) bool {
	_, ok := parseHexColor(value)
	return ok && strings.HasPrefix(strings.TrimSpace(value), "#")
}

// ServiceCenter is the service point printed on the backdrop.
type ServiceCenter struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Phone   string `json:"phone"`
}

// Input is everything a banner needs besides its template.
type Input struct {
	Kind          Kind
	RegencyName   string // e.g. "KAB. BONE"
	Logo          []byte // PNG or JPEG; nil leaves the slot empty
	ServiceCenter ServiceCenter
	PersonName    string    // seremoni and plakat
	Place         string    // plakat, e.g. "Bone"
	Date          time.Time // plakat
	Styles        map[string]TextStyle
}

// Filename is the download name, e.g. "PLAKAT PETANI KABUPATEN BONE.pdf".
func Filename(kind Kind, regencyName string) string {
	return fmt.Sprintf("%s %s.pdf", layouts[kind].title, strings.ToUpper(RegencyDisplayName(regencyName)))
}

// Render returns the banner PDF.
func Render(in Input) ([]byte, error) {
	lay, ok := layouts[in.Kind]
	if !ok {
		return nil, ErrUnknownKind
	}
	template, err := assets.ReadFile(lay.template)
	if err != nil {
		return nil, fmt.Errorf("read banner template: %w", err)
	}
	overlay, err := drawOverlay(lay, in)
	if err != nil {
		return nil, err
	}
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	// Same page size as the template, so scale 1 at the origin lines up exactly.
	wm, err := pdfapi.PDFWatermarkForReadSeeker(bytes.NewReader(overlay), 1, "scale:1 abs, rotation:0, position:bl, offset:0 0", true, false, types.POINTS)
	if err != nil {
		return nil, fmt.Errorf("prepare banner layer: %w", err)
	}
	var out bytes.Buffer
	if err := pdfapi.AddWatermarks(bytes.NewReader(template), &out, nil, wm, conf); err != nil {
		return nil, fmt.Errorf("stamp banner layer: %w", err)
	}
	return out.Bytes(), nil
}

func drawOverlay(lay layout, in Input) ([]byte, error) {
	pdf := fpdf.NewCustom(&fpdf.InitType{UnitStr: "pt", Size: fpdf.SizeType{Wd: lay.w, Ht: lay.h}})
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)
	for _, style := range []struct{ name, file string }{{"S", "Montserrat-SemiBold.ttf"}, {"B", "Montserrat-Bold.ttf"}, {"X", "Montserrat-ExtraBold.ttf"}} {
		data, err := assets.ReadFile("assets/" + style.file)
		if err != nil {
			return nil, err
		}
		pdf.AddUTF8FontFromBytes("Montserrat"+style.name, "", data)
	}
	pdf.AddPage()
	if len(in.Logo) > 0 {
		if err := placeLogo(pdf, in.Logo, lay.logo); err != nil {
			return nil, err
		}
	}
	switch in.Kind {
	case KindBackdrop:
		scale, color := resolveStyle(in.Styles, StyleBackdropService)
		drawServiceCenter(pdf, in.ServiceCenter, scale, color)
	case KindSeremoni:
		// Between the program lines (to y 3989) and the products (from y 4482).
		scale, color := resolveStyle(in.Styles, StyleSeremoniName)
		centeredFit(pdf, "MontserratX", strings.ToUpper(strings.TrimSpace(in.PersonName)), 410*scale, 9600, lay.w/2, 4400, color)
	case KindPlakat:
		drawPlakat(pdf, lay, in)
	}
	if err := pdf.Error(); err != nil {
		return nil, fmt.Errorf("draw banner layer: %w", err)
	}
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		return nil, fmt.Errorf("write banner layer: %w", err)
	}
	return out.Bytes(), nil
}

// placeLogo fits the logo in its slot, centered, keeping its proportions.
func placeLogo(pdf *fpdf.Fpdf, logo []byte, slot box) error {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(logo))
	if err != nil {
		return fmt.Errorf("read kabupaten logo: %w", err)
	}
	imageType := map[string]string{"png": "PNG", "jpeg": "JPG"}[format]
	if imageType == "" {
		return fmt.Errorf("kabupaten logo must be PNG or JPEG, got %s", format)
	}
	pdf.RegisterImageOptionsReader("logo", fpdf.ImageOptions{ImageType: imageType}, bytes.NewReader(logo))
	scale := min(slot.w()/float64(cfg.Width), slot.h()/float64(cfg.Height))
	w, h := float64(cfg.Width)*scale, float64(cfg.Height)*scale
	pdf.ImageOptions("logo", slot.x0+(slot.w()-w)/2, slot.y0+(slot.h()-h)/2, w, h, false, fpdf.ImageOptions{ImageType: imageType}, 0, "")
	return nil
}

// Montserrat's cap height, as a share of the font size.
const capHeight = 0.70

// centeredFit writes one centered line with its baseline at y, shrinking the
// size until it fits maxWidth.
func centeredFit(pdf *fpdf.Fpdf, font, text string, size, maxWidth, cx, y float64, color [3]int) {
	if text == "" {
		return
	}
	pdf.SetFont(font, "", size)
	if w := pdf.GetStringWidth(text); w > maxWidth {
		size *= maxWidth / w
		pdf.SetFont(font, "", size)
	}
	pdf.SetTextColor(color[0], color[1], color[2])
	pdf.Text(cx-pdf.GetStringWidth(text)/2, y, text)
}

// Plakat: the template ends its title with "Di Kabupaten"; that line is
// written again with the kabupaten name, then the place and date and the
// name go between the program line and the products.
func drawPlakat(pdf *fpdf.Fpdf, lay layout, in Input) {
	// "Di Kabupaten" sits at x 718-986, caps from y 346; cover it and write the
	// full line in the title's size, taken from the line above it (x 188-1518).
	pdf.SetFillColor(255, 255, 255)
	pdf.Rect(690, 336, 330, 54, "F")
	pdf.SetFont("MontserratB", "", 100)
	size := 100 * (1518 - 188) / pdf.GetStringWidth("Untuk Mesin Pompa Air Bagi Petani Sasaran Tahun Anggaran 2026")
	centeredFit(pdf, "MontserratB", "Di "+RegencyDisplayName(in.RegencyName), size, 1500, lay.w/2, 346+capHeight*size, inkColor)

	if place, date := strings.TrimSpace(in.Place), FormatDate(in.Date); place != "" || date != "" {
		line := strings.Trim(place+", "+date, ", ")
		scale, color := resolveStyle(in.Styles, StylePlakatDate)
		centeredFit(pdf, "MontserratB", line, 46*scale, 1500, lay.w/2, 594, color)
	}
	// Between the program line (to y 514) and the products (from y 707).
	scale, color := resolveStyle(in.Styles, StylePlakatName)
	centeredFit(pdf, "MontserratX", strings.ToUpper(strings.TrimSpace(in.PersonName)), 58*scale, 1500, lay.w/2, 672, color)
}

// drawServiceCenter fills the yellow "Lokasi Servis" box of the backdrop:
// the name, the address, and the phone at the bottom, shrunk to fit.
func drawServiceCenter(pdf *fpdf.Fpdf, sc ServiceCenter, base float64, color [3]int) {
	area := box{640, 3650, 1770, 4410}
	name, address := strings.TrimSpace(sc.Name), strings.TrimSpace(sc.Address)
	phone := strings.TrimSpace(sc.Phone)
	if name == "" && address == "" && phone == "" {
		return
	}
	var nameLines, addressLines []string
	var sName, sBody float64
	for scale := base; ; scale -= 0.05 {
		sName, sBody = 118*scale, 80*scale
		pdf.SetFont("MontserratX", "", sName)
		nameLines = wrapText(pdf, name, area.w())
		pdf.SetFont("MontserratB", "", sBody)
		addressLines = wrapText(pdf, address, area.w())
		need := float64(len(nameLines))*sName*1.15 + sBody*0.6 + float64(len(addressLines))*sBody*1.3 + sBody*1.6
		if need <= area.h() || scale < 0.4 {
			break
		}
	}
	pdf.SetTextColor(color[0], color[1], color[2])
	y := area.y0 + sName*capHeight
	pdf.SetFont("MontserratX", "", sName)
	for _, line := range nameLines {
		pdf.Text(area.x0, y, line)
		y += sName * 1.15
	}
	y += sBody*0.6 - (sName*1.15 - sBody)
	pdf.SetFont("MontserratB", "", sBody)
	for _, line := range addressLines {
		pdf.Text(area.x0, y, line)
		y += sBody * 1.3
	}
	if phone != "" {
		pdf.Text(area.x0, area.y1-sBody*0.3, "Telp : "+phone)
	}
}

func wrapText(pdf *fpdf.Fpdf, text string, width float64) []string {
	var lines []string
	current := ""
	for _, word := range strings.Fields(text) {
		next := strings.TrimSpace(current + " " + word)
		if current == "" || pdf.GetStringWidth(next) <= width {
			current = next
			continue
		}
		lines = append(lines, current)
		current = word
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

// RegencyDisplayName turns "KAB. TANAH DATAR" into "Kabupaten Tanah Datar"
// and "KOTA LANGSA" into "Kota Langsa".
func RegencyDisplayName(name string) string {
	name = strings.Join(strings.Fields(name), " ")
	upper := strings.ToUpper(name)
	prefix, rest := "", name
	switch {
	case strings.HasPrefix(upper, "KAB. "), strings.HasPrefix(upper, "KAB "):
		prefix, rest = "Kabupaten ", strings.TrimSpace(name[strings.Index(name, " ")+1:])
	case strings.HasPrefix(upper, "KABUPATEN "):
		prefix, rest = "Kabupaten ", name[len("KABUPATEN "):]
	case strings.HasPrefix(upper, "KOTA "):
		prefix, rest = "Kota ", name[len("KOTA "):]
	}
	return prefix + titleCase(rest)
}

// RegencyShortName is the name without "Kabupaten"/"Kota", the default place
// on the plakat ("Bone").
func RegencyShortName(name string) string {
	display := RegencyDisplayName(name)
	for _, prefix := range []string{"Kabupaten ", "Kota "} {
		if strings.HasPrefix(display, prefix) {
			return strings.TrimPrefix(display, prefix)
		}
	}
	return display
}

func titleCase(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, word := range words {
		words[i] = strings.ToUpper(word[:1]) + word[1:]
	}
	return strings.Join(words, " ")
}

var months = [...]string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}

// FormatDate writes "29 November 2026"; the zero time is empty.
func FormatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return fmt.Sprintf("%d %s %d", t.Day(), months[t.Month()-1], t.Year())
}
