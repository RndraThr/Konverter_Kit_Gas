package textnorm

import "strings"

// BusinessUpper returns the canonical persisted representation of
// business-facing text.
func BusinessUpper(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

// ItemText merapikan teks barang/merek dari Template Paket (spasi ganda dan
// spasi di tepi) tanpa mengubah huruf besar-kecil: merek, nama barang, dan
// satuan dicetak di BA persis seperti diketik admin sesuai EYD.
func ItemText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
