package bast

import (
	"encoding/json"
	"sort"
	"strings"
)

func BuildSnapshot(source SourceData) (Snapshot, error) {
	if source.ProgramType != "farmer" {
		return Snapshot{}, ErrTemplateUnavailable
	}
	if strings.TrimSpace(source.DocumentNumber) == "" || strings.TrimSpace(source.LocalDate) == "" || strings.TrimSpace(source.Recipient.FullName) == "" || strings.TrimSpace(source.Recipient.NIK) == "" || strings.TrimSpace(source.Recipient.SectorIdentifier) == "" {
		return Snapshot{}, ErrInvalidInput
	}
	if len(source.Render.Logos) == 0 {
		return Snapshot{}, ErrBrandingNotConfigured
	}
	logos := append([]LogoSnapshot(nil), source.Render.Logos...)
	sort.SliceStable(logos, func(i, j int) bool { return logos[i].SortOrder < logos[j].SortOrder })
	source.Render.Logos = logos
	components := append([]ComponentSnapshot(nil), source.Components...)
	for i := range components {
		if strings.TrimSpace(components[i].Label) == "" || components[i].Quantity < 1 || strings.TrimSpace(components[i].Unit) == "" {
			return Snapshot{}, ErrInvalidInput
		}
		components[i].Checked = true
	}
	return Snapshot{ProgramType: source.ProgramType, DocumentNumber: source.DocumentNumber, LocalDate: source.LocalDate, Render: source.Render, Recipient: source.Recipient, Equipment: source.Equipment, Components: components, Signatures: SignatureSnapshot{ReceiverName: source.Recipient.FullName, ExecutorName: strings.TrimSpace(source.ExecutorName), SupervisorName: strings.TrimSpace(source.SupervisorName)}}, nil
}

func DecodeSnapshot(raw []byte) (Snapshot, error) {
	var snapshot Snapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return Snapshot{}, err
	}
	if len(snapshot.Render.Logos) == 0 {
		var legacy struct {
			Profile struct {
				Logos []LogoSnapshot `json:"logos"`
			} `json:"profile"`
		}
		if json.Unmarshal(raw, &legacy) == nil && len(legacy.Profile.Logos) > 0 {
			snapshot.Render.Logos = legacy.Profile.Logos
		}
	}
	return snapshot, nil
}
