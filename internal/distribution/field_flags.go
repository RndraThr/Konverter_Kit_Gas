package distribution

import (
	"context"
	"fmt"
)

// ReviewableField is recipient or equipment data the central monitoring can
// send back for revision, with the stage (POS) that holds it.
type ReviewableField struct {
	Label string
	Stage string
}

// ReviewableFields by their JSON name in DistributionSlot.
var ReviewableFields = map[string]ReviewableField{
	"full_name":               {Label: "Nama", Stage: "dokumen"},
	"nik":                     {Label: "NIK", Stage: "dokumen"},
	"sector_identifier":       {Label: "Nomor kartu tani / KUSUKA", Stage: "dokumen"},
	"phone_number":            {Label: "Nomor HP", Stage: "dokumen"},
	"address":                 {Label: "Alamat", Stage: "dokumen"},
	"village":                 {Label: "Desa/kelurahan", Stage: "dokumen"},
	"district":                {Label: "Kecamatan", Stage: "dokumen"},
	"machine_serial_number":   {Label: "Nomor seri mesin", Stage: "mesin"},
	"converter_serial_number": {Label: "Nomor seri konkit", Stage: "mesin"},
}

// FieldValue is the slot's current value of a reviewable field.
func (s DistributionSlot) FieldValue(field string) string {
	switch field {
	case "full_name":
		return s.FullName
	case "nik":
		return s.NIK
	case "sector_identifier":
		return s.SectorIdentifier
	case "phone_number":
		return s.PhoneNumber
	case "address":
		return s.Address
	case "village":
		return s.Village
	case "district":
		return s.District
	case "machine_serial_number":
		return s.MachineSerialNumber
	case "converter_serial_number":
		return s.ConverterSerialNumber
	}
	return ""
}

// fieldFlags lists the slot's data the central monitoring sent back that
// still holds the value it marked (a corrected field is no longer flagged).
func (r *Repository) fieldFlags(ctx context.Context, slot DistributionSlot) ([]RevisionFlag, error) {
	rows, err := r.pool.Query(ctx, `SELECT field, value_snapshot, reason FROM slot_field_reviews WHERE distribution_slot_id = $1::uuid ORDER BY reviewed_at, field`, slot.ID)
	if err != nil {
		return nil, fmt.Errorf("list field revision flags: %w", err)
	}
	defer rows.Close()
	var flags []RevisionFlag
	for rows.Next() {
		var field, snapshot, reason string
		if err := rows.Scan(&field, &snapshot, &reason); err != nil {
			return nil, fmt.Errorf("scan field revision flag: %w", err)
		}
		def, known := ReviewableFields[field]
		if !known || slot.FieldValue(field) != snapshot {
			continue
		}
		flags = append(flags, RevisionFlag{Field: field, SlotCode: field, Label: def.Label, Stage: def.Stage, Reason: reason})
	}
	return flags, rows.Err()
}
