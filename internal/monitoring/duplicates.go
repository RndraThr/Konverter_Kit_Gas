package monitoring

import (
	"context"
	"fmt"
	"strings"
	"time"

	"konkit/internal/auth"
)

// Equipment serials recorded on more than one slot. Saving one is not
// blocked (the field app only warns, so work is never stopped by an earlier
// typo); the central team follows the clashes here until one side is fixed.

// SerialEntry is one slot that recorded a duplicated serial.
type SerialEntry struct {
	ScheduleID  string     `json:"schedule_id"`
	RegencyName string     `json:"regency_name"`
	SlotNumber  int        `json:"slot_number"`
	Status      string     `json:"status"`
	EnteredBy   string     `json:"entered_by"`
	EnteredAt   *time.Time `json:"entered_at"`
}

// SerialDuplicate is a serial of one kind (mesin, konkit, selang) recorded on several slots.
type SerialDuplicate struct {
	Field   string        `json:"field"`
	Serial  string        `json:"serial"`
	Entries []SerialEntry `json:"entries"`
}

// SerialDuplicates lists every serial of the program recorded on more than one
// slot that is not cancelled, with who last set it. A clash is listed when at
// least one of its slots is in scope; the others show their kabupaten and slot
// number so both teams can check the box.
func (s *Service) SerialDuplicates(ctx context.Context, programID string, scope auth.RegencyScope) ([]SerialDuplicate, error) {
	rows, err := s.pool.Query(ctx, `
		WITH serials AS (
			SELECT d.id, d.schedule_id, ps.regency_id, r.name AS regency, d.slot_number, d.status, f.field, upper(trim(f.value)) AS serial
			FROM distribution_slots d
			JOIN program_schedules ps ON ps.id = d.schedule_id AND ps.program_id::text = $1
			JOIN regencies r ON r.id = ps.regency_id
			CROSS JOIN LATERAL (VALUES ('machine', d.machine_serial_number), ('converter', d.converter_serial_number), ('hose', d.hose_serial_number)) f(field, value)
			WHERE d.status <> 'cancelled' AND COALESCE(trim(f.value), '') <> ''
		), clashes AS (
			SELECT field, serial FROM serials GROUP BY field, serial
			HAVING count(*) > 1 AND bool_or($2 OR regency_id::text = ANY($3))
		)
		SELECT s.field, s.serial, s.schedule_id::text, s.regency, s.slot_number, s.status,
			COALESCE(e.name, ''), e.at
		FROM serials s
		JOIN clashes USING (field, serial)
		LEFT JOIN LATERAL (
			SELECT `+userName("u")+` AS name, a.created_at AS at FROM audit_logs a LEFT JOIN users u ON u.id = a.actor_user_id
			WHERE a.resource_id = s.id::text AND a.action IN ('distribution.equipment_serials_updated', 'distribution.slot_created')
			ORDER BY a.created_at DESC LIMIT 1
		) e ON true
		ORDER BY s.field, s.serial, e.at NULLS FIRST, s.regency, s.slot_number`, strings.TrimSpace(programID), scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list serial duplicates: %w", err)
	}
	defer rows.Close()
	out := []SerialDuplicate{}
	for rows.Next() {
		var field, serial string
		var e SerialEntry
		if err := rows.Scan(&field, &serial, &e.ScheduleID, &e.RegencyName, &e.SlotNumber, &e.Status, &e.EnteredBy, &e.EnteredAt); err != nil {
			return nil, fmt.Errorf("scan serial duplicate: %w", err)
		}
		if n := len(out); n == 0 || out[n-1].Field != field || out[n-1].Serial != serial {
			out = append(out, SerialDuplicate{Field: field, Serial: serial, Entries: []SerialEntry{}})
		}
		out[len(out)-1].Entries = append(out[len(out)-1].Entries, e)
	}
	return out, rows.Err()
}
