package bast

import (
	"sort"
	"strings"
	"time"

	"konkit/internal/textnorm"
)

const rakordaRowsPerPage = 25

// RakordaSnapshot is the immutable input used to render a blank attendance
// sheet. Only the sequence number is printed; the remaining cells are filled
// by hand at the event.
type RakordaSnapshot struct {
	DocumentDate string         `json:"document_date"`
	Location     string         `json:"location"`
	RegencyName  string         `json:"regency_name"`
	ProvinceName string         `json:"province_name"`
	ZoneName     string         `json:"zone_name"`
	FiscalYear   int            `json:"fiscal_year"`
	RowCount     int            `json:"row_count"`
	Logos        []LogoSnapshot `json:"logos"`
}

func buildRakordaSnapshot(ctx DP3Context, settings ScheduleSettings, logos []LogoSnapshot, documentDate string) (RakordaSnapshot, error) {
	documentDate = strings.TrimSpace(documentDate)
	if _, err := time.Parse("2006-01-02", documentDate); err != nil || strings.TrimSpace(settings.RakordaLocation) == "" || settings.RakordaRowCount < 5 || settings.RakordaRowCount > 200 {
		return RakordaSnapshot{}, ErrInvalidInput
	}
	if ctx.ZonePlaceholder || strings.TrimSpace(ctx.ZoneName) == "" {
		return RakordaSnapshot{}, ErrZoneNotConfigured
	}
	sortedLogos := append([]LogoSnapshot(nil), logos...)
	sort.SliceStable(sortedLogos, func(i, j int) bool { return sortedLogos[i].SortOrder < sortedLogos[j].SortOrder })
	return RakordaSnapshot{
		DocumentDate: documentDate,
		Location:     textnorm.BusinessUpper(settings.RakordaLocation),
		RegencyName:  textnorm.BusinessUpper(ctx.RegencyName),
		ProvinceName: textnorm.BusinessUpper(ctx.ProvinceName),
		ZoneName:     textnorm.BusinessUpper(ctx.ZoneName),
		FiscalYear:   ctx.FiscalYear,
		RowCount:     settings.RakordaRowCount,
		Logos:        sortedLogos,
	}, nil
}
