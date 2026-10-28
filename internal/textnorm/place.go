package textnorm

import (
	"regexp"
	"strings"
)

var placeKeySeparators = regexp.MustCompile(`[^A-Z0-9]+`)

// PlaceKey is the comparison key of a place name (titik serah): upper case,
// with every run of spaces and punctuation as one space. "Balai Ds. Tempe "
// and "balai ds tempe" share one key. Matches the SQL of migration 00079:
// btrim(regexp_replace(upper(name), '[^A-Z0-9]+', ' ', 'g')).
func PlaceKey(value string) string {
	return strings.TrimSpace(placeKeySeparators.ReplaceAllString(strings.ToUpper(value), " "))
}

// PlaceName tidies a place name as typed: single spaces, no edge spaces.
func PlaceName(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
