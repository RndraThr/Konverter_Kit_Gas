package bast

import (
	"context"
	"errors"
	"testing"

	"konkit/internal/auth"
)

// profileRepo stores one profile; other repository calls are not expected.
type profileRepo struct {
	pemeriksaanRepository
	profile PemeriksaanProfile
	saved   int
}

func (r *profileRepo) GetPemeriksaanProfile(context.Context, string) (PemeriksaanProfile, error) {
	return r.profile, nil
}

func (r *profileRepo) UpsertPemeriksaanProfile(_ context.Context, _ auth.Principal, p PemeriksaanProfile, _ auth.ClientMeta) (PemeriksaanProfile, error) {
	r.saved++
	r.profile = p
	return p, nil
}

// A BA Pemeriksaan form can be edited or added by any manager, but only a
// super admin can remove one.
func TestOnlySuperAdminRemovesPemeriksaanForm(t *testing.T) {
	ctx := context.Background()
	repo := &profileRepo{profile: defaultPemeriksaanProfile("p")}
	service := NewPemeriksaanService(repo, nil)
	manager, superAdmin := auth.Principal{Roles: []string{"leader_tim_02"}}, auth.Principal{Roles: []string{"super_admin"}}

	edited := defaultPemeriksaanProfile("p")
	edited.Forms[0].Title = "Mesin Pompa Air"
	if _, err := service.SaveProfile(ctx, manager, edited, auth.ClientMeta{}); err != nil {
		t.Fatalf("edit: %v", err)
	}
	fewer := defaultPemeriksaanProfile("p")
	fewer.Forms = fewer.Forms[:len(fewer.Forms)-1]
	if _, err := service.SaveProfile(ctx, manager, fewer, auth.ClientMeta{}); !errors.Is(err, ErrFormDeleteForbidden) {
		t.Fatalf("manager remove: %v", err)
	}
	if repo.saved != 1 || len(repo.profile.Forms) != 7 {
		t.Fatalf("saved %d forms %d", repo.saved, len(repo.profile.Forms))
	}
	if _, err := service.SaveProfile(ctx, superAdmin, fewer, auth.ClientMeta{}); err != nil || len(repo.profile.Forms) != 6 {
		t.Fatalf("super admin remove: %v (%d forms)", err, len(repo.profile.Forms))
	}
}

// resolverRepo serves a template with an LPG tank, a zone PO and a kabupaten PO.
type resolverRepo struct{ own map[string]string }

func (resolverRepo) GetScheduleTemplateEntries(context.Context, string) ([]TemplateEntry, error) {
	return TemplateEntriesFromValues([]byte(`{"components":[{"code":"lpg_tank","label":"Tabung LPG 3 kg","brand":"Pertamina"},{"code":"oil","label":"Oli","brand":"Enduro"}]}`))
}
func (resolverRepo) GetScheduleItemSelection(context.Context, string) (map[string]string, error) {
	return map[string]string{}, nil
}
func (resolverRepo) GetZonePONumbers(context.Context, string) (map[string]string, error) {
	return map[string]string{"component|lpg_tank": "PO-ZONA", "component|oil": "PO-OLI"}, nil
}
func (r resolverRepo) GetSchedulePONumbers(context.Context, string) (map[string]string, error) {
	return r.own, nil
}

// The kabupaten's own No. PO replaces the zone's for that item only.
func TestKabupatenPOOverridesZonePO(t *testing.T) {
	ctx := context.Background()
	_, resolved, _, err := resolveScheduleItems(ctx, resolverRepo{own: map[string]string{"component|lpg_tank": "LO-WONOGIRI-01"}}, DP3Context{ScheduleID: "s", ZoneID: "z"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resolved["component:lpg_tank"].PONumber != "LO-WONOGIRI-01" || resolved["component:oil"].PONumber != "PO-OLI" {
		t.Fatalf("resolved = %+v", resolved)
	}
	_, resolved, _, _ = resolveScheduleItems(ctx, resolverRepo{own: map[string]string{}}, DP3Context{ScheduleID: "s", ZoneID: "z"}, nil)
	if resolved["component:lpg_tank"].PONumber != "PO-ZONA" {
		t.Fatalf("without kabupaten PO = %+v", resolved["component:lpg_tank"])
	}
}
