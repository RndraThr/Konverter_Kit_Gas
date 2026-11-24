package finalreport

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type sourcesFixture struct{ programID, regencyID string }

// seedSources: one kabupaten schedule with BA Pemeriksaan Mesin finalized on
// two dates (plus an older superseded version) and Selang Hisap & Buang on
// one, no Tabung Gas; Rakorda and Sosialisasi locations in the BA settings;
// two titik serah with completed recipients and a third used only by a
// recipient not handed over yet.
func seedSources(t *testing.T, pool *pgxpool.Pool) sourcesFixture {
	t.Helper()
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var cleanups []string
	t.Cleanup(func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			_, _ = pool.Exec(context.Background(), cleanups[i])
		}
	})
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	var f sourcesFixture
	var code, pkg, doc, scheduleID string
	must(pool.QueryRow(ctx, `SELECT chr(65+(g%26)::int)||chr(65+((g/26)%26)::int)||chr(65+((g/676)%26)::int) FROM generate_series(0,17575) g
		WHERE NOT EXISTS (SELECT 1 FROM regencies WHERE document_code=chr(65+(g%26)::int)||chr(65+((g/26)%26)::int)||chr(65+((g/676)%26)::int)) ORDER BY g DESC LIMIT 1`).Scan(&code))
	must(pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code) VALUES('Jambi',$1,$2) RETURNING id::text`, "KAB. SUMBER "+suffix, code).Scan(&f.regencyID))
	must(pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Program Sumber','farmer',2026,'active') RETURNING id::text`, "SRC-"+suffix).Scan(&f.programID))
	must(pool.QueryRow(ctx, `INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES($1,1,'Paket','farmer','{}','published') RETURNING id::text`, "PKG-SRC-"+suffix).Scan(&pkg))
	must(pool.QueryRow(ctx, `INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES($1,1,'Dok','farmer','published') RETURNING id::text`, "DOC-SRC-"+suffix).Scan(&doc))
	must(pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,slot_quota)
		VALUES($1,$2,$3,$4,'KAB. SUMBER','2026-01-01','2026-12-31','active',10) RETURNING id::text`, f.programID, f.regencyID, pkg, doc).Scan(&scheduleID))
	cleanups = append(cleanups,
		fmt.Sprintf(`DELETE FROM regencies WHERE id='%s'`, f.regencyID),
		fmt.Sprintf(`DELETE FROM programs WHERE id='%s'`, f.programID),
		fmt.Sprintf(`DELETE FROM package_template_versions WHERE id='%s'`, pkg),
		fmt.Sprintf(`DELETE FROM documentation_template_versions WHERE id='%s'`, doc),
		fmt.Sprintf(`DELETE FROM program_schedules WHERE id='%s'`, scheduleID),
		fmt.Sprintf(`DELETE FROM bast_handover_locations WHERE regency_id='%s'`, f.regencyID),
		fmt.Sprintf(`DELETE FROM distribution_slots WHERE schedule_id='%s'`, scheduleID),
		fmt.Sprintf(`DELETE FROM bast_aggregate_documents WHERE schedule_id='%s'`, scheduleID),
	)

	_, err := pool.Exec(ctx, `INSERT INTO bast_schedule_settings(schedule_id, handover_location, rakorda_location, sosialisasi_location)
		VALUES($1, 'Kolom lama', 'Aula Kantor Bupati', 'Balai Desa Sukamaju')`, scheduleID)
	must(err)

	ba := func(form, date, status string, version int) {
		_, err := pool.Exec(ctx, `INSERT INTO bast_aggregate_documents(schedule_id,program_id,regency_id,document_type,document_date,filename,recipient_count,page_count,version,status,checksum,snapshot_json)
			VALUES($1,$2,$3,$4,$5,'BA.pdf',1,1,$6,$7,$8,'{}')`, scheduleID, f.programID, f.regencyID, "pemeriksaan:"+form, date, version, status, strings.Repeat("a", 64))
		must(err)
	}
	ba("mesin", "2026-11-03", "superseded", 1) // older version, not counted
	ba("mesin", "2026-11-05", "active", 2)
	ba("mesin", "2026-11-07", "active", 1)
	ba("selang_hisap_buang", "2026-11-04", "stale", 1)

	location := func(name string) string {
		var id string
		must(pool.QueryRow(ctx, `INSERT INTO bast_handover_locations(regency_id,name,name_key) VALUES($1,$2,upper($2)) RETURNING id::text`, f.regencyID, name).Scan(&id))
		return id
	}
	balai, gudang, belum := location("Balai Desa Rantau"), location("Gudang Sukamaju"), location("Lapangan Belum")
	for i, l := range []struct{ location, status string }{{gudang, "completed"}, {balai, "completed"}, {balai, "completed"}, {belum, "open"}} {
		var completedAt any
		if l.status == "completed" {
			completedAt = "2026-11-20T03:00:00Z"
		}
		_, err := pool.Exec(ctx, `INSERT INTO distribution_slots(schedule_id,slot_number,status,handover_location_id,distribution_date,completed_at)
			VALUES($1,$2,$3,$4,'2026-11-20',$5)`, scheduleID, i+1, l.status, l.location, completedAt)
		must(err)
	}
	return f
}

func TestPreviewTakesDatesAndLocationsFromBeritaAcara(t *testing.T) {
	pool := testPool(t)
	f := seedSources(t, pool)
	svc := NewService(NewRepository(pool), stubProgramContexts{}, nil, nil)
	preview, err := svc.Preview(context.Background(), f.programID, f.regencyID, unrestrictedScope())
	if err != nil {
		t.Fatal(err)
	}
	checks := map[string][2]string{
		"selang":       {preview.DeliveryHoseDate, "4 November 2026"},
		"mesin":        {preview.DeliveryMachineGroupDate, "5 November 2026"},
		"tabung":       {preview.DeliveryGasCylinderDate, "Belum diisi"},
		"range start":  {preview.DeliveryRangeStart, "4 November 2026"},
		"range end":    {preview.DeliveryRangeEnd, "5 November 2026"},
		"rakorda":      {preview.RakordaLocation, "Aula Kantor Bupati"},
		"sosialisasi":  {preview.SosialisasiLocation, "Balai Desa Sukamaju"},
		"titik serah":  {preview.HandoverLocation, "Balai Desa Rantau, Gudang Sukamaju"},
		"team (unset)": {preview.LeaderCount, "Belum diisi"},
	}
	for name, c := range checks {
		if c[0] != c[1] {
			t.Errorf("%s = %q, want %q", name, c[0], c[1])
		}
	}
}
