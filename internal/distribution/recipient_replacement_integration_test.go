package distribution

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"konkit/internal/auth"

	"github.com/jackc/pgx/v5/pgxpool"
)

// replacementFixture is a minimal schedule with no zone, no date, and no media: enough to exercise
// recipient eligibility and replacement without dragging in the media move pipeline.
type replacementFixture struct {
	scheduleID      string
	slotNumber      int
	slotID          string
	allocationID    string
	personID        string
	nik             string
	regencyID       string
	programID       string
	packageID       string
	documentationID string
}

func seedReplacementSchedule(t *testing.T, pool *pgxpool.Pool) (scheduleID string) {
	t.Helper()
	ctx := context.Background()
	suffix := strings.ReplaceAll(t.Name(), "/", "-") + "-" + fmt.Sprintf("%d", time.Now().UnixNano()%1000000)

	var regencyCode, regencyID string
	must(t, pool.QueryRow(ctx, `
		SELECT code FROM (
			SELECT chr(65+(g%26)::int)||chr(65+((g/26)%26)::int)||chr(65+((g/676)%26)::int) AS code
			FROM generate_series(0,17575) AS g
		) candidates
		WHERE NOT EXISTS (SELECT 1 FROM regencies WHERE document_code=candidates.code)
		ORDER BY code DESC LIMIT 1
	`).Scan(&regencyCode))
	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan',$1,$2,true) RETURNING id::text`, "Replacement "+suffix, regencyCode).Scan(&regencyID))

	var programID string
	must(t, pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Program Test Pengganti','farmer',2026,'active') RETURNING id::text`, "RPL-TEST-"+suffix).Scan(&programID))
	var packageID, documentationID string
	must(t, pool.QueryRow(ctx, `INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES($1,1,'Paket Test Pengganti','farmer','{}'::jsonb,'published') RETURNING id::text`, "PKG-RPL-"+suffix).Scan(&packageID))
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES($1,1,'Dok Test Pengganti','farmer','published') RETURNING id::text`, "DOC-RPL-"+suffix).Scan(&documentationID))
	must(t, pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,receipt_policy_json) VALUES($1,$2,$3,$4,'Jadwal Test Pengganti','2026-01-01','2026-12-31','active','{}'::jsonb) RETURNING id::text`, programID, regencyID, packageID, documentationID).Scan(&scheduleID))

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM distribution_slots WHERE schedule_id=$1`, scheduleID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM package_allocations WHERE schedule_id=$1`, scheduleID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM program_schedules WHERE id=$1`, scheduleID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM programs WHERE id=$1`, programID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM package_template_versions WHERE id=$1`, packageID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM documentation_template_versions WHERE id=$1`, documentationID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM regencies WHERE id=$1`, regencyID)
	})
	return scheduleID
}

// uniqueNIK keeps generated NIKs away from the fixed literals other packages use against the same
// shared konkit_test database, which runs in parallel under go test ./....
func uniqueNIK(offset int64) string {
	return fmt.Sprintf("%016d", (time.Now().UnixNano()+offset)%1e16)
}

// createReplacementCandidate inserts a person, nomination, and allocation in the given state.
func createReplacementCandidate(t *testing.T, pool *pgxpool.Pool, scheduleID, fullName, nik, status string, distributionNumber *int) (personID, allocationID string) {
	t.Helper()
	ctx := context.Background()
	must(t, pool.QueryRow(ctx, `INSERT INTO people(full_name,nik) VALUES($1,$2) RETURNING id::text`, fullName, nik).Scan(&personID))
	var nominationID string
	must(t, pool.QueryRow(ctx, `INSERT INTO candidate_nominations(person_id,program_type,source_snapshot_json,status) VALUES($1,'farmer','{}'::jsonb,'ready') RETURNING id::text`, personID).Scan(&nominationID))
	must(t, pool.QueryRow(ctx, `INSERT INTO package_allocations(schedule_id,nomination_id,intended_person_id,distribution_number,status,package_snapshot_json) VALUES($1,$2,$3,$4,$5,'{}'::jsonb) RETURNING id::text`, scheduleID, nominationID, personID, distributionNumber, status).Scan(&allocationID))
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM people WHERE id=$1`, personID) })
	return personID, allocationID
}

// linkedReplacementSlot creates a slot and links candidate A to it, which is the precondition for
// every replacement path.
func linkedReplacementSlot(t *testing.T, pool *pgxpool.Pool, scheduleID string) replacementFixture {
	t.Helper()
	ctx := context.Background()
	repo := NewRepository(pool)
	created, err := repo.CreateSlot(ctx, auth.Principal{}, CreateSlotInput{ScheduleID: scheduleID}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	must(t, err)

	nik := uniqueNIK(0)
	personID, allocationID := createReplacementCandidate(t, pool, scheduleID, "Penerima Awal", nik, "ready", nil)
	linked, err := repo.LinkSlot(ctx, auth.Principal{}, LinkSlotInput{ScheduleID: scheduleID, SlotNumber: created.SlotNumber, NIK: nik}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	return replacementFixture{
		scheduleID: scheduleID, slotNumber: created.SlotNumber, slotID: linked.ID,
		allocationID: allocationID, personID: personID, nik: nik,
	}
}

func TestSuggestCandidatesHidesCandidatesThatCannotReceive(t *testing.T) {
	pool := distributionIntegrationPool(t)
	ctx := context.Background()
	scheduleID := seedReplacementSchedule(t, pool)

	prefix := uniqueNIK(100)[:10]
	readyNIK := prefix + "000001"
	needsReviewNIK := prefix + "000002"
	cancelledNIK := prefix + "000003"
	replacedNIK := prefix + "000004"
	assignedNIK := prefix + "000005"

	createReplacementCandidate(t, pool, scheduleID, "Siap Terima", readyNIK, "ready", nil)
	createReplacementCandidate(t, pool, scheduleID, "Perlu Tinjau", needsReviewNIK, "needs_review", nil)
	createReplacementCandidate(t, pool, scheduleID, "Dibatalkan", cancelledNIK, "cancelled", nil)
	createReplacementCandidate(t, pool, scheduleID, "Sudah Diganti", replacedNIK, "replaced", nil)
	number := 90
	createReplacementCandidate(t, pool, scheduleID, "Sudah Terpasang", assignedNIK, "ready", &number)

	suggestions, err := NewRepository(pool).SuggestCandidates(ctx, scheduleID, prefix, 20, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, item := range suggestions {
		seen[item.NIK] = true
	}
	if !seen[readyNIK] {
		t.Fatalf("receivable candidate missing from suggestions: %+v", suggestions)
	}
	for name, nik := range map[string]string{
		"needs_review": needsReviewNIK, "cancelled": cancelledNIK, "replaced": replacedNIK, "already assigned": assignedNIK,
	} {
		if seen[nik] {
			t.Fatalf("%s candidate %s must not be suggested", name, nik)
		}
	}
}

func TestSuggestCandidatesHidesPeopleWhoAlreadyReceived(t *testing.T) {
	pool := distributionIntegrationPool(t)
	ctx := context.Background()
	scheduleID := seedReplacementSchedule(t, pool)

	prefix := uniqueNIK(200)[:10]
	nik := prefix + "000001"
	personID, _ := createReplacementCandidate(t, pool, scheduleID, "Pernah Menerima", nik, "ready", nil)
	must(t, func() error {
		_, err := pool.Exec(ctx, `INSERT INTO distribution_slots(schedule_id,slot_number,status,recipient_person_id) VALUES($1,80,'completed',$2)`, scheduleID, personID)
		return err
	}())

	suggestions, err := NewRepository(pool).SuggestCandidates(ctx, scheduleID, prefix, 20, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(suggestions) != 0 {
		t.Fatalf("previously received candidate was suggested: %+v", suggestions)
	}
}

func TestLinkSlotRejectsUnreceivableCandidatesWithoutOverwritingStatus(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status string
		want   error
	}{
		{name: "needs review", status: "needs_review", want: ErrCandidateNeedsReview},
		{name: "cancelled", status: "cancelled", want: ErrCandidateNotAvailable},
		{name: "replaced", status: "replaced", want: ErrCandidateNotAvailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := distributionIntegrationPool(t)
			ctx := context.Background()
			scheduleID := seedReplacementSchedule(t, pool)
			repo := NewRepository(pool)
			created, err := repo.CreateSlot(ctx, auth.Principal{}, CreateSlotInput{ScheduleID: scheduleID}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
			must(t, err)

			nik := uniqueNIK(300)
			_, allocationID := createReplacementCandidate(t, pool, scheduleID, "Tidak Dapat Menerima", nik, tc.status, nil)

			_, err = repo.LinkSlot(ctx, auth.Principal{}, LinkSlotInput{ScheduleID: scheduleID, SlotNumber: created.SlotNumber, NIK: nik}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
			if !errors.Is(err, tc.want) {
				t.Fatalf("link err=%v, want %v", err, tc.want)
			}
			// The whole point of the guard: mounting must never silently resurrect a candidate by
			// rewriting their status to ready.
			var status string
			var number *int
			must(t, pool.QueryRow(ctx, `SELECT status,distribution_number FROM package_allocations WHERE id=$1`, allocationID).Scan(&status, &number))
			if status != tc.status || number != nil {
				t.Fatalf("status=%q number=%v, want untouched %q with no number", status, number, tc.status)
			}
			var slotStatus string
			must(t, pool.QueryRow(ctx, `SELECT status FROM distribution_slots WHERE id=$1`, created.ID).Scan(&slotStatus))
			if slotStatus != "open" {
				t.Fatalf("slot status=%q, want open", slotStatus)
			}
		})
	}
}

func TestReplaceRecipientCreatesAllocationForUnregisteredSubstitute(t *testing.T) {
	pool := distributionIntegrationPool(t)
	ctx := context.Background()
	scheduleID := seedReplacementSchedule(t, pool)
	fixture := linkedReplacementSlot(t, pool, scheduleID)
	repo := NewRepository(pool)

	substituteNIK := uniqueNIK(400)
	replaced, err := repo.ReplaceRecipient(ctx, auth.Principal{}, ReplaceRecipientInput{
		ScheduleID: scheduleID, SlotNumber: fixture.slotNumber, NIK: substituteNIK,
		FullName: "Pengganti Keluarga", Address: "JL. PENGGANTI", SectorIdentifier: "KP" + substituteNIK[8:],
		Reason: "Penerima awal sakit dan diwakilkan keluarga",
	}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if replaced.NIK != substituteNIK || replaced.FullName != "Pengganti Keluarga" {
		t.Fatalf("replaced slot=%+v", replaced)
	}

	var origin, reason string
	must(t, pool.QueryRow(ctx, `SELECT origin,reason FROM recipient_replacements WHERE distribution_slot_id=$1`, fixture.slotID).Scan(&origin, &reason))
	if origin != "new_allocation" || reason != "Penerima awal sakit dan diwakilkan keluarga" {
		t.Fatalf("origin=%q reason=%q", origin, reason)
	}

	// The substitute must end up holding their own allocation carrying the slot's number, because
	// DP3 and BA read the recipient from the allocation's nomination.
	var newPersonID, newNumber string
	var newStatus string
	must(t, pool.QueryRow(ctx, `
		SELECT ds.allocation_id::text, pa.distribution_number::text, pa.status
		FROM distribution_slots ds JOIN package_allocations pa ON pa.id=ds.allocation_id
		WHERE ds.id=$1
	`, fixture.slotID).Scan(&newPersonID, &newNumber, &newStatus))
	if newNumber != fmt.Sprint(fixture.slotNumber) || newStatus != "ready" {
		t.Fatalf("replacement allocation number=%s status=%q", newNumber, newStatus)
	}
	if fixture.allocationID == newPersonID {
		t.Fatalf("slot still points at the original allocation")
	}

	var oldStatus string
	var oldNumber *int
	must(t, pool.QueryRow(ctx, `SELECT status,distribution_number FROM package_allocations WHERE id=$1`, fixture.allocationID).Scan(&oldStatus, &oldNumber))
	if oldStatus != "replaced" || oldNumber != nil {
		t.Fatalf("original allocation status=%q number=%v", oldStatus, oldNumber)
	}

	// The new person exists exactly once, so people_nik_uq can never be violated by a retry.
	var peopleCount int
	must(t, pool.QueryRow(ctx, `SELECT count(*) FROM people WHERE nik=$1`, substituteNIK).Scan(&peopleCount))
	if peopleCount != 1 {
		t.Fatalf("people rows for substitute=%d, want 1", peopleCount)
	}
}

func TestReplaceRecipientRequiresReasonAndRejectsSelfAndAssignedCandidates(t *testing.T) {
	pool := distributionIntegrationPool(t)
	ctx := context.Background()
	scheduleID := seedReplacementSchedule(t, pool)
	fixture := linkedReplacementSlot(t, pool, scheduleID)
	repo := NewRepository(pool)
	scope := auth.RegencyScope{Unrestricted: true}

	if _, err := repo.ReplaceRecipient(ctx, auth.Principal{}, ReplaceRecipientInput{ScheduleID: scheduleID, SlotNumber: fixture.slotNumber, NIK: uniqueNIK(500)}, auth.ClientMeta{}, scope); !errors.Is(err, ErrReplacementReasonRequired) {
		t.Fatalf("blank reason err=%v, want ErrReplacementReasonRequired", err)
	}
	if _, err := repo.ReplaceRecipient(ctx, auth.Principal{}, ReplaceRecipientInput{ScheduleID: scheduleID, SlotNumber: fixture.slotNumber, NIK: fixture.nik, Reason: "Coba pasang ulang penerima yang sama"}, auth.ClientMeta{}, scope); !errors.Is(err, ErrReplacementSameRecipient) {
		t.Fatalf("self replacement err=%v, want ErrReplacementSameRecipient", err)
	}
	if _, err := repo.ReplaceRecipient(ctx, auth.Principal{}, ReplaceRecipientInput{ScheduleID: scheduleID, SlotNumber: fixture.slotNumber, NIK: uniqueNIK(501), FullName: "", Reason: "Pengganti belum terdaftar tanpa nama"}, auth.ClientMeta{}, scope); !errors.Is(err, ErrReplacementNameRequired) {
		t.Fatalf("missing name err=%v, want ErrReplacementNameRequired", err)
	}
}

func TestReplaceRecipientChainsHistoryAndListsIt(t *testing.T) {
	pool := distributionIntegrationPool(t)
	ctx := context.Background()
	scheduleID := seedReplacementSchedule(t, pool)
	fixture := linkedReplacementSlot(t, pool, scheduleID)
	repo := NewRepository(pool)
	scope := auth.RegencyScope{Unrestricted: true}

	secondNIK := uniqueNIK(600)
	if _, err := repo.ReplaceRecipient(ctx, auth.Principal{}, ReplaceRecipientInput{ScheduleID: scheduleID, SlotNumber: fixture.slotNumber, NIK: secondNIK, FullName: "Pengganti Pertama", Reason: "Alasan pertama"}, auth.ClientMeta{}, scope); err != nil {
		t.Fatal(err)
	}
	thirdNIK := uniqueNIK(601)
	if _, err := repo.ReplaceRecipient(ctx, auth.Principal{}, ReplaceRecipientInput{ScheduleID: scheduleID, SlotNumber: fixture.slotNumber, NIK: thirdNIK, FullName: "Pengganti Kedua", Reason: "Alasan kedua"}, auth.ClientMeta{}, scope); err != nil {
		t.Fatal(err)
	}

	history, err := repo.ListReplacements(ctx, scheduleID, fixture.slotNumber, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Fatalf("history=%d entries, want 2: %+v", len(history), history)
	}
	if history[0].Reason != "Alasan pertama" || history[1].Reason != "Alasan kedua" {
		t.Fatalf("history order=%q,%q", history[0].Reason, history[1].Reason)
	}
	if history[0].OldFullName != "Penerima Awal" || history[0].NewFullName != "Pengganti Pertama" || history[1].NewFullName != "Pengganti Kedua" {
		t.Fatalf("history names=%+v", history)
	}
	if history[0].SlotNumber != fixture.slotNumber {
		t.Fatalf("history slot number=%d", history[0].SlotNumber)
	}

	// Only the newest allocation may stay mounted on the slot.
	var mounted string
	must(t, pool.QueryRow(ctx, `SELECT allocation_id::text FROM distribution_slots WHERE id=$1`, fixture.slotID).Scan(&mounted))
	if mounted == fixture.allocationID {
		t.Fatal("slot still points at the first allocation after two replacements")
	}
	var replacedCount int
	must(t, pool.QueryRow(ctx, `
		SELECT count(*) FROM package_allocations pa
		JOIN candidate_nominations cn ON cn.id=pa.nomination_id
		JOIN people p ON p.id=cn.person_id
		WHERE pa.schedule_id=$1 AND pa.status='replaced' AND p.nik IN ($2,$3)
	`, scheduleID, fixture.nik, secondNIK).Scan(&replacedCount))
	if replacedCount != 2 {
		t.Fatalf("replaced allocations=%d, want 2", replacedCount)
	}
}

func TestLookupCandidateClassifiesEveryState(t *testing.T) {
	pool := distributionIntegrationPool(t)
	ctx := context.Background()
	scheduleID := seedReplacementSchedule(t, pool)
	repo := NewRepository(pool)
	scope := auth.RegencyScope{Unrestricted: true}

	prefix := uniqueNIK(700)[:10]
	receivableNIK := prefix + "000001"
	needsReviewNIK := prefix + "000002"
	cancelledNIK := prefix + "000003"
	assignedNIK := prefix + "000004"
	receivedNIK := prefix + "000005"

	createReplacementCandidate(t, pool, scheduleID, "Siap Terima", receivableNIK, "ready", nil)
	createReplacementCandidate(t, pool, scheduleID, "Perlu Tinjau", needsReviewNIK, "needs_review", nil)
	createReplacementCandidate(t, pool, scheduleID, "Dibatalkan", cancelledNIK, "cancelled", nil)
	number := 95
	createReplacementCandidate(t, pool, scheduleID, "Sudah Terpasang", assignedNIK, "ready", &number)
	receivedPersonID, _ := createReplacementCandidate(t, pool, scheduleID, "Pernah Menerima", receivedNIK, "ready", nil)
	must(t, func() error {
		_, err := pool.Exec(ctx, `INSERT INTO distribution_slots(schedule_id,slot_number,status,recipient_person_id) VALUES($1,96,'completed',$2)`, scheduleID, receivedPersonID)
		return err
	}())

	for name, tc := range map[string]struct {
		nik  string
		want string
	}{
		"receivable":          {receivableNIK, CandidateStateReceivable},
		"needs review":        {needsReviewNIK, CandidateStateNeedsReview},
		"cancelled":           {cancelledNIK, CandidateStateNotAvailable},
		"already assigned":    {assignedNIK, CandidateStateAlreadyAssigned},
		"previously received": {receivedNIK, CandidateStatePreviouslyReceived},
		"not registered":      {prefix + "999999", CandidateStateNotRegistered},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := repo.LookupCandidate(ctx, scheduleID, tc.nik, scope)
			if err != nil {
				t.Fatal(err)
			}
			if result.State != tc.want {
				t.Fatalf("state=%q, want %q (%+v)", result.State, tc.want, result)
			}
		})
	}

	assigned, err := repo.LookupCandidate(ctx, scheduleID, assignedNIK, scope)
	if err != nil {
		t.Fatal(err)
	}
	if assigned.DistributionNumber == nil || *assigned.DistributionNumber != number {
		t.Fatalf("assigned lookup=%+v", assigned)
	}
}
