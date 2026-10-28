package geo

import (
	"regexp"
	"strings"
)

var (
	regencyPrefix   = regexp.MustCompile(`^(KABUPATEN|KAB\.?)\s+`)
	regencyNonAlnum = regexp.MustCompile(`[^A-Z0-9]`)
)

// NormalizeRegencyName ports normalizeRegencyName from the frontend (mapRegions.ts) so master
// regency names and boundary names reduce to the same key. Only the KABUPATEN/KAB prefix is
// dropped; KOTA stays so cities and same-named regencies remain distinct.
func NormalizeRegencyName(name string) string {
	upper := strings.ToUpper(name)
	upper = regencyPrefix.ReplaceAllString(upper, "")
	return regencyNonAlnum.ReplaceAllString(upper, "")
}
