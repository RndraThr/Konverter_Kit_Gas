package bast

import (
	"sort"
	"strings"
)

func BuildSnapshot(source SourceData) (Snapshot, error) {
	if source.ProgramType != "farmer" {
		return Snapshot{}, ErrTemplateUnavailable
	}
	if strings.TrimSpace(source.DocumentNumber) == "" || strings.TrimSpace(source.LocalDate) == "" || strings.TrimSpace(source.Profile.VersionID) == "" || strings.TrimSpace(source.Recipient.FullName) == "" || strings.TrimSpace(source.Recipient.NIK) == "" || strings.TrimSpace(source.Recipient.SectorIdentifier) == "" {
		return Snapshot{}, ErrInvalidInput
	}
	logos := append([]LogoSnapshot(nil), source.Profile.Logos...)
	sort.SliceStable(logos, func(i, j int) bool { return logos[i].SortOrder < logos[j].SortOrder })
	source.Profile.Logos = logos
	components := append([]ComponentSnapshot(nil), source.Components...)
	for i := range components {
		if strings.TrimSpace(components[i].Label) == "" || components[i].Quantity < 1 || strings.TrimSpace(components[i].Unit) == "" {
			return Snapshot{}, ErrInvalidInput
		}
		components[i].Checked = true
	}
	return Snapshot{ProgramType: source.ProgramType, DocumentNumber: source.DocumentNumber, LocalDate: source.LocalDate, Profile: source.Profile, Recipient: source.Recipient, Equipment: source.Equipment, Components: components, Signatures: SignatureSnapshot{ReceiverName: source.Recipient.FullName, ExecutorName: strings.TrimSpace(source.ExecutorName), SupervisorName: strings.TrimSpace(source.SupervisorName)}}, nil
}
