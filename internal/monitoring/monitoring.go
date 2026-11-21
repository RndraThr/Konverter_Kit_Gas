// Package monitoring serves the central team's live distribution monitoring:
// the phase calendar of every kabupaten (material, rakor, sosialisasi,
// distribusi, penutupan), who checks which kabupaten, and the check of each
// completed recipient (approve, or mark photos wrong and ask for a revision).
package monitoring

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"konkit/internal/distribution"
)

// Phases of a kabupaten's schedule, in calendar order.
const (
	PhaseMaterial    = "material"
	PhaseRakor       = "rakor"
	PhaseSosialisasi = "sosialisasi"
	PhaseDistribusi  = "distribusi"
	PhasePenutupan   = "penutupan"
)

var Phases = []string{PhaseMaterial, PhaseRakor, PhaseSosialisasi, PhaseDistribusi, PhasePenutupan}

// Assignment kinds.
const (
	KindReviewer = "reviewer"
	KindLeader   = "leader"
)

// Review states of a recipient in the check queue, from its photos.
const (
	StatePending  = "pending"  // has photos not checked yet
	StateRevision = "revision" // has rejected photos, waiting for the field team
	StatePartial  = "partial"  // every photo so far approved, the recipient is not completed yet
	StateApproved = "approved" // completed and every photo approved
)

const dateLayout = "2006-01-02"

// jakarta is the field's clock: "today" and day numbers follow WIB.
var jakarta = func() *time.Location {
	if loc, err := time.LoadLocation("Asia/Jakarta"); err == nil {
		return loc
	}
	return time.FixedZone("WIB", 7*60*60)
}()

var (
	ErrProgramNotFound  = errors.New("program not found")
	ErrScheduleNotFound = errors.New("schedule not found")
	ErrInvalidPhase     = errors.New("phase day is invalid")
	ErrInvalidUser      = errors.New("assigned user is invalid")
	ErrNothingToApprove = errors.New("no photo to approve")
	ErrRevisionEmpty    = errors.New("a revision needs at least one marked photo")
	ErrFlagUnknown      = errors.New("marked photo does not belong to this recipient")
)

type Person struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type PhaseDay struct {
	Day   string `json:"day"`
	Phase string `json:"phase"`
}

type Program struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	FiscalYear int    `json:"fiscal_year"`
	Status     string `json:"status"`
}

// ScheduleRow is one kabupaten of the calendar, with its live numbers.
type ScheduleRow struct {
	ScheduleID   string `json:"schedule_id"`
	Name         string `json:"name"`
	RegencyID    string `json:"regency_id"`
	RegencyName  string `json:"regency_name"`
	ProvinceName string `json:"province_name"`
	ZoneName     string `json:"zone_name"`
	ZoneSort     int    `json:"zone_sort"`
	StartDate    string `json:"start_date"`
	EndDate      string `json:"end_date"`
	Quota        int    `json:"quota"`
	// SheetNo is the row number of the schedule sheet (0: not on it).
	SheetNo   int        `json:"sheet_no"`
	Days      []PhaseDay `json:"days"`
	Reviewers []Person   `json:"reviewers"`
	Leaders   []Person   `json:"leaders"`

	// Today's phase ("" when none) and, during distribution, which
	// distribution day it is out of how many.
	TodayPhase      string `json:"today_phase"`
	DistributionDay int    `json:"distribution_day"`
	DistributionOf  int    `json:"distribution_of"`

	// Recipients: completed, completed today, completed with every photo
	// approved, with photos to check, and with rejected photos.
	Completed      int        `json:"completed"`
	CompletedToday int        `json:"completed_today"`
	Approved       int        `json:"approved"`
	Pending        int        `json:"pending"`
	Revision       int        `json:"revision"`
	PendingPhotos  int        `json:"pending_photos"`
	LastActivityAt *time.Time `json:"last_activity_at"`
}

// Overview is the whole monitoring page of one program on one date.
type Overview struct {
	Programs  []Program     `json:"programs"`
	ProgramID string        `json:"program_id"`
	Date      string        `json:"date"`
	Rakornas  []string      `json:"rakornas"`
	Rows      []ScheduleRow `json:"rows"`
	// Reviewers are the accounts that may check (monitoring.review); Users
	// are all active accounts, offered as field leaders.
	Reviewers []Person `json:"reviewers"`
	Users     []Person `json:"users"`
}

// QueueItem is one recipient of the check queue: every recipient with photos.
type QueueItem struct {
	SlotNumber       int        `json:"slot_number"`
	FullName         string     `json:"full_name"`
	NIK              string     `json:"nik"`
	Village          string     `json:"village"`
	MachineSerial    string     `json:"machine_serial"`
	DistributionDate string     `json:"distribution_date"`
	Status           string     `json:"status"`
	CompletedAt      *time.Time `json:"completed_at"`
	CompletedBy      string     `json:"completed_by"`
	Photos           int        `json:"photos"`
	ApprovedPhotos   int        `json:"approved_photos"`
	RejectedPhotos   int        `json:"rejected_photos"`
	PendingPhotos    int        `json:"pending_photos"`
	State            string     `json:"state"`
	LastUploadAt     *time.Time `json:"last_upload_at"`
	ReviewedBy       string     `json:"reviewed_by"`
	ReviewedAt       *time.Time `json:"reviewed_at"`
}

// MediaReview is the check of one photo.
type MediaReview struct {
	Status     string    `json:"status"`
	Reason     string    `json:"reason"`
	ReviewedBy string    `json:"reviewed_by"`
	ReviewedAt time.Time `json:"reviewed_at"`
}

// SlotDetail is everything the review dialog shows for one recipient.
type SlotDetail struct {
	Slot         distribution.DistributionSlot `json:"slot"`
	ScheduleName string                        `json:"schedule_name"`
	RegencyName  string                        `json:"regency_name"`
	CompletedAt  *time.Time                    `json:"completed_at"`
	CompletedBy  string                        `json:"completed_by"`
	State        string                        `json:"state"`
	// Reviews holds the check of each current photo, by media id; a photo
	// without an entry is not checked yet.
	Reviews map[string]MediaReview `json:"reviews"`
}

// CalendarInput saves the phase days of some kabupaten and the Rakornas days.
type CalendarInput struct {
	ProgramID string        `json:"program_id"`
	Rows      []CalendarRow `json:"rows"`
	Rakornas  *[]string     `json:"rakornas,omitempty"`
}

type CalendarRow struct {
	ScheduleID string     `json:"schedule_id"`
	Days       []PhaseDay `json:"days"`
}

type AssignmentInput struct {
	ScheduleID string   `json:"schedule_id"`
	Reviewers  []string `json:"reviewers"`
	Leaders    []string `json:"leaders"`
}

type FlagInput struct {
	MediaID string `json:"media_id"`
	Reason  string `json:"reason"`
}

type RevisionInput struct {
	Flags []FlagInput `json:"flags"`
	Note  string      `json:"note"`
}

// ApproveInput: the photos to approve; empty means every photo not checked yet.
type ApproveInput struct {
	MediaIDs []string `json:"media_ids"`
}

// cleanDays validates phase days: real dates, known phases, one phase a day.
// The result is sorted by day.
func cleanDays(days []PhaseDay) ([]PhaseDay, error) {
	byDay := map[string]string{}
	for _, d := range days {
		day := strings.TrimSpace(d.Day)
		if _, err := time.Parse(dateLayout, day); err != nil {
			return nil, ErrInvalidPhase
		}
		if !slices.Contains(Phases, d.Phase) {
			return nil, ErrInvalidPhase
		}
		byDay[day] = d.Phase
	}
	out := make([]PhaseDay, 0, len(byDay))
	for day, phase := range byDay {
		out = append(out, PhaseDay{Day: day, Phase: phase})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Day < out[j].Day })
	return out, nil
}

// cleanDates validates and sorts plain dates, without duplicates.
func cleanDates(values []string) ([]string, error) {
	out := []string{}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if _, err := time.Parse(dateLayout, v); err != nil {
			return nil, ErrInvalidPhase
		}
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out, nil
}

// todayPhase gives the phase on date and, for distribution, the day number
// among the distribution days and their count.
func todayPhase(days []PhaseDay, date string) (phase string, day, of int) {
	for _, d := range days {
		if d.Phase == PhaseDistribusi {
			of++
			if d.Day <= date {
				day = of
			}
		}
		if d.Day == date {
			phase = d.Phase
		}
	}
	if phase != PhaseDistribusi {
		day = 0
	}
	return phase, day, of
}

var stageOrder = map[string]int{"mesin": 0, "dokumen": 1, "penyerahan": 2}

// earliestStage is where the field team restarts: the first stage among the
// marked photos (dokumen when none is marked, only a note).
func earliestStage(flags []distribution.RevisionFlag) string {
	stage := ""
	for _, f := range flags {
		if _, ok := stageOrder[f.Stage]; !ok {
			continue
		}
		if stage == "" || stageOrder[f.Stage] < stageOrder[stage] {
			stage = f.Stage
		}
	}
	if stage == "" {
		return "dokumen"
	}
	return stage
}

// revisionReason is the text kept on the reopened slot, read by older apps.
func revisionReason(flags []distribution.RevisionFlag, note string) string {
	parts := []string{}
	for _, f := range flags {
		text := f.Label
		if f.Reason != "" {
			text += ": " + f.Reason
		}
		parts = append(parts, text)
	}
	if note = strings.TrimSpace(note); note != "" {
		parts = append(parts, note)
	}
	return strings.Join(parts, "; ")
}

// reviewState tells where a recipient stands from its photo counts.
func reviewState(photos, approved, rejected int, completed bool) string {
	switch {
	case photos-approved-rejected > 0:
		return StatePending
	case rejected > 0:
		return StateRevision
	case completed:
		return StateApproved
	default:
		return StatePartial
	}
}

func parseDate(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Now().In(jakarta).Format(dateLayout), nil
	}
	if _, err := time.Parse(dateLayout, value); err != nil {
		return "", fmt.Errorf("%w: %s", ErrInvalidPhase, value)
	}
	return value, nil
}
