package monitoring

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"konkit/internal/auth"
)

// DashboardStages is how far the recipients of a kabupaten, a zone or the
// whole program have gone, against the quota.
type DashboardStages struct {
	Quota     int `json:"quota"`
	Linked    int `json:"linked"`    // slots with a recipient (linked or completed)
	Completed int `json:"completed"` // handed over
	Approved  int `json:"approved"`  // completed with every photo approved
	BAFinal   int `json:"ba_final"`  // completed with a final BA Perorangan
}

func (s *DashboardStages) add(o DashboardStages) {
	s.Quota += o.Quota
	s.Linked += o.Linked
	s.Completed += o.Completed
	s.Approved += o.Approved
	s.BAFinal += o.BAFinal
}

// DashboardRegency is one kabupaten schedule of the program.
type DashboardRegency struct {
	ScheduleID   string `json:"schedule_id"`
	RegencyID    string `json:"regency_id"`
	RegencyName  string `json:"regency_name"`
	ProvinceName string `json:"province_name"`
	ZoneName     string `json:"zone_name"` // "" = no zone
	ZoneSort     int    `json:"zone_sort"`
	DashboardStages
}

// DashboardZone sums the kabupaten of one zone.
type DashboardZone struct {
	ZoneName string `json:"zone_name"`
	ZoneSort int    `json:"zone_sort"`
	DashboardStages
}

// DashboardDay is the number of recipients handed over on one date (WIB).
type DashboardDay struct {
	Date      string `json:"date"`
	Completed int    `json:"completed"`
}

// DashboardActivity is the number of active activity photos and videos of one type.
type DashboardActivity struct {
	ActivityType string `json:"activity_type"`
	Media        int    `json:"media"`
}

// Dashboard is the super admin's charts of one program.
type Dashboard struct {
	Programs   []Program           `json:"programs"`
	ProgramID  string              `json:"program_id"`
	Totals     DashboardStages     `json:"totals"`
	Zones      []DashboardZone     `json:"zones"`
	Regencies  []DashboardRegency  `json:"regencies"`
	Daily      []DashboardDay      `json:"daily"`
	Activities []DashboardActivity `json:"activities"`
}

// pickProgram lists the programs and resolves programID, defaulting to the
// newest active one. programID is "" only when there is no program.
func (s *Service) pickProgram(ctx context.Context, programID string) ([]Program, string, error) {
	programs := []Program{}
	rows, err := s.pool.Query(ctx, `SELECT id::text, name, fiscal_year, status FROM programs WHERE status <> 'archived'
		ORDER BY (status = 'active') DESC, fiscal_year DESC, name`)
	if err != nil {
		return programs, "", fmt.Errorf("list programs: %w", err)
	}
	for rows.Next() {
		var p Program
		if err := rows.Scan(&p.ID, &p.Name, &p.FiscalYear, &p.Status); err != nil {
			rows.Close()
			return programs, "", fmt.Errorf("scan program: %w", err)
		}
		programs = append(programs, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return programs, "", fmt.Errorf("list programs: %w", err)
	}
	programID = strings.TrimSpace(programID)
	if programID == "" && len(programs) > 0 {
		programID = programs[0].ID
	}
	if !slices.ContainsFunc(programs, func(p Program) bool { return p.ID == programID }) {
		if programID == "" {
			return programs, "", nil
		}
		return programs, "", ErrProgramNotFound
	}
	return programs, programID, nil
}

// Dashboard counts each stage per kabupaten, zone and day, and the activity
// documentation, of one program (default: the newest active one).
func (s *Service) Dashboard(ctx context.Context, programID string, scope auth.RegencyScope) (Dashboard, error) {
	out := Dashboard{Zones: []DashboardZone{}, Regencies: []DashboardRegency{}, Daily: []DashboardDay{}, Activities: []DashboardActivity{}}
	var err error
	if out.Programs, out.ProgramID, err = s.pickProgram(ctx, programID); err != nil || out.ProgramID == "" {
		return out, err
	}

	rows, err := s.pool.Query(ctx, `
		WITH photos AS (`+photoCounts+`)
		SELECT ps.id::text, r.id::text, r.name, r.province_name, COALESCE(z.name, ''), COALESCE(z.sort_order, 0),
			COALESCE(ps.slot_quota, 0),
			count(ds.id) FILTER (WHERE ds.allocation_id IS NOT NULL AND ds.status IN ('linked', 'completed')),
			count(ds.id) FILTER (WHERE ds.status = 'completed'),
			count(ds.id) FILTER (WHERE ds.status = 'completed' AND m.photos > 0 AND m.approved = m.photos),
			count(ds.id) FILTER (WHERE ds.status = 'completed' AND EXISTS (
				SELECT 1 FROM bast_individual_documents d WHERE d.distribution_slot_id = ds.id AND d.status = 'final'))
		FROM program_schedules ps
		JOIN regencies r ON r.id = ps.regency_id
		LEFT JOIN program_regency_assignments pra ON pra.program_id = ps.program_id AND pra.regency_id = ps.regency_id
		LEFT JOIN program_zones z ON z.id = pra.zone_id
		LEFT JOIN schedule_sheet_rows sr ON sr.schedule_id = ps.id
		LEFT JOIN distribution_slots ds ON ds.schedule_id = ps.id
		LEFT JOIN photos m ON m.slot_id = ds.id
		WHERE ps.program_id = $1 AND ps.status <> 'cancelled' AND ($2 OR ps.regency_id::text = ANY($3))
		GROUP BY ps.id, r.id, z.name, z.sort_order, sr.sort_order
		ORDER BY z.name IS NULL, z.sort_order, z.name, sr.sort_order NULLS LAST, r.province_name, r.name`,
		out.ProgramID, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return out, fmt.Errorf("list dashboard schedules: %w", err)
	}
	for rows.Next() {
		var row DashboardRegency
		if err := rows.Scan(&row.ScheduleID, &row.RegencyID, &row.RegencyName, &row.ProvinceName, &row.ZoneName, &row.ZoneSort,
			&row.Quota, &row.Linked, &row.Completed, &row.Approved, &row.BAFinal); err != nil {
			rows.Close()
			return out, fmt.Errorf("scan dashboard schedule: %w", err)
		}
		out.Regencies = append(out.Regencies, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("list dashboard schedules: %w", err)
	}
	for _, row := range out.Regencies {
		out.Totals.add(row.DashboardStages)
		i := slices.IndexFunc(out.Zones, func(z DashboardZone) bool { return z.ZoneName == row.ZoneName })
		if i < 0 {
			out.Zones = append(out.Zones, DashboardZone{ZoneName: row.ZoneName, ZoneSort: row.ZoneSort})
			i = len(out.Zones) - 1
		}
		out.Zones[i].add(row.DashboardStages)
	}

	rows, err = s.pool.Query(ctx, `
		SELECT (ds.completed_at AT TIME ZONE 'Asia/Jakarta')::date::text, count(*)
		FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id = ds.schedule_id
		WHERE ps.program_id = $1 AND ps.status <> 'cancelled' AND ($2 OR ps.regency_id::text = ANY($3))
			AND ds.status = 'completed' AND ds.completed_at IS NOT NULL
		GROUP BY 1 ORDER BY 1`, out.ProgramID, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return out, fmt.Errorf("list dashboard days: %w", err)
	}
	for rows.Next() {
		var day DashboardDay
		if err := rows.Scan(&day.Date, &day.Completed); err != nil {
			rows.Close()
			return out, fmt.Errorf("scan dashboard day: %w", err)
		}
		out.Daily = append(out.Daily, day)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("list dashboard days: %w", err)
	}

	rows, err = s.pool.Query(ctx, `
		SELECT activity_type, count(*) FROM activity_media
		WHERE program_id = $1 AND status = 'active' AND ($2 OR regency_id::text = ANY($3))
		GROUP BY activity_type ORDER BY count(*) DESC, activity_type`, out.ProgramID, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return out, fmt.Errorf("list dashboard activities: %w", err)
	}
	for rows.Next() {
		var activity DashboardActivity
		if err := rows.Scan(&activity.ActivityType, &activity.Media); err != nil {
			rows.Close()
			return out, fmt.Errorf("scan dashboard activity: %w", err)
		}
		out.Activities = append(out.Activities, activity)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("list dashboard activities: %w", err)
	}
	return out, nil
}
