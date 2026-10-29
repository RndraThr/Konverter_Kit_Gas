package distmap

import (
	"context"
	"errors"
	"strings"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/recipients/recipienttest"
)

// zoneEnv adds two further regencies, each linked to a copy of a real boundary, to the map fixture
// so zones have geometry to union.
type zoneEnv struct {
	*mapEnv
	regencyA, regencyB string
}

func newZoneEnv(t *testing.T) *zoneEnv {
	t.Helper()
	env := newMapEnv(t)
	z := &zoneEnv{mapEnv: env}
	// Two other real boundaries (Cirebon, Majalengka) stand in for two kabupaten of one zone.
	for i, name := range []string{"Cirebon", "Majalengka"} {
		regencyID := env.scalar(t, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Jawa Barat',$1,$2,true) RETURNING id::text`, "Zona Test "+name, []string{"ZTA", "ZTB"}[i])
		env.exec(t, `INSERT INTO regency_boundaries(boundary_name,normalized_key,regency_id,geom,source,license,asset_checksum)
			SELECT 'Distmap Zone '||$2::text,'DISTMAPZONE'||upper($2::text),$1,geom,source,license,'distmap-test' FROM regency_boundaries WHERE boundary_name=$2::text LIMIT 1`, regencyID, name)
		if i == 0 {
			z.regencyA = regencyID
		} else {
			z.regencyB = regencyID
		}
	}
	t.Cleanup(func() {
		c := context.Background()
		_, _ = env.pool.Exec(c, `DELETE FROM program_regency_assignments WHERE regency_id = ANY($1::uuid[])`, []string{z.regencyA, z.regencyB})
		_, _ = env.pool.Exec(c, `DELETE FROM regency_boundaries WHERE regency_id = ANY($1::uuid[])`, []string{z.regencyA, z.regencyB})
		_, _ = env.pool.Exec(c, `DELETE FROM regencies WHERE id = ANY($1::uuid[])`, []string{z.regencyA, z.regencyB})
	})
	return z
}

func (z *zoneEnv) assign(t *testing.T, regencyID, zoneID string) {
	t.Helper()
	z.exec(t, `INSERT INTO program_regency_assignments(program_id,regency_id,zone_id) VALUES($1,$2,$3)
		ON CONFLICT (program_id,regency_id) DO UPDATE SET zone_id=EXCLUDED.zone_id`, z.fx.ProgramID, regencyID, zoneID)
}

func (z *zoneEnv) zone(t *testing.T, code, name string, sortOrder int, placeholder bool) string {
	t.Helper()
	id := z.scalar(t, `INSERT INTO program_zones(program_id,code,name,sort_order,is_placeholder) VALUES($1,$2,$3,$4,$5) RETURNING id::text`, z.fx.ProgramID, code, name, sortOrder, placeholder)
	t.Cleanup(func() {
		_, _ = z.pool.Exec(context.Background(), `DELETE FROM program_regency_assignments WHERE zone_id=$1`, id)
		_, _ = z.pool.Exec(context.Background(), `DELETE FROM program_zones WHERE id=$1`, id)
	})
	return id
}

func (z *zoneEnv) area(t *testing.T, regencyID string) float64 {
	t.Helper()
	var a float64
	recipienttest.Must(t, z.pool.QueryRow(context.Background(), `SELECT ST_Area(geom::geography) FROM regency_boundaries WHERE regency_id=$1`, regencyID).Scan(&a))
	return a
}

func (z *zoneEnv) geometryArea(t *testing.T, geojson []byte) float64 {
	t.Helper()
	var a float64
	recipienttest.Must(t, z.pool.QueryRow(context.Background(), `SELECT ST_Area(ST_SetSRID(ST_GeomFromGeoJSON($1),4326)::geography)`, string(geojson)).Scan(&a))
	return a
}

func zoneByID(zones []ZoneBoundary, id string) ZoneBoundary {
	for _, z := range zones {
		if z.ZoneID == id {
			return z
		}
	}
	return ZoneBoundary{}
}

func TestZoneBoundaries(t *testing.T) {
	z := newZoneEnv(t)
	ctx := context.Background()
	all := auth.RegencyScope{Unrestricted: true}

	zoneA := z.fx.ZoneID // ZONA-TEST, owns the fixture's farmer regency (Indramayu copy)
	placeholder := z.zone(t, "ZTPH", "Belum ada zona", 99, true)
	empty := z.zone(t, "ZTEMPTY", "Zona Kosong", 3, false)
	zoneB := z.zone(t, "ZTB", "Zona B", 2, false)
	z.assign(t, z.regencyA, zoneB)
	z.assign(t, z.regencyB, zoneB)

	zones, err := z.repo.ZoneBoundaries(ctx, z.fx.ProgramID, all)
	if err != nil {
		t.Fatal(err)
	}
	b := zoneByID(zones, zoneB)
	if b.RegencyCount != 2 || b.Code != "ZTB" || b.Name != "Zona B" || b.IsPlaceholder {
		t.Fatalf("zone B must be one feature over two kabupaten: %+v", b)
	}
	// Union of two disjoint regencies, simplified by 0.005 degrees (~500 m): the area stays within a
	// few percent of the sum of both.
	want := z.area(t, z.regencyA) + z.area(t, z.regencyB)
	got := z.geometryArea(t, b.Geometry)
	if got < want*0.95 || got > want*1.05 {
		t.Fatalf("zone B area %.0f is not about the sum of its kabupaten %.0f", got, want)
	}
	if a := zoneByID(zones, zoneA); a.RegencyCount != 1 {
		t.Fatalf("zone A must hold the fixture regency: %+v", a)
	}
	for _, id := range []string{placeholder, empty} {
		if zoneByID(zones, id).ZoneID != "" {
			t.Fatal("a zone without kabupaten has no geometry and must not be returned")
		}
	}

	// A placeholder zone with a kabupaten is reported as such; the change is visible after invalidation.
	z.assign(t, z.regencyB, placeholder)
	z.repo.InvalidateZones(z.fx.ProgramID)
	zones, err = z.repo.ZoneBoundaries(ctx, z.fx.ProgramID, all)
	if err != nil {
		t.Fatal(err)
	}
	if p := zoneByID(zones, placeholder); !p.IsPlaceholder || p.RegencyCount != 1 {
		t.Fatalf("placeholder zone must carry is_placeholder: %+v", p)
	}
	if b := zoneByID(zones, zoneB); b.RegencyCount != 1 {
		t.Fatalf("zone B lost one kabupaten after invalidation: %+v", b)
	}
}

func TestZoneBoundariesCacheIsInvalidatedPerProgram(t *testing.T) {
	z := newZoneEnv(t)
	ctx := context.Background()
	all := auth.RegencyScope{Unrestricted: true}
	zoneB := z.zone(t, "ZTB", "Zona B", 2, false)
	z.assign(t, z.regencyA, zoneB)

	first, err := z.repo.ZoneBoundaries(ctx, z.fx.ProgramID, all)
	if err != nil {
		t.Fatal(err)
	}
	if b := zoneByID(first, zoneB); b.RegencyCount != 1 {
		t.Fatalf("unexpected first result %+v", b)
	}

	// Changed behind the cache's back: still the cached answer.
	z.assign(t, z.regencyB, zoneB)
	cached, _ := z.repo.ZoneBoundaries(ctx, z.fx.ProgramID, all)
	if b := zoneByID(cached, zoneB); b.RegencyCount != 1 {
		t.Fatalf("result must be served from the cache, got %+v", b)
	}
	// Invalidating another program leaves it alone.
	z.repo.InvalidateZones("00000000-0000-0000-0000-000000000000")
	again, _ := z.repo.ZoneBoundaries(ctx, z.fx.ProgramID, all)
	if b := zoneByID(again, zoneB); b.RegencyCount != 1 {
		t.Fatal("invalidating another program must not drop this program's cache")
	}
	z.repo.InvalidateZones(z.fx.ProgramID)
	fresh, _ := z.repo.ZoneBoundaries(ctx, z.fx.ProgramID, all)
	if b := zoneByID(fresh, zoneB); b.RegencyCount != 2 {
		t.Fatalf("after InvalidateZones the new assignment must be visible, got %+v", b)
	}
}

func TestZoneBoundariesRespectScope(t *testing.T) {
	z := newZoneEnv(t)
	ctx := context.Background()
	zoneB := z.zone(t, "ZTB", "Zona B", 2, false)
	z.assign(t, z.regencyA, zoneB)
	z.assign(t, z.regencyB, zoneB)

	scoped, err := z.repo.ZoneBoundaries(ctx, z.fx.ProgramID, auth.RegencyScope{RegencyIDs: []string{z.regencyA}})
	if err != nil {
		t.Fatal(err)
	}
	if len(scoped) != 1 || scoped[0].ZoneID != zoneB || scoped[0].RegencyCount != 1 {
		t.Fatalf("a scoped user must only get zones and kabupaten inside their scope: %+v", scoped)
	}
	if want := z.area(t, z.regencyA); z.geometryArea(t, scoped[0].Geometry) > want*1.05 {
		t.Fatal("scoped zone geometry must not include out-of-scope kabupaten")
	}
	// The scoped result must not poison the unrestricted one (separate cache entries).
	full, _ := z.repo.ZoneBoundaries(ctx, z.fx.ProgramID, auth.RegencyScope{Unrestricted: true})
	if b := zoneByID(full, zoneB); b.RegencyCount != 2 {
		t.Fatalf("unrestricted result must see both kabupaten, got %+v", b)
	}
}

func TestRegencyBoundary(t *testing.T) {
	env := newMapEnv(t)
	ctx := context.Background()
	raw, err := env.repo.RegencyBoundary(ctx, env.fx.FarmerRegencyID, unrestricted)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(raw), `{"type":"Feature","geometry":{`) {
		t.Fatalf("expected a GeoJSON Feature, got %.80s", raw)
	}
	if _, err := env.repo.RegencyBoundary(ctx, env.fx.OtherRegencyID, unrestricted); !errors.Is(err, ErrBoundaryNotFound) {
		t.Fatalf("regency without boundary: want ErrBoundaryNotFound, got %v", err)
	}
	if _, err := env.repo.RegencyBoundary(ctx, env.fx.FarmerRegencyID, auth.RegencyScope{RegencyIDs: []string{env.fx.OtherRegencyID}}); !errors.Is(err, ErrBoundaryNotFound) {
		t.Fatalf("out-of-scope regency: want ErrBoundaryNotFound, got %v", err)
	}
}
