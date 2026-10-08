package bast

import (
	"context"
	"slices"
	"strings"

	"konkit/internal/auth"
)

type itemResolverRepository interface {
	GetScheduleTemplateEntries(context.Context, string) ([]TemplateEntry, error)
	GetScheduleItemSelection(context.Context, string) (map[string]string, error)
	GetZonePONumbers(context.Context, string) (map[string]string, error)
}

// resolveScheduleItems memilih merk tiap barang template jadwal untuk
// kabupaten, memakai merk mesin dominan dari distribusi selesai dan No. PO
// zona kabupaten.
func resolveScheduleItems(ctx context.Context, repository itemResolverRepository, ctxData DP3Context, closingRows []ClosingKabupatenRow) ([]TemplateEntry, map[string]ResolvedEntry, string, error) {
	entries, err := repository.GetScheduleTemplateEntries(ctx, ctxData.ScheduleID)
	if err != nil {
		return nil, nil, "", err
	}
	selection, err := repository.GetScheduleItemSelection(ctx, ctxData.ScheduleID)
	if err != nil {
		return nil, nil, "", err
	}
	zoneID := ctxData.ZoneID
	if ctxData.ZonePlaceholder {
		zoneID = ""
	}
	poNumbers, err := repository.GetZonePONumbers(ctx, zoneID)
	if err != nil {
		return nil, nil, "", err
	}
	machineBrand := dominantMachineBrand(closingRows)
	return entries, resolveEntries(entries, selection, machineBrand, poNumbers), machineBrand, nil
}

type itemRepository interface {
	itemResolverRepository
	GetDP3Context(context.Context, string, auth.RegencyScope) (DP3Context, error)
	ListClosingKabupatenRows(context.Context, string, auth.RegencyScope) ([]ClosingKabupatenRow, error)
	GetTKDNProfile(context.Context, string) (TKDNProfile, error)
	GetPemeriksaanProfile(context.Context, string) (PemeriksaanProfile, error)
	UpsertScheduleItemSelection(context.Context, auth.Principal, string, map[string]string, auth.ClientMeta) error
	ListProgramZoneTemplates(context.Context, string) ([]ZoneTemplateValues, error)
	ReplaceZonePONumbers(context.Context, auth.Principal, string, string, []ZonePOInput, auth.ClientMeta) error
}

// ItemService melayani merk & No. PO per jadwal dan pengisian No. PO per zona.
type ItemService struct {
	repository itemRepository
}

func NewItemService(repository itemRepository) *ItemService {
	return &ItemService{repository: repository}
}

// ScheduleItems mengembalikan merk terpilih dan No. PO setiap barang yang
// dirujuk susunan TKDN atau form Pemeriksaan untuk kabupaten jadwal.
func (s *ItemService) ScheduleItems(ctx context.Context, scheduleID string, scope auth.RegencyScope) (ScheduleItems, error) {
	ctxData, err := s.repository.GetDP3Context(ctx, strings.TrimSpace(scheduleID), scope)
	if err != nil {
		return ScheduleItems{}, err
	}
	rows, err := s.repository.ListClosingKabupatenRows(ctx, ctxData.ScheduleID, scope)
	if err != nil {
		return ScheduleItems{}, err
	}
	_, resolved, machineBrand, err := resolveScheduleItems(ctx, s.repository, ctxData, rows)
	if err != nil {
		return ScheduleItems{}, err
	}
	tkdn, err := s.repository.GetTKDNProfile(ctx, ctxData.ProgramID)
	if err != nil {
		return ScheduleItems{}, err
	}
	pemeriksaan, err := s.repository.GetPemeriksaanProfile(ctx, ctxData.ProgramID)
	if err != nil {
		return ScheduleItems{}, err
	}
	order, names, inspected := referencedRefs(tkdn.Rows, pemeriksaan.Forms)
	choices := []ScheduleItemChoice{}
	for _, ref := range order {
		r, ok := resolved[ref]
		if !ok {
			continue
		}
		variants := make([]VariantRef, 0, len(r.Entry.Variants))
		for _, v := range r.Entry.Variants {
			variants = append(variants, VariantRef{Code: v.Code, Brand: v.Brand})
		}
		choices = append(choices, ScheduleItemChoice{Ref: ref, Name: names[ref], Variants: variants, Selected: r.Variant.Code, Source: r.Source, PONumber: r.PONumber, Inspected: inspected[ref]})
	}
	zoneName := ctxData.ZoneName
	if ctxData.ZonePlaceholder {
		zoneName = ""
	}
	return ScheduleItems{ScheduleID: ctxData.ScheduleID, ProgramID: ctxData.ProgramID, ZoneName: zoneName, MachineBrand: machineBrand, Items: choices}, nil
}

// SaveScheduleSelection menyimpan pilihan merk manual. Barang tanpa pilihan
// kembali ke pemilihan otomatis.
func (s *ItemService) SaveScheduleSelection(ctx context.Context, actor auth.Principal, scheduleID string, selection map[string]string, scope auth.RegencyScope, meta auth.ClientMeta) (ScheduleItems, error) {
	ctxData, err := s.repository.GetDP3Context(ctx, strings.TrimSpace(scheduleID), scope)
	if err != nil {
		return ScheduleItems{}, err
	}
	entries, err := s.repository.GetScheduleTemplateEntries(ctx, ctxData.ScheduleID)
	if err != nil {
		return ScheduleItems{}, err
	}
	clean, err := normalizeItemSelection(entries, selection)
	if err != nil {
		return ScheduleItems{}, err
	}
	if err := s.repository.UpsertScheduleItemSelection(ctx, actor, ctxData.ScheduleID, clean, meta); err != nil {
		return ScheduleItems{}, err
	}
	return s.ScheduleItems(ctx, ctxData.ScheduleID, scope)
}

// ZonePORow adalah satu barang+merk yang dapat diberi No. PO di sebuah zona.
type ZonePORow struct {
	Kind     string `json:"kind"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	Brand    string `json:"brand"`
	PONumber string `json:"po_number"`
}

type ZonePO struct {
	ZoneID   string      `json:"zone_id"`
	ZoneName string      `json:"zone_name"`
	Rows     []ZonePORow `json:"rows"`
}

// ProgramZonePO mengembalikan, per zona, barang+merk yang diperiksa di BA
// Pemeriksaan (dari template paket jadwal zona tersebut) dan No. PO-nya.
func (s *ItemService) ProgramZonePO(ctx context.Context, programID string) ([]ZonePO, error) {
	programID = strings.TrimSpace(programID)
	if programID == "" {
		return nil, ErrInvalidInput
	}
	zones, err := s.repository.ListProgramZoneTemplates(ctx, programID)
	if err != nil {
		return nil, err
	}
	pemeriksaan, err := s.repository.GetPemeriksaanProfile(ctx, programID)
	if err != nil {
		return nil, err
	}
	order, names, _ := referencedRefs(nil, pemeriksaan.Forms)
	result := make([]ZonePO, 0, len(zones))
	for _, zone := range zones {
		numbers, err := s.repository.GetZonePONumbers(ctx, zone.ZoneID)
		if err != nil {
			return nil, err
		}
		var templates [][]TemplateEntry
		for _, raw := range zone.Values {
			entries, err := TemplateEntriesFromValues(raw)
			if err != nil {
				return nil, err
			}
			templates = append(templates, entries)
		}
		rows := []ZonePORow{}
		seen := map[string]bool{}
		for _, ref := range order {
			for _, entries := range templates {
				entry, ok := entryByRef(entries, ref)
				if !ok {
					continue
				}
				for _, variant := range entry.Variants {
					key := poKey(entry.Kind, variant.Code)
					if seen[key] {
						continue
					}
					seen[key] = true
					rows = append(rows, ZonePORow{Kind: entry.Kind, Code: variant.Code, Name: names[ref], Brand: variant.Brand, PONumber: numbers[key]})
				}
			}
		}
		result = append(result, ZonePO{ZoneID: zone.ZoneID, ZoneName: zone.ZoneName, Rows: rows})
	}
	return result, nil
}

var poKinds = []string{ItemKindMachine, ItemKindConverter, ItemKindHoseSuction, ItemKindHoseDischarge, ItemKindComponent}

// SaveZonePO mengganti seluruh No. PO satu zona.
func (s *ItemService) SaveZonePO(ctx context.Context, actor auth.Principal, programID, zoneID string, numbers []ZonePOInput, meta auth.ClientMeta) ([]ZonePO, error) {
	programID, zoneID = strings.TrimSpace(programID), strings.TrimSpace(zoneID)
	if programID == "" || zoneID == "" || len(numbers) > 500 {
		return nil, ErrInvalidInput
	}
	clean := []ZonePOInput{}
	for _, number := range numbers {
		number.Kind, number.Code, number.PONumber = strings.TrimSpace(number.Kind), strings.TrimSpace(number.Code), strings.TrimSpace(number.PONumber)
		if number.PONumber == "" {
			continue
		}
		if !slices.Contains(poKinds, number.Kind) || number.Code == "" || len(number.PONumber) > 100 ||
			slices.ContainsFunc(clean, func(c ZonePOInput) bool { return c.Kind == number.Kind && c.Code == number.Code }) {
			return nil, ErrInvalidInput
		}
		clean = append(clean, number)
	}
	if err := s.repository.ReplaceZonePONumbers(ctx, actor, programID, zoneID, clean, meta); err != nil {
		return nil, err
	}
	return s.ProgramZonePO(ctx, programID)
}
