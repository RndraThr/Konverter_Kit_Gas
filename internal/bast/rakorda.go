package bast

import (
	"sort"
	"strings"
	"time"

	"konkit/internal/textnorm"
)

// RakordaSnapshot is the immutable input used to render an attendance sheet.
// RAKORDA and Sosialisasi print blank numbered rows to be filled by hand;
// Training 10%/100% print Participants taken from the day's distribution.
type RakordaSnapshot struct {
	DocumentDate string                `json:"document_date"`
	Location     string                `json:"location"`
	RegencyName  string                `json:"regency_name"`
	ProvinceName string                `json:"province_name"`
	ZoneName     string                `json:"zone_name"`
	FiscalYear   int                   `json:"fiscal_year"`
	RowCount     int                   `json:"row_count"`
	Participants []ActivityParticipant `json:"participants,omitempty"`
	Logos        []LogoSnapshot        `json:"logos"`
	Signatories  closingSignatories    `json:"signatories"`
}

func buildRakordaSnapshot(spec rakordaDocumentSpec, ctx DP3Context, settings ScheduleSettings, logos []LogoSnapshot, documentDate string, participants []ActivityParticipant) (RakordaSnapshot, error) {
	documentDate = strings.TrimSpace(documentDate)
	location, rowCount := spec.settings(settings)
	if spec.participants != participantsNone {
		// Baris Training mengikuti jumlah peserta tanggal tersebut.
		participants = selectParticipants(spec.participants, participants)
		rowCount = len(participants)
		if rowCount == 0 {
			return RakordaSnapshot{}, ErrAggregateNoRecipients
		}
	} else {
		participants = nil
		if rowCount < 5 || rowCount > 200 {
			return RakordaSnapshot{}, ErrInvalidInput
		}
	}
	if _, err := time.Parse("2006-01-02", documentDate); err != nil || strings.TrimSpace(location) == "" {
		return RakordaSnapshot{}, ErrInvalidInput
	}
	if ctx.ZonePlaceholder || strings.TrimSpace(ctx.ZoneName) == "" {
		return RakordaSnapshot{}, ErrZoneNotConfigured
	}
	sortedLogos := append([]LogoSnapshot(nil), logos...)
	sort.SliceStable(sortedLogos, func(i, j int) bool { return sortedLogos[i].SortOrder < sortedLogos[j].SortOrder })
	return RakordaSnapshot{
		DocumentDate: documentDate,
		Location:     textnorm.BusinessUpper(location),
		RegencyName:  textnorm.BusinessUpper(ctx.RegencyName),
		ProvinceName: textnorm.BusinessUpper(ctx.ProvinceName),
		ZoneName:     textnorm.BusinessUpper(ctx.ZoneName),
		FiscalYear:   ctx.FiscalYear,
		RowCount:     rowCount,
		Participants: participants,
		Logos:        sortedLogos,
		Signatories: closingSignatories{
			AgricultureOfficeName: textnorm.BusinessUpper(settings.AgricultureOfficeName),
			AgricultureOfficeNIP:  strings.TrimSpace(settings.AgricultureOfficeNIP),
			InstallerName:         textnorm.BusinessUpper(settings.InstallerName),
			SupervisorName:        textnorm.BusinessUpper(settings.SupervisorName),
			PertaminaRepName:      textnorm.BusinessUpper(settings.PertaminaRepName),
		},
	}, nil
}
