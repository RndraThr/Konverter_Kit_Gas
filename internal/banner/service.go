package banner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"konkit/internal/audit"
	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound     = errors.New("banner schedule not found")
	ErrNotSupported = errors.New("banners are only available for farmer programs")
	ErrInvalidLogo  = errors.New("kabupaten logo must be a PNG or JPEG image")
	ErrLogoTooLarge = errors.New("kabupaten logo is too large")
	ErrInvalidDate  = errors.New("plakat date must be YYYY-MM-DD")
	ErrInvalidStyle = errors.New("text style must have a known text, a size of 60-140% and a #RRGGBB color")
	// ErrInvalidProgram: the seremoni program text has too many or too long lines.
	ErrInvalidProgram = errors.New("seremoni program text must have at most 2 lines of 60 characters")
	supportedBrands   = []string{"shark", "loncin"}
)

const (
	// MaxLogoBytes bounds an uploaded kabupaten logo.
	MaxLogoBytes      = 10 << 20
	dateLayout        = "2006-01-02"
	defaultBrand      = "shark"
	farmerProgramType = "farmer"
)

// Settings are the typed parts of a schedule's banners. An empty service
// center field falls back to the master data.
type Settings struct {
	ServiceCenterName    string `json:"service_center_name"`
	ServiceCenterAddress string `json:"service_center_address"`
	ServiceCenterPhone   string `json:"service_center_phone"`
	SeremoniName         string `json:"seremoni_name"`
	// SeremoniProgram is the seremoni banner's program text, lines separated
	// by a newline; the overview gives the template's own text when none was
	// typed. Nil on save keeps the stored text, empty restores the template's.
	SeremoniProgram *string `json:"seremoni_program,omitempty"`
	PlakatName      string  `json:"plakat_name"`
	PlakatPlace     string  `json:"plakat_place"`
	PlakatDate      string  `json:"plakat_date"`
	// Styles holds the size and color of the typed texts, by text. Nil on
	// save keeps the stored styles.
	Styles StyleMap `json:"styles"`
}

// StyleMap is Settings.Styles. Anything but a JSON object reads as nil, so
// app 1.2.10, which sends the styles back as text, keeps them unchanged.
type StyleMap map[string]TextStyle

func (m *StyleMap) UnmarshalJSON(data []byte) error {
	var styles map[string]TextStyle
	if json.Unmarshal(data, &styles) != nil {
		*m = nil
		return nil
	}
	*m = styles
	return nil
}

// Overview is what the banner page shows for one schedule.
type Overview struct {
	ScheduleID    string        `json:"schedule_id"`
	ScheduleName  string        `json:"schedule_name"`
	RegencyID     string        `json:"regency_id"`
	RegencyName   string        `json:"regency_name"`
	DisplayName   string        `json:"display_name"`
	ShortName     string        `json:"short_name"`
	ProgramType   string        `json:"program_type"`
	Supported     bool          `json:"supported"`
	Brand         string        `json:"brand"`
	HasLogo       bool          `json:"has_logo"`
	LogoUpdatedAt *time.Time    `json:"logo_updated_at,omitempty"`
	MasterCenter  ServiceCenter `json:"master_service_center"`
	ServiceCenter ServiceCenter `json:"service_center"`
	Settings      Settings      `json:"settings"`
	Kinds         []KindInfo    `json:"kinds"`
	// StyleDefaults are the default colors of the styled texts.
	StyleDefaults map[string]string `json:"style_defaults"`
	// SeremoniProgramDefault is the program text of the seremoni template.
	SeremoniProgramDefault string `json:"seremoni_program_default"`
}

// KindInfo describes one banner for the page.
type KindInfo struct {
	Kind     Kind   `json:"kind"`
	Label    string `json:"label"`
	Size     string `json:"size"`
	Filename string `json:"filename"`
}

var kindLabels = map[Kind][2]string{
	KindBackdrop:  {"Backdrop", "300 x 200 cm"},
	KindPelatihan: {"Pelatihan Teknis", "300 x 200 cm"},
	KindSeremoni:  {"Seremoni", "400 x 250 cm"},
	KindPlakat:    {"Plakat", "60 x 40 cm"},
}

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

type scheduleRow struct {
	id, name, regencyID, regencyName, programType string
	packageValues                                 []byte
}

func (s *Service) schedule(ctx context.Context, scheduleID string, scope auth.RegencyScope) (scheduleRow, error) {
	var row scheduleRow
	err := s.pool.QueryRow(ctx, `
		SELECT ps.id::text, ps.name, r.id::text, r.name, p.program_type, COALESCE(pt.values_json, '{}'::jsonb)
		FROM program_schedules ps
		JOIN regencies r ON r.id = ps.regency_id
		JOIN programs p ON p.id = ps.program_id
		LEFT JOIN package_template_versions pt ON pt.id = ps.package_template_version_id
		WHERE ps.id = $1 AND ($2 OR ps.regency_id::text = ANY($3))`, strings.TrimSpace(scheduleID), scope.Unrestricted, scope.RegencyIDs).
		Scan(&row.id, &row.name, &row.regencyID, &row.regencyName, &row.programType, &row.packageValues)
	if errors.Is(err, pgx.ErrNoRows) {
		return scheduleRow{}, ErrNotFound
	}
	if err != nil {
		return scheduleRow{}, fmt.Errorf("get banner schedule: %w", err)
	}
	return row, nil
}

// machineBrand is the brand of the package's default (or only) machine, which
// picks the service center sheet; Shark when it cannot be told.
func machineBrand(packageValues []byte) string {
	var values struct {
		Machines []struct {
			Brand     string `json:"brand"`
			IsDefault bool   `json:"is_default"`
		} `json:"machine_options"`
	}
	if json.Unmarshal(packageValues, &values) != nil || len(values.Machines) == 0 {
		return defaultBrand
	}
	brand := values.Machines[0].Brand
	for _, machine := range values.Machines {
		if machine.IsDefault {
			brand = machine.Brand
		}
	}
	for _, known := range supportedBrands {
		if strings.Contains(strings.ToLower(brand), known) {
			return known
		}
	}
	return defaultBrand
}

// Overview returns the schedule's banner data with the master service center.
func (s *Service) Overview(ctx context.Context, scheduleID string, scope auth.RegencyScope) (Overview, error) {
	row, err := s.schedule(ctx, scheduleID, scope)
	if err != nil {
		return Overview{}, err
	}
	o := Overview{
		ScheduleID: row.id, ScheduleName: row.name, RegencyID: row.regencyID, RegencyName: row.regencyName,
		DisplayName: RegencyDisplayName(row.regencyName), ShortName: RegencyShortName(row.regencyName),
		ProgramType: row.programType, Supported: row.programType == farmerProgramType, Brand: machineBrand(row.packageValues),
	}
	var logoAt time.Time
	err = s.pool.QueryRow(ctx, `SELECT updated_at FROM regency_logos WHERE regency_id = $1`, row.regencyID).Scan(&logoAt)
	switch {
	case err == nil:
		o.HasLogo, o.LogoUpdatedAt = true, &logoAt
	case !errors.Is(err, pgx.ErrNoRows):
		return Overview{}, fmt.Errorf("get regency logo: %w", err)
	}
	err = s.pool.QueryRow(ctx, `SELECT name, address, phone FROM banner_service_centers WHERE regency_name = upper($1) AND brand = $2 ORDER BY sort_order LIMIT 1`, row.regencyName, o.Brand).
		Scan(&o.MasterCenter.Name, &o.MasterCenter.Address, &o.MasterCenter.Phone)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Overview{}, fmt.Errorf("get service center: %w", err)
	}
	var date *time.Time
	var styles []byte
	program := DefaultSeremoniProgram
	err = s.pool.QueryRow(ctx, `
		SELECT COALESCE(service_center_name, ''), COALESCE(service_center_address, ''), COALESCE(service_center_phone, ''),
			seremoni_name, COALESCE(seremoni_program, $2), plakat_name, plakat_place, plakat_date, styles
		FROM banner_settings WHERE schedule_id = $1`, row.id, DefaultSeremoniProgram).
		Scan(&o.Settings.ServiceCenterName, &o.Settings.ServiceCenterAddress, &o.Settings.ServiceCenterPhone,
			&o.Settings.SeremoniName, &program, &o.Settings.PlakatName, &o.Settings.PlakatPlace, &date, &styles)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Overview{}, fmt.Errorf("get banner settings: %w", err)
	}
	if date != nil {
		o.Settings.PlakatDate = date.Format(dateLayout)
	}
	o.Settings.Styles = StyleMap{}
	if len(styles) > 0 {
		_ = json.Unmarshal(styles, &o.Settings.Styles)
	}
	o.Settings.SeremoniProgram = &program
	o.StyleDefaults = DefaultStyleColors()
	o.SeremoniProgramDefault = DefaultSeremoniProgram
	o.ServiceCenter = ServiceCenter{
		Name:    firstNonEmpty(o.Settings.ServiceCenterName, o.MasterCenter.Name),
		Address: firstNonEmpty(o.Settings.ServiceCenterAddress, o.MasterCenter.Address),
		Phone:   firstNonEmpty(o.Settings.ServiceCenterPhone, o.MasterCenter.Phone),
	}
	for _, kind := range Kinds {
		o.Kinds = append(o.Kinds, KindInfo{Kind: kind, Label: kindLabels[kind][0], Size: kindLabels[kind][1], Filename: Filename(kind, row.regencyName)})
	}
	return o, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// SaveSettings stores the schedule's banner text. A service center field
// equal to the master data is stored empty, so later master changes apply.
func (s *Service) SaveSettings(ctx context.Context, actor auth.Principal, scheduleID string, in Settings, scope auth.RegencyScope, meta auth.ClientMeta) (Overview, error) {
	current, err := s.Overview(ctx, scheduleID, scope)
	if err != nil {
		return Overview{}, err
	}
	var date *time.Time
	if d := strings.TrimSpace(in.PlakatDate); d != "" {
		parsed, err := time.Parse(dateLayout, d)
		if err != nil {
			return Overview{}, ErrInvalidDate
		}
		date = &parsed
	}
	incoming := in.Styles
	if incoming == nil {
		incoming = current.Settings.Styles
	}
	styles := map[string]TextStyle{}
	for key, style := range incoming {
		if _, known := defaultStyleColors[key]; !known {
			return Overview{}, ErrInvalidStyle
		}
		if style.SizePercent == 100 {
			style.SizePercent = 0
		}
		if style.SizePercent != 0 && (style.SizePercent < MinSizePercent || style.SizePercent > MaxSizePercent) {
			return Overview{}, ErrInvalidStyle
		}
		style.Color = strings.ToUpper(strings.TrimSpace(style.Color))
		if style.Color == defaultStyleColors[key] {
			style.Color = ""
		}
		if style.Color != "" && !ValidHexColor(style.Color) {
			return Overview{}, ErrInvalidStyle
		}
		if style != (TextStyle{}) {
			styles[key] = style
		}
	}
	stylesJSON, _ := json.Marshal(styles)
	program, err := programSetting(in.SeremoniProgram, current.Settings.SeremoniProgram)
	if err != nil {
		return Overview{}, err
	}
	override := func(value, master string) *string {
		value = strings.Join(strings.Fields(value), " ")
		if value == "" || value == master {
			return nil
		}
		return &value
	}
	clean := func(value string) string { return strings.Join(strings.Fields(value), " ") }
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Overview{}, fmt.Errorf("begin banner settings: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		INSERT INTO banner_settings (schedule_id, service_center_name, service_center_address, service_center_phone, seremoni_name, plakat_name, plakat_place, plakat_date, styles, updated_by, seremoni_program, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NULLIF($10, '')::uuid, $11, now())
		ON CONFLICT (schedule_id) DO UPDATE SET service_center_name = EXCLUDED.service_center_name, service_center_address = EXCLUDED.service_center_address,
			service_center_phone = EXCLUDED.service_center_phone, seremoni_name = EXCLUDED.seremoni_name, plakat_name = EXCLUDED.plakat_name,
			plakat_place = EXCLUDED.plakat_place, plakat_date = EXCLUDED.plakat_date, styles = EXCLUDED.styles, updated_by = EXCLUDED.updated_by,
			seremoni_program = EXCLUDED.seremoni_program, updated_at = now()`,
		current.ScheduleID, override(in.ServiceCenterName, current.MasterCenter.Name), override(in.ServiceCenterAddress, current.MasterCenter.Address),
		override(in.ServiceCenterPhone, current.MasterCenter.Phone), clean(in.SeremoniName), clean(in.PlakatName), clean(in.PlakatPlace), date, stylesJSON, actor.UserID, program); err != nil {
		return Overview{}, fmt.Errorf("save banner settings: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "banner.settings_saved", ResourceType: "program_schedule", ResourceID: current.ScheduleID, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return Overview{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Overview{}, fmt.Errorf("commit banner settings: %w", err)
	}
	return s.Overview(ctx, scheduleID, scope)
}

// programSetting is the seremoni program text to store: nil (the template's
// own text) when it is empty or the same as the template's, the current text
// when none was sent (an older app), and otherwise the cleaned lines.
func programSetting(in, current *string) (*string, error) {
	if in == nil {
		in = current
	}
	if in == nil {
		return nil, nil
	}
	lines := ProgramLines(*in)
	if len(lines) > MaxProgramLines {
		return nil, ErrInvalidProgram
	}
	for _, line := range lines {
		if utf8.RuneCountInString(line) > MaxProgramLineRunes {
			return nil, ErrInvalidProgram
		}
	}
	text := strings.Join(lines, "\n")
	if text == "" || text == DefaultSeremoniProgram {
		return nil, nil
	}
	return &text, nil
}

// SaveLogo replaces the kabupaten logo of the schedule's regency. The file
// is kept as uploaded (no resizing), so it prints sharp on the large banners.
func (s *Service) SaveLogo(ctx context.Context, actor auth.Principal, scheduleID, filename string, data []byte, scope auth.RegencyScope, meta auth.ClientMeta) (Overview, error) {
	row, err := s.schedule(ctx, scheduleID, scope)
	if err != nil {
		return Overview{}, err
	}
	if len(data) > MaxLogoBytes {
		return Overview{}, ErrLogoTooLarge
	}
	mime := http.DetectContentType(data)
	if mime != "image/png" && mime != "image/jpeg" {
		return Overview{}, ErrInvalidLogo
	}
	if _, _, err := image.DecodeConfig(bytes.NewReader(data)); err != nil {
		return Overview{}, ErrInvalidLogo
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Overview{}, fmt.Errorf("begin regency logo: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		INSERT INTO regency_logos (regency_id, content, mime_type, original_filename, updated_by, updated_at)
		VALUES ($1, $2, $3, $4, NULLIF($5, '')::uuid, now())
		ON CONFLICT (regency_id) DO UPDATE SET content = EXCLUDED.content, mime_type = EXCLUDED.mime_type,
			original_filename = EXCLUDED.original_filename, updated_by = EXCLUDED.updated_by, updated_at = now()`,
		row.regencyID, data, mime, filename, actor.UserID); err != nil {
		return Overview{}, fmt.Errorf("save regency logo: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "banner.logo_saved", ResourceType: "regency", ResourceID: row.regencyID, Metadata: map[string]any{"filename": filename}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return Overview{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Overview{}, fmt.Errorf("commit regency logo: %w", err)
	}
	return s.Overview(ctx, scheduleID, scope)
}

// Logo returns the schedule's kabupaten logo and its MIME type.
func (s *Service) Logo(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]byte, string, error) {
	row, err := s.schedule(ctx, scheduleID, scope)
	if err != nil {
		return nil, "", err
	}
	var data []byte
	var mime string
	err = s.pool.QueryRow(ctx, `SELECT content, mime_type FROM regency_logos WHERE regency_id = $1`, row.regencyID).Scan(&data, &mime)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", fmt.Errorf("read regency logo: %w", err)
	}
	return data, mime, nil
}

// Banner renders one banner of the schedule with its saved text.
func (s *Service) Banner(ctx context.Context, scheduleID string, kind Kind, scope auth.RegencyScope) ([]byte, string, error) {
	if _, ok := layouts[kind]; !ok {
		return nil, "", ErrUnknownKind
	}
	o, err := s.Overview(ctx, scheduleID, scope)
	if err != nil {
		return nil, "", err
	}
	if !o.Supported {
		return nil, "", ErrNotSupported
	}
	in := Input{Kind: kind, RegencyName: o.RegencyName, ServiceCenter: o.ServiceCenter, PersonName: o.Settings.SeremoniName, Styles: o.Settings.Styles}
	if program := o.Settings.SeremoniProgram; kind == KindSeremoni && program != nil && *program != DefaultSeremoniProgram {
		in.Program = ProgramLines(*program)
	}
	if kind == KindPlakat {
		in.PersonName = o.Settings.PlakatName
		in.Place = firstNonEmpty(o.Settings.PlakatPlace, o.ShortName)
		if o.Settings.PlakatDate != "" {
			in.Date, _ = time.Parse(dateLayout, o.Settings.PlakatDate)
		}
	}
	if o.HasLogo {
		if in.Logo, _, err = s.Logo(ctx, scheduleID, scope); err != nil {
			return nil, "", err
		}
	}
	pdf, err := Render(in)
	if err != nil {
		return nil, "", err
	}
	return pdf, Filename(kind, o.RegencyName), nil
}

// BannerImage returns the banner as a PNG, for printers or phones that cannot
// open the full-size PDF.
func (s *Service) BannerImage(ctx context.Context, scheduleID string, kind Kind, scope auth.RegencyScope) ([]byte, string, error) {
	pdf, filename, err := s.Banner(ctx, scheduleID, kind, scope)
	if err != nil {
		return nil, "", err
	}
	png, err := RenderPNG(ctx, kind, pdf)
	if err != nil {
		return nil, "", err
	}
	return png, strings.TrimSuffix(filename, ".pdf") + ".png", nil
}
