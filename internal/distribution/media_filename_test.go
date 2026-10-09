package distribution

import "testing"

func TestFormatDistributionFinalMediaFilename(t *testing.T) {
	tests := []struct {
		name          string
		recipientName string
		label         string
		mimeType      string
		sequence      int
		maxFiles      int
		want          string
	}{
		{
			name:          "prefixes the recipient name for multi-file slots",
			recipientName: "Ahmad",
			label:         "Foto Mesin",
			mimeType:      "image/jpeg",
			sequence:      1,
			maxFiles:      3,
			want:          "AHMAD - FOTO MESIN - 01.jpg",
		},
		{
			name:          "drops the sequence for single-file slots",
			recipientName: "Siti Aminah",
			label:         "Foto Penerima",
			mimeType:      "image/png",
			sequence:      1,
			maxFiles:      1,
			want:          "SITI AMINAH - FOTO PENERIMA.png",
		},
		{
			name:          "keeps the upload-time name when the recipient is unknown",
			recipientName: "   ",
			label:         "Foto Mesin",
			mimeType:      "image/jpeg",
			sequence:      2,
			maxFiles:      2,
			want:          "FOTO MESIN - 02.jpg",
		},
		{
			name:          "replaces path separators in both segments",
			recipientName: "Ahmad/Subur",
			label:         "Foto Mesin/Lapangan",
			mimeType:      "video/mp4",
			sequence:      3,
			maxFiles:      5,
			want:          "AHMAD-SUBUR - FOTO MESIN-LAPANGAN - 03.mp4",
		},
		{
			name:          "never leaks a NIK-shaped digit run into the name",
			recipientName: "Ahmad",
			label:         "Foto Mesin",
			mimeType:      "image/webp",
			sequence:      10,
			maxFiles:      10,
			want:          "AHMAD - FOTO MESIN - 10.webp",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatDistributionFinalMediaFilename(tt.recipientName, tt.label, tt.mimeType, tt.sequence, tt.maxFiles); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
