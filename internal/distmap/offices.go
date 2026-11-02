package distmap

import (
	"context"
	"fmt"

	"konkit/internal/auth"
)

// AgricultureOffice is a kabupaten's Dinas Pertanian office location.
type AgricultureOffice struct {
	RegencyID    string
	RegencyName  string
	ProvinceName string
	Lat, Lon     float64
}

// AgricultureOffices returns the located Dinas Pertanian offices of the active kabupaten in scope.
func (r *Repository) AgricultureOffices(ctx context.Context, scope auth.RegencyScope) ([]AgricultureOffice, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, name, province_name, agriculture_office_latitude::float8, agriculture_office_longitude::float8
		FROM regencies
		WHERE is_active AND agriculture_office_latitude IS NOT NULL AND ($1 OR id::text = ANY($2))
		ORDER BY province_name, name
	`, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("query agriculture offices: %w", err)
	}
	defer rows.Close()
	offices := []AgricultureOffice{}
	for rows.Next() {
		var office AgricultureOffice
		if err := rows.Scan(&office.RegencyID, &office.RegencyName, &office.ProvinceName, &office.Lat, &office.Lon); err != nil {
			return nil, fmt.Errorf("scan agriculture office: %w", err)
		}
		offices = append(offices, office)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agriculture offices: %w", err)
	}
	return offices, nil
}
