package monitoring

import (
	"context"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/distribution"
)

// A serial recorded on two slots is listed with both slots and who set it;
// case and spaces do not hide a clash, cancelled slots and single serials are
// not listed, and an account outside the kabupaten does not see it.
func TestSerialDuplicates(t *testing.T) {
	pool := testPool(t)
	f := seed(t, pool)
	ctx := context.Background()
	service := NewService(pool, distribution.NewRepository(pool), nil)

	var second, cancelled string
	must(t, pool.QueryRow(ctx, `INSERT INTO distribution_slots(schedule_id,slot_number,status,machine_serial_number,converter_serial_number) VALUES($1,2,'open',' sn-001 ','CV-ONLY') RETURNING id::text`, f.scheduleID).Scan(&second))
	must(t, pool.QueryRow(ctx, `INSERT INTO distribution_slots(schedule_id,slot_number,status,machine_serial_number) VALUES($1,3,'cancelled','SN-001') RETURNING id::text`, f.scheduleID).Scan(&cancelled))
	_, err := pool.Exec(ctx, `INSERT INTO audit_logs(actor_user_id,action,resource_type,resource_id,metadata) VALUES($1,'distribution.equipment_serials_updated','distribution_slot',$2,'{}')`, f.uploader, second)
	must(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE resource_id = ANY($1)`, []string{second, cancelled})
		_, _ = pool.Exec(context.Background(), `DELETE FROM distribution_slots WHERE id = ANY($1::uuid[])`, []string{second, cancelled})
	})

	list, err := service.SerialDuplicates(ctx, f.programID, all)
	must(t, err)
	if len(list) != 1 || list[0].Field != "machine" || list[0].Serial != "SN-001" || len(list[0].Entries) != 2 {
		t.Fatalf("duplicates = %+v", list)
	}
	byslot := map[int]SerialEntry{}
	for _, e := range list[0].Entries {
		byslot[e.SlotNumber] = e
	}
	if byslot[2].EnteredBy != "Uploader" || byslot[2].EnteredAt == nil || byslot[1].RegencyName == "" {
		t.Fatalf("entries = %+v", list[0].Entries)
	}
	if list, err := service.SerialDuplicates(ctx, f.programID, auth.RegencyScope{RegencyIDs: []string{"00000000-0000-0000-0000-000000000000"}}); err != nil || len(list) != 0 {
		t.Fatalf("out of scope = %+v %v", list, err)
	}
}
