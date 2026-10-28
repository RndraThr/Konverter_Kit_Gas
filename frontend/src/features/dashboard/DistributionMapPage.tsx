import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useSearchParams } from 'react-router-dom';
import { Globe2, MapPinned, Maximize2, X } from 'lucide-react';
import { apiRequest } from '../../lib/api';
import { PageHeader } from '../../components/PageHeader';
import { DataState } from '../../components/DataState';
import { Button } from '../../components/ui/button';
import { Card, CardContent, CardHeader } from '../../components/ui/card';
import { loadRegencyAsset } from './mapAsset';
import { normalizeRegencyName, type RegencyAsset } from './mapRegions';
import type { MapData, MapRegion } from './types';
import { MapView, type CameraRequest } from './map/MapView';
import { MapFilters } from './map/MapFilters';
import { MapSearch } from './map/MapSearch';
import { KpiStrip, MilestoneLegend } from './map/MapSummary';
import { MilestoneFilter } from './map/MilestoneFilter';
import { ProgressByZone } from './map/ProgressByZone';
import { Segmented } from './map/Segmented';
import { TimelinePlayer } from './map/TimelinePlayer';
import { ZoneBreadcrumb } from './map/ZoneBreadcrumb';
import { DETAIL_PANEL_WIDTH, INDONESIA_BOUNDS, assetRegencyBounds, fitPadding, geometryBounds, initialFitTarget, pointsBounds, regencyBoundsIndex } from './map/fitTargets';
import { buildRegionFills, groupSites, zoneColorIndex, PLACEHOLDER_ZONE_LABEL } from './map/layerData';
import { progressByZone, stageCounts } from './map/progress';
import type { SearchResult } from './map/search';
import { filterPoints, timelineRange } from './map/timeline';
import { useMapConfig } from './map/useMapConfig';
import { FILTER_KEYS, filterQuery, useMapPoints } from './map/useMapPoints';
import { useMapSelection } from './map/useMapSelection';
import { useMapUrlState } from './map/useMapUrlState';
import { useRegencyBoundary, useZoneBoundaries } from './map/useZoneBoundaries';
import type { BasemapMode, Bounds, ColorMode, MapSelection, PointCollection, PointFeature, ZoneGeometry } from './map/types';

type ScheduleOption = { id: string; name: string };
type RegencyOption = { id: string; name: string; document_code: string };
type ProgramOption = { id: string; name: string; program_type: string };
type Focus = { regencyId?: string; geometry: ZoneGeometry | null; name: string };

const emptyRegions: MapRegion[] = [];
const emptyPoints: PointFeature[] = [];
const emptyCollection: PointCollection = { type: 'FeatureCollection', features: [] };
const SELECT_ZOOM = 15;

const basemapOptions: Array<{ value: BasemapMode; label: string }> = [{ value: 'map', label: 'Biasa' }, { value: 'sat', label: 'Satelit' }, { value: 'hyb', label: 'Hybrid' }];
const colorOptions: Array<{ value: ColorMode; label: string }> = [{ value: 'zone', label: 'Zona' }, { value: 'evidence', label: 'Bukti lengkap' }, { value: 'progress', label: 'Diserahkan' }, { value: 'recipients', label: 'Sebaran' }];

function assetGeometry(asset: RegencyAsset | null, name: string): ZoneGeometry | null {
  const key = normalizeRegencyName(name);
  const feature = asset?.regencies.find((item) => normalizeRegencyName(item.n) === key);
  return feature ? { type: 'MultiPolygon', coordinates: feature.r.map((ring) => [ring]) } : null;
}

function selectionLabel(selected: MapSelection, points: PointFeature[]): string {
  if (selected.kind === 'point') {
    const point = points.find((item) => item.properties.slot_id === selected.slotId);
    return `Titik terpilih: ${point ? `Nomor bagi ${point.properties.code}` : 'nomor bagi'}`;
  }
  const members = points.filter((item) => item.properties.site_id === selected.siteId);
  return `Titik serah terpilih: ${members[0]?.properties.site_name || `${members.length} titik`}`;
}

export function DistributionMapPage() {
  const [params, setParams] = useSearchParams();
  const { state: view, update } = useMapUrlState();
  const filter = filterQuery(params);
  const programId = params.get('program_id') || undefined;
  const zoneId = params.get('zone_id') || undefined;
  const regencyFilter = params.get('regency_id') || undefined;

  const [asset, setAsset] = useState<RegencyAsset | null>(null);
  const [assetError, setAssetError] = useState(false);
  // The boundary asset (~440 kB) is a separate chunk fetched only on this page; it feeds regency
  // search, fit bounds and the regency fills.
  useEffect(() => {
    let cancelled = false;
    void loadRegencyAsset().then((loaded) => { if (!cancelled) setAsset(loaded); }).catch(() => { if (!cancelled) setAssetError(true); });
    return () => { cancelled = true; };
  }, []);

  const aggregate = useQuery({
    queryKey: ['recipients', 'map', filter],
    queryFn: () => apiRequest<{ data: MapData }>(`/api/v1/recipients/map${filter ? `?${filter}` : ''}`),
  });
  const regencies = useQuery({ queryKey: ['program-setup', 'regencies'], queryFn: () => apiRequest<{ data: RegencyOption[] }>('/api/v1/program-setup/regencies') });
  const programs = useQuery({ queryKey: ['program-setup', 'programs'], queryFn: () => apiRequest<{ data: ProgramOption[] }>('/api/v1/program-setup/programs') });
  const schedules = useQuery({ queryKey: ['program-setup', 'schedules'], queryFn: () => apiRequest<{ data: ScheduleOption[] }>('/api/v1/program-setup/schedules') });
  const pointsQuery = useMapPoints(filter);
  const config = useMapConfig();
  const zonesQuery = useZoneBoundaries(programId);
  const zones = programId ? zonesQuery.data ?? null : null;

  const regions = aggregate.data?.data.regions ?? emptyRegions;
  const allPoints = pointsQuery.data?.features ?? emptyPoints;
  const searchRegencies = useMemo(() => (asset ? assetRegencyBounds(asset) : []), [asset]);
  const boundsIndex = useMemo(() => (asset ? regencyBoundsIndex(asset, regions) : new Map<string, Bounds>()), [asset, regions]);
  const regencyNames = useMemo(() => new Map(regions.map((region) => [region.regency_id, region.regency_name])), [regions]);

  // Timeline (null = live) and milestone filter, both client-side over the loaded points.
  const range = useMemo(() => timelineRange(allPoints), [allPoints]);
  const [day, setDay] = useState<number | null>(null);
  const timelineDay = range && day !== null ? Math.min(day, range.days - 1) : null;
  const dayPoints = useMemo(() => filterPoints(allPoints, { range, day: timelineDay }), [allPoints, range, timelineDay]);
  const counts = useMemo(() => Object.fromEntries(stageCounts(dayPoints).map((count, i) => [i + 1, count])), [dayPoints]);
  const visiblePoints = useMemo(() => (view.stage ? dayPoints.filter((point) => point.properties.stage === view.stage) : dayPoints), [dayPoints, view.stage]);

  const [show, setShow] = useState({ zones: true, points: true });
  const [focus, setFocus] = useState<Focus | null>(null);
  const boundary = useRegencyBoundary(focus?.regencyId);
  const zoneFeature = zones?.features.find((zone) => zone.properties.zone_id === zoneId);
  const focusGeometry = focus ? boundary.data?.geometry ?? focus.geometry : zoneFeature?.geometry ?? null;

  const zoneColors = useMemo(() => zoneColorIndex(zones), [zones]);
  const regionFills = useMemo(() => buildRegionFills(asset, regions, view.color, zoneColors), [asset, regions, view.color, zoneColors]);
  const layers = useMemo(() => ({
    points: show.points ? visiblePoints : emptyPoints,
    zones: show.zones ? zones : null,
    regions: regionFills,
    colorMode: view.color,
    focus: focusGeometry,
  }), [show, visiblePoints, zones, regionFills, view.color, focusGeometry]);

  const { selected, select } = useMapSelection(view, update);
  const [camera, setCamera] = useState<CameraRequest | null>(null);
  const cameraSeq = useRef(0);
  const panelWidth = selected ? DETAIL_PANEL_WIDTH : 0;
  const fit = useCallback((bounds: Bounds) => {
    cameraSeq.current += 1;
    setCamera({ id: cameraSeq.current, kind: 'fit', bounds, padding: fitPadding(window.innerWidth, panelWidth) });
  }, [panelWidth]);
  const fly = (center: [number, number]) => {
    cameraSeq.current += 1;
    setCamera({ id: cameraSeq.current, kind: 'fly', center, zoom: SELECT_ZOOM });
  };

  const scopeIds = useMemo(() => (regencyFilter ? [regencyFilter] : regions.map((region) => region.regency_id)), [regencyFilter, regions]);
  const homeTarget = () => initialFitTarget(pointsQuery.data ?? emptyCollection, scopeIds, boundsIndex);
  // Opening view: a shared link's position, else the points, else the user's regencies, else Indonesia.
  // The link position is read once at mount: later c/z values are our own moveend writes and must not
  // be mistaken for a shared link.
  const linkCamera = useRef(view.center && view.zoom !== undefined ? { center: view.center, zoom: view.zoom } : null);
  const initialDone = useRef(false);
  useEffect(() => {
    if (initialDone.current) return;
    const link = linkCamera.current;
    if (link) {
      initialDone.current = true;
      cameraSeq.current += 1;
      setCamera({ id: cameraSeq.current, kind: 'jump', center: link.center, zoom: link.zoom });
      return;
    }
    if (pointsQuery.isPending || aggregate.isPending || (!asset && !assetError)) return;
    initialDone.current = true;
    fit(initialFitTarget(pointsQuery.data ?? emptyCollection, scopeIds, boundsIndex));
  }, [pointsQuery.isPending, pointsQuery.data, aggregate.isPending, asset, assetError, scopeIds, boundsIndex, fit]);
  // Moves before the opening camera is requested (e.g. resize() of the default view) are not written
  // to the URL, so the default position never overwrites a pending fit or the shared link.
  const onMove = (center: [number, number], zoom: number) => {
    if (initialDone.current) update({ center, zoom });
  };

  const focusRegency = (region: MapRegion) => {
    setFocus({ regencyId: region.regency_id, geometry: assetGeometry(asset, region.regency_name), name: region.regency_name });
    const bounds = boundsIndex.get(region.regency_id);
    if (bounds) fit(bounds);
  };

  const setFilter = (key: string, value: string) => {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value); else next.delete(key);
    if (key === 'program_id') next.delete('zone_id'); // zones belong to one program
    setParams(next, { replace: true });
    if (key === 'regency_id') {
      const region = regions.find((item) => item.regency_id === value);
      const option = regencies.data?.data.find((item) => item.id === value);
      if (region) focusRegency(region);
      else if (option) {
        // Not in the current aggregate (no data yet): still zoom to it through the boundary asset.
        setFocus({ regencyId: option.id, geometry: assetGeometry(asset, option.name), name: option.name });
        const bounds = searchRegencies.find((item) => item.key === normalizeRegencyName(option.name))?.bounds;
        if (bounds) fit(bounds);
      } else setFocus(null);
    }
    if (key === 'zone_id') {
      setFocus(null);
      const zone = zones?.features.find((item) => item.properties.zone_id === value);
      const bounds = zone ? geometryBounds(zone.geometry) : null;
      if (bounds) fit(bounds);
    }
  };
  const activeFilterCount = FILTER_KEYS.filter((key) => Boolean(params.get(key))).length;
  const resetFilters = () => {
    const next = new URLSearchParams(params);
    FILTER_KEYS.forEach((key) => next.delete(key));
    setParams(next, { replace: true });
    setFocus(null);
  };

  const selectPoint = (slotId: string) => {
    select({ kind: 'point', slotId });
    const point = allPoints.find((item) => item.properties.slot_id === slotId);
    if (point) fly(point.geometry.coordinates);
  };
  const selectSite = (siteId: string) => {
    select({ kind: 'site', siteId });
    const members = groupSites(allPoints).get(siteId);
    if (members?.length) fly(members[0].geometry.coordinates);
  };
  const onSearch = (result: SearchResult) => {
    if (result.kind === 'point') { selectPoint(result.id); return; }
    const region = regions.find((item) => normalizeRegencyName(item.regency_name) === result.id);
    setFocus({ regencyId: region?.regency_id, geometry: assetGeometry(asset, result.label), name: region?.regency_name ?? result.label });
    fit(result.bounds);
  };

  const focusedRegion = focus?.regencyId ? regions.find((item) => item.regency_id === focus.regencyId) : regions.find((item) => item.regency_id === regencyFilter);
  const crumbZoneId = zoneId ?? focusedRegion?.zone_id;
  const crumbZone = zones?.features.find((zone) => zone.properties.zone_id === crumbZoneId);
  const crumbZoneName = crumbZone ? (crumbZone.properties.is_placeholder ? PLACEHOLDER_ZONE_LABEL : crumbZone.properties.name) : focusedRegion?.zone_name || undefined;
  const toIndonesia = () => {
    const next = new URLSearchParams(params);
    next.delete('zone_id');
    next.delete('regency_id');
    setParams(next, { replace: true });
    setFocus(null);
    fit(INDONESIA_BOUNDS);
  };
  const toZone = () => {
    const next = new URLSearchParams(params);
    next.delete('regency_id');
    if (crumbZoneId) next.set('zone_id', crumbZoneId);
    setParams(next, { replace: true });
    setFocus(null);
    const bounds = crumbZone ? geometryBounds(crumbZone.geometry) : null;
    if (bounds) fit(bounds);
  };

  const [mapAttempt, setMapAttempt] = useState(0);
  const [styleFailed, setStyleFailed] = useState(false);
  const groups = useMemo(() => progressByZone(allPoints, regions, Boolean(programId)), [allPoints, regions, programId]);
  const zoneLegend = view.color === 'zone' && zones ? zones.features.map((zone) => ({ name: zone.properties.is_placeholder ? PLACEHOLDER_ZONE_LABEL : zone.properties.name, color: zoneColors.get(zone.properties.zone_id) ?? '' })) : undefined;

  let mapBody;
  if (config.isError || styleFailed) {
    // Task 7 replaces this with the light (SVG) map.
    mapBody = <div className="absolute inset-0 overflow-auto p-6"><DataState kind="error" title="Peta dasar tidak dapat dimuat" description="Periksa koneksi, lalu coba memuat peta kembali." action={{ label: 'Coba lagi', onClick: () => { setStyleFailed(false); setMapAttempt((n) => n + 1); void config.refetch(); } }} /></div>;
  } else if (!config.data) {
    mapBody = <div className="absolute inset-0 p-6"><DataState kind="loading" title="Memuat peta" description="Menyiapkan peta dasar." /></div>;
  } else {
    mapBody = <MapView
      key={`${config.data.style_url}|${config.data.attribution}|${mapAttempt}`}
      config={config.data}
      basemap={view.base}
      layers={layers}
      camera={camera}
      onMove={onMove}
      onStyleError={() => setStyleFailed(true)}
      onPoint={selectPoint}
      onSite={selectSite}
    />;
  }

  return <div className="space-y-6">
    <PageHeader title="Map Distribusi" description="Titik distribusi per nomor bagi, milestone penyaluran, dan zona program." />
    <KpiStrip points={pointsQuery.data} />

    <section aria-label="Ruang kerja Map Distribusi" className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_320px]">
      <div className="min-w-0 space-y-4">
        <Card className="gap-0 overflow-hidden py-0 shadow-none">
          <CardHeader className="space-y-3 border-b py-4">
            <div className="flex flex-wrap items-center gap-2">
              <MapSearch regencies={searchRegencies} points={allPoints} regencyNames={regencyNames} onPick={onSearch} />
              <Segmented label="Jenis peta" value={view.base} options={basemapOptions} onChange={(base) => update({ base })} />
              <Segmented label="Mode warna" value={view.color} options={colorOptions} onChange={(color) => update({ color })} />
            </div>
            <div className="flex flex-wrap items-center justify-between gap-2">
              <ZoneBreadcrumb zoneName={crumbZoneName} regencyName={focus?.name ?? focusedRegion?.regency_name} onIndonesia={toIndonesia} onZone={toZone} />
              <div role="group" aria-label="Lapisan peta" className="flex gap-1">
                <Button type="button" size="sm" variant={show.zones ? 'secondary' : 'outline'} aria-pressed={show.zones} onClick={() => setShow((s) => ({ ...s, zones: !s.zones }))}>Batas zona</Button>
                <Button type="button" size="sm" variant={show.points ? 'secondary' : 'outline'} aria-pressed={show.points} onClick={() => setShow((s) => ({ ...s, points: !s.points }))}>Titik</Button>
              </div>
            </div>
            {view.color === 'zone' && !programId && <p className="rounded-md bg-muted px-3 py-2 text-sm text-muted-foreground">Pilih program untuk mewarnai zona.</p>}
          </CardHeader>
          <CardContent className="p-0">
            <div className="relative aspect-video min-h-80 w-full bg-muted/30">
              {mapBody}
              <div className="absolute top-3 left-3 flex flex-col gap-2">
                <Button type="button" size="sm" variant="secondary" className="shadow-sm" onClick={() => fit(pointsBounds(visiblePoints) ?? homeTarget())}><Maximize2 aria-hidden="true" />Pas ke semua titik</Button>
                <Button type="button" size="sm" variant="secondary" className="shadow-sm" onClick={() => fit(INDONESIA_BOUNDS)}><Globe2 aria-hidden="true" />Seluruh Indonesia</Button>
              </div>
              <div className="absolute bottom-8 left-3"><MilestoneLegend zones={zoneLegend} /></div>
            </div>
            {pointsQuery.isError && <p role="alert" className="border-t px-4 py-2 text-sm text-destructive">Titik distribusi tidak dapat dimuat. Data akan dicoba lagi otomatis.</p>}
            {assetError && <p className="border-t px-4 py-2 text-xs text-muted-foreground">Batas wilayah tidak dapat dimuat; pencarian kabupaten tidak tersedia.</p>}
          </CardContent>
        </Card>

        <MilestoneFilter counts={counts} selected={view.stage} onChange={(stage) => update({ stage })} />
        {range && <TimelinePlayer start={range.start} days={range.days} day={timelineDay} onChange={setDay} />}
      </div>

      <div className="space-y-5">
        {/* Task 7 replaces this card with the detail panel (desktop) / bottom sheet (phone), driven by `selected` and `select`. */}
        <Card className="gap-0 py-0 shadow-none" aria-label="Detail titik">
          <CardHeader className="border-b py-4"><div className="flex items-center gap-3"><MapPinned className="size-5 text-primary" aria-hidden="true" /><h2 className="text-base leading-snug font-medium">{selected ? 'Detail titik' : 'Pilih titik di peta'}</h2></div></CardHeader>
          <CardContent className="space-y-3 py-5 text-sm">
            {selected
              ? <>
                <p className="font-medium">{selectionLabel(selected, allPoints)}</p>
                <Button type="button" size="sm" variant="outline" onClick={() => select(null)}><X aria-hidden="true" />Tutup pilihan</Button>
              </>
              : <p className="text-muted-foreground">Klik titik atau titik serah untuk melihat milestone, bukti foto, dan lokasinya.</p>}
          </CardContent>
        </Card>
        <MapFilters
          params={params}
          regencies={regencies.data?.data.map((item) => ({ value: item.id, label: `${item.document_code} - ${item.name}` })) ?? []}
          programs={programs.data?.data.map((item) => ({ value: item.id, label: item.name })) ?? []}
          schedules={schedules.data?.data.map((item) => ({ value: item.id, label: item.name })) ?? []}
          zones={zones?.features.map((zone) => ({ value: zone.properties.zone_id, label: zone.properties.is_placeholder ? PLACEHOLDER_ZONE_LABEL : zone.properties.name })) ?? []}
          onChange={setFilter}
          onReset={resetFilters}
          activeCount={activeFilterCount}
        />
      </div>
    </section>

    {aggregate.isError
      ? <DataState kind="error" title="Agregat wilayah tidak dapat dimuat" description="Muat ulang halaman, lalu coba lagi." />
      : <ProgressByZone groups={groups} onSelectRegency={(id) => { const region = regions.find((item) => item.regency_id === id); if (region) focusRegency(region); }} />}
  </div>;
}
