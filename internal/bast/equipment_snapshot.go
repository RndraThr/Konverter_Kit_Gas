package bast

import "encoding/json"

func equipmentFromVerificationSnapshot(raw []byte) (EquipmentSnapshot, bool, error) {
	var verification struct {
		Equipment *EquipmentSnapshot `json:"equipment"`
	}
	if err := json.Unmarshal(raw, &verification); err != nil {
		return EquipmentSnapshot{}, false, err
	}
	if verification.Equipment == nil {
		return EquipmentSnapshot{}, false, nil
	}
	return *verification.Equipment, true, nil
}
