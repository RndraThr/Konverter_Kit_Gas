package textnorm

import "strings"

// BusinessUpper returns the canonical persisted representation of
// business-facing text.
func BusinessUpper(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}
