package bast

import (
	"regexp"
	"strconv"
	"strings"
)

var trailingNumberPattern = regexp.MustCompile(`(\d+)\s*$`)

// zoneWithWords menambahkan bilangan terbilang di belakang nomor zona:
// "Zona 1" -> "Zona 1 (Satu)". Nama tanpa nomor di akhir tidak diubah.
func zoneWithWords(zoneName string) string {
	zoneName = strings.TrimSpace(zoneName)
	match := trailingNumberPattern.FindStringSubmatch(zoneName)
	if match == nil {
		return zoneName
	}
	number, err := strconv.Atoi(match[1])
	if err != nil || number <= 0 || number > 999 {
		return zoneName
	}
	return zoneName + " (" + titleCaseWords(indonesianNumberWords(number)) + ")"
}

var indonesianDigits = [...]string{"", "satu", "dua", "tiga", "empat", "lima", "enam", "tujuh", "delapan", "sembilan", "sepuluh", "sebelas"}

// indonesianNumberWords menuliskan 1–999 dalam kata (huruf kecil).
func indonesianNumberWords(n int) string {
	switch {
	case n < 12:
		return indonesianDigits[n]
	case n < 20:
		return indonesianDigits[n-10] + " belas"
	case n < 100:
		return strings.TrimSpace(indonesianDigits[n/10] + " puluh " + indonesianNumberWords(n%10))
	case n < 200:
		return strings.TrimSpace("seratus " + indonesianNumberWords(n-100))
	default:
		return strings.TrimSpace(indonesianDigits[n/100] + " ratus " + indonesianNumberWords(n%100))
	}
}
