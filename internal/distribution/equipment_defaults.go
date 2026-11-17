package distribution

import "encoding/json"

// defaultEquipmentCodes returns the option of each equipment list that a new
// slot starts with: the one marked default in the package template, or the
// only option. An empty code means the officer still has to choose.
func defaultEquipmentCodes(packageValues []byte) (machine, converter, hose string) {
	var values struct {
		Machines   []equipmentDefaultOption `json:"machine_options"`
		Converters []equipmentDefaultOption `json:"converter_options"`
		Hoses      []equipmentDefaultOption `json:"hose_options"`
	}
	if len(packageValues) == 0 || json.Unmarshal(packageValues, &values) != nil {
		return "", "", ""
	}
	return defaultOptionCode(values.Machines), defaultOptionCode(values.Converters), defaultOptionCode(values.Hoses)
}

type equipmentDefaultOption struct {
	Code      string `json:"code"`
	IsDefault bool   `json:"is_default"`
}

func defaultOptionCode(options []equipmentDefaultOption) string {
	for _, option := range options {
		if option.IsDefault && option.Code != "" {
			return option.Code
		}
	}
	if len(options) == 1 {
		return options[0].Code
	}
	return ""
}
