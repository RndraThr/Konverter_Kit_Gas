package settings

type Definition struct {
	Type        string
	MaxLength   int
	Allowed     []string
	Description string
}

var Definitions = map[string]Definition{
	"application_name": {
		Type:        "string",
		MaxLength:   120,
		Description: "Nama aplikasi yang tampil pada area administrasi",
	},
	"timezone": {
		Type:        "timezone",
		Allowed:     []string{"Asia/Jakarta", "Asia/Makassar", "Asia/Jayapura"},
		Description: "Zona waktu utama aplikasi",
	},
	"date_format": {
		Type:        "date_format",
		Allowed:     []string{"02/01/2006", "02 January 2006"},
		Description: "Format tanggal utama aplikasi",
	},
	"locale": {
		Type:        "locale",
		Allowed:     []string{"id-ID"},
		Description: "Bahasa dan locale aplikasi",
	},
	"organization_name": {
		Type:        "string",
		MaxLength:   160,
		Description: "Nama organisasi pelaksana",
	},
}
