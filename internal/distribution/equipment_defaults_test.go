package distribution

import "testing"

func TestDefaultEquipmentCodes(t *testing.T) {
	machine, converter, hose := defaultEquipmentCodes([]byte(`{
		"machine_options": [{"code": "shark", "is_default": true}, {"code": "honda"}],
		"converter_options": [{"code": "ergas"}],
		"hose_options": [{"code": "a"}, {"code": "b"}]
	}`))
	if machine != "shark" || converter != "ergas" || hose != "" {
		t.Fatalf("defaults = %q %q %q", machine, converter, hose)
	}
	if m, c, h := defaultEquipmentCodes([]byte(`not json`)); m != "" || c != "" || h != "" {
		t.Fatalf("invalid template gave %q %q %q", m, c, h)
	}
}
