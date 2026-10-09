import { useEffect, useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useSearchParams } from 'react-router-dom';
import { Layers3, MapPinned, RotateCcw, SlidersHorizontal } from 'lucide-react';
import { apiRequest } from '../../lib/api';
import { PageHeader } from '../../components/PageHeader';
import { DataState } from '../../components/DataState';
import { Badge } from '../../components/ui/badge';
import { Button } from '../../components/ui/button';
import { Card, CardContent, CardHeader } from '../../components/ui/card';
import { Label } from '../../components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../components/ui/select';
import { DistributionMap } from './DistributionMap';
import { loadRegencyAsset } from './mapAsset';
import { buildRegionIndex, colorForValue, mapMetrics, metricByKey, metricMax, progressDenominator, type MetricKey, type RegencyAsset } from './mapRegions';
import { allocationStatusLabel, type MapData, type MapRegion } from './types';

type ScheduleOption = { id: string; name: string; regency?: { name: string }; program?: { program_type: string } };
type RegencyOption = { id: string; name: string; document_code: string };
type ProgramOption = { id: string; name: string; program_type: string };

const numberFormat = new Intl.NumberFormat('id-ID');
const emptyRegions: MapRegion[] = [];
const filterKeys = ['regency_id', 'program_id', 'schedule_id', 'district', 'evidence_status', 'allocation_status'] as const;

const evidenceLabels: Record<string, string> = {
  complete: 'Lengkap', partial: 'Sebagian', empty: 'Belum ada', 'not-configured': 'Belum diatur',
};

export function DistributionMapPage() {
  const [params, setParams] = useSearchParams();
  const [metricKey, setMetricKey] = useState<MetricKey>('recipients');
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const [asset, setAsset] = useState<RegencyAsset | null>(null);
  const [assetError, setAssetError] = useState(false);
  const [assetLoadAttempt, setAssetLoadAttempt] = useState(0);

  // The boundary asset is ~440 kB, so it is a separate chunk fetched only when this page is opened.
  // Until it arrives the aggregate request and the ranking table already work.
  useEffect(() => {
    let cancelled = false;
    setAssetError(false);
    void loadRegencyAsset().then((loaded) => {
      if (cancelled) return;
      setAsset(loaded);
    }).catch(() => {
      if (!cancelled) setAssetError(true);
    });
    return () => { cancelled = true; };
  }, [assetLoadAttempt]);

  const query = params.toString();
  const map = useQuery({
    queryKey: ['recipients', 'map', query],
    queryFn: () => apiRequest<{ data: MapData }>(`/api/v1/recipients/map?${query}`),
  });
  const regencies = useQuery({ queryKey: ['program-setup', 'regencies'], queryFn: () => apiRequest<{ data: RegencyOption[] }>('/api/v1/program-setup/regencies') });
  const programs = useQuery({ queryKey: ['program-setup', 'programs'], queryFn: () => apiRequest<{ data: ProgramOption[] }>('/api/v1/program-setup/programs') });
  const schedules = useQuery({ queryKey: ['program-setup', 'schedules'], queryFn: () => apiRequest<{ data: ScheduleOption[] }>('/api/v1/program-setup/schedules') });

  const data = map.data?.data;
  const regions = data?.regions ?? emptyRegions;
  const totals = data?.totals;
  const metric = metricByKey(metricKey);
  const max = useMemo(() => metricMax(metric, regions), [metric, regions]);
  const regionIndex = useMemo(() => buildRegionIndex(regions), [regions]);
  // Reverse lookup so each ranking row does not scan the index.
  const keyByRegencyID = useMemo(() => new Map([...regionIndex.entries()].map(([key, region]) => [region.regency_id, key])), [regionIndex]);
  const selectedRegion = selectedKey ? regionIndex.get(selectedKey) : undefined;

  const ranked = useMemo(
    () => [...regions].filter((region) => region.recipients > 0 || region.slot_quota > 0).sort((a, b) => metric.value(b) - metric.value(a) || a.regency_name.localeCompare(b.regency_name)),
    [metric, regions],
  );
  const ranking = useMemo(() => {
    const top = ranked.slice(0, 12);
    if (selectedRegion && !top.some((region) => region.regency_id === selectedRegion.regency_id)) return [selectedRegion, ...top.slice(0, 11)];
    return top;
  }, [ranked, selectedRegion]);

  const setFilter = (key: string, value: string) => {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value); else next.delete(key);
    setParams(next);
    setSelectedKey(null);
  };
  const activeFilterCount = filterKeys.filter((key) => Boolean(params.get(key))).length;
  const resetFilters = () => { setParams(new URLSearchParams()); setSelectedKey(null); };

  const summary = (region: MapRegion) => [
    { label: 'Penerima di wilayah ini', value: numberFormat.format(region.recipients) },
    { label: 'Nomor bagi selesai', value: `${region.slots_completed}/${progressDenominator(region)}` },
    { label: 'Dokumentasi lengkap', value: numberFormat.format(region.evidence_complete) },
    { label: 'Perlu ditinjau', value: numberFormat.format(region.needs_review) },
  ];

  return <div className="space-y-6">
    <PageHeader title="Map Distribusi" description="Pemetaan sebaran penerima, progres penyaluran, dan kelengkapan bukti per kabupaten/kota." />

    <section aria-label="Ruang kerja Map Distribusi" className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_320px]">
      <Card className="gap-0 overflow-hidden py-0 shadow-none">
        <CardHeader className="flex-row flex-wrap items-center justify-between gap-3 border-b py-4">
          <div className="flex items-center gap-3"><MapPinned className="size-5 text-primary" aria-hidden="true" /><h2 className="text-base leading-snug font-medium">Area peta</h2></div>
          <div className="flex flex-wrap items-center gap-2" role="group" aria-label="Pilih metrik peta">
            {mapMetrics.map((item) => <Button
              key={item.key}
              type="button"
              size="sm"
              variant={item.key === metricKey ? 'default' : 'outline'}
              aria-pressed={item.key === metricKey}
              onClick={() => setMetricKey(item.key)}
            >{item.label}</Button>)}
          </div>
        </CardHeader>
        <CardContent className="p-0">
          {map.isPending && <div className="p-6"><DataState kind="loading" title="Menghitung agregat wilayah" description="Menyiapkan angka peta dari data penerima." /></div>}
          {map.isError && <div className="p-6"><DataState kind="error" title="Agregat peta tidak dapat dimuat" description="Muat ulang halaman, lalu coba lagi." /></div>}
          {data && (asset
            ? <DistributionMap asset={asset} regions={regions} metric={metric} selectedKey={selectedKey} onSelect={setSelectedKey} />
            : assetError
              ? <div className="p-6"><DataState kind="error" title="Batas wilayah tidak dapat dimuat" description="Periksa koneksi, lalu coba memuat aset peta kembali." action={{ label: 'Coba lagi', onClick: () => setAssetLoadAttempt((current) => current + 1) }} /></div>
              : <div className="p-6"><DataState kind="loading" title="Memuat batas wilayah" description="Menyiapkan geometri kabupaten/kota." /></div>)}
        </CardContent>
      </Card>

      <div className="space-y-5">
        <Card className="gap-0 py-0 shadow-none">
          <CardHeader className="border-b py-4"><div className="flex items-center gap-3"><SlidersHorizontal className="size-5 text-primary" aria-hidden="true" /><h2 className="text-base leading-snug font-medium">Kontrol peta</h2></div></CardHeader>
          <CardContent className="space-y-4 py-5">
            <div className="grid gap-2"><Label id="map-filter-regency">Kabupaten</Label><Select value={params.get('regency_id') ?? ''} onValueChange={(value) => setFilter('regency_id', value ?? '')}><SelectTrigger className="w-full" aria-labelledby="map-filter-regency"><SelectValue placeholder="Semua kabupaten" /></SelectTrigger><SelectContent><SelectItem value="">Semua kabupaten</SelectItem>{regencies.data?.data.map((item) => <SelectItem key={item.id} value={item.id}>{item.document_code} - {item.name}</SelectItem>)}</SelectContent></Select></div>
            <div className="grid gap-2"><Label id="map-filter-program">Program</Label><Select value={params.get('program_id') ?? ''} onValueChange={(value) => setFilter('program_id', value ?? '')}><SelectTrigger className="w-full" aria-labelledby="map-filter-program"><SelectValue placeholder="Semua program" /></SelectTrigger><SelectContent><SelectItem value="">Semua program</SelectItem>{programs.data?.data.map((item) => <SelectItem key={item.id} value={item.id}>{item.name}</SelectItem>)}</SelectContent></Select></div>
            <div className="grid gap-2"><Label id="map-filter-schedule">Jadwal</Label><Select value={params.get('schedule_id') ?? ''} onValueChange={(value) => setFilter('schedule_id', value ?? '')}><SelectTrigger className="w-full" aria-labelledby="map-filter-schedule"><SelectValue placeholder="Semua jadwal" /></SelectTrigger><SelectContent><SelectItem value="">Semua jadwal</SelectItem>{schedules.data?.data.map((item) => <SelectItem key={item.id} value={item.id}>{item.name}</SelectItem>)}</SelectContent></Select></div>
            <div className="grid gap-2"><Label id="map-filter-evidence">Kelengkapan bukti</Label><Select value={params.get('evidence_status') ?? ''} onValueChange={(value) => setFilter('evidence_status', value ?? '')}><SelectTrigger className="w-full" aria-labelledby="map-filter-evidence"><SelectValue placeholder="Semua kelengkapan" /></SelectTrigger><SelectContent><SelectItem value="">Semua kelengkapan</SelectItem>{Object.entries(evidenceLabels).map(([value, label]) => <SelectItem key={value} value={value}>{label}</SelectItem>)}</SelectContent></Select></div>
            <div className="grid gap-2"><Label id="map-filter-allocation">Status alokasi</Label><Select value={params.get('allocation_status') ?? ''} onValueChange={(value) => setFilter('allocation_status', value ?? '')}><SelectTrigger className="w-full" aria-labelledby="map-filter-allocation"><SelectValue placeholder="Aktif (bukan dibatalkan)" /></SelectTrigger><SelectContent><SelectItem value="">Aktif (bukan dibatalkan)</SelectItem>{Object.entries(allocationStatusLabel).map(([value, label]) => <SelectItem key={value} value={value}>{label}</SelectItem>)}</SelectContent></Select></div>
            <Button type="button" variant="outline" className="w-full" disabled={activeFilterCount === 0} onClick={resetFilters}><RotateCcw aria-hidden="true" />Reset filter{activeFilterCount > 0 ? ` (${activeFilterCount})` : ''}</Button>
          </CardContent>
        </Card>

        <Card className="gap-0 py-0 shadow-none">
          <CardHeader className="border-b py-4"><div className="flex items-center gap-3"><Layers3 className="size-5 text-primary" aria-hidden="true" /><h2 className="text-base leading-snug font-medium">Legenda</h2></div></CardHeader>
          <CardContent className="space-y-3 py-5">
            <p className="text-sm text-muted-foreground">{metric.description}</p>
            <div className="flex items-center gap-1" aria-hidden="true">
              {[0, 0.25, 0.5, 0.75, 1].map((ratio) => <span key={ratio} className="h-3 flex-1 rounded-sm border" style={{ backgroundColor: colorForValue(ratio, 1, true) }} />)}
            </div>
            <div className="flex justify-between text-xs text-muted-foreground"><span>Rendah</span><span>{metric.ratio ? '100%' : numberFormat.format(Math.round(max))}</span></div>
            <p className="flex items-center gap-2 text-xs text-muted-foreground"><span className="size-3 rounded-sm border" style={{ backgroundColor: colorForValue(0, 1, false) }} />Belum ada data pada filter ini</p>
            {totals && <dl className="space-y-1 border-t pt-3 text-sm">
              <div className="flex justify-between"><dt className="text-muted-foreground">Penerima aktif</dt><dd className="font-medium tabular-nums">{numberFormat.format(totals.recipients)}</dd></div>
              <div className="flex justify-between"><dt className="text-muted-foreground">Nomor selesai</dt><dd className="font-medium tabular-nums">{`${numberFormat.format(totals.slots_completed)} / ${numberFormat.format(progressDenominator(totals))}`}</dd></div>
              <div className="flex justify-between"><dt className="text-muted-foreground">Bukti lengkap</dt><dd className="font-medium tabular-nums">{numberFormat.format(totals.evidence_complete)}</dd></div>
              <div className="flex justify-between"><dt className="text-muted-foreground">Kabupaten berdata</dt><dd className="font-medium tabular-nums">{numberFormat.format(ranked.length)}</dd></div>
            </dl>}
            <p className="border-t pt-3 text-xs text-muted-foreground">Angka peta memakai filter yang sama dengan Data Penerima, sehingga keduanya selalu sinkron.</p>
          </CardContent>
        </Card>
      </div>
    </section>

    <section aria-label="Peringkat wilayah" className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-base font-medium">Peringkat wilayah — {metric.label}</h2>
        {selectedRegion && <Badge variant="secondary">{`Terpilih: ${selectedRegion.regency_name}`}</Badge>}
      </div>
      {selectedRegion && <div role="group" aria-label="Rincian wilayah terpilih" className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {summary(selectedRegion).map((item) => <Card key={item.label} className="py-4 shadow-none"><CardContent><strong className="text-xl font-semibold tabular-nums">{item.value}</strong><span className="block text-sm text-muted-foreground">{item.label}</span></CardContent></Card>)}
      </div>}
      <div className="overflow-x-auto rounded-lg border">
        <table className="w-full text-sm">
          <thead className="bg-muted/50 text-left">
            <tr>
              <th scope="col" className="px-3 py-2 font-medium">Kabupaten/Kota</th>
              <th scope="col" className="px-3 py-2 font-medium">Provinsi</th>
              <th scope="col" className="px-3 py-2 text-right font-medium">{metric.label}</th>
              <th scope="col" className="px-3 py-2 text-right font-medium">Penerima</th>
              <th scope="col" className="px-3 py-2 text-right font-medium">Nomor selesai</th>
              <th scope="col" className="px-3 py-2 text-right font-medium">Bukti lengkap</th>
            </tr>
          </thead>
          <tbody>
            {ranking.length === 0 && <tr><td colSpan={6} className="px-3 py-6 text-center text-muted-foreground">Belum ada wilayah dengan data pada filter ini.</td></tr>}
            {ranking.map((region) => {
              const key = keyByRegencyID.get(region.regency_id) ?? null;
              return <tr key={region.regency_id} className={`border-t hover:bg-muted/40 ${key && key === selectedKey ? 'bg-muted/60' : ''}`}>
                <td className="p-1"><button type="button" aria-label={`Pilih ${region.regency_name}`} aria-pressed={key === selectedKey} className="flex min-h-11 w-full items-center rounded-md px-2 text-left hover:bg-muted focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50" onClick={() => setSelectedKey(key)}><span className="mr-2 inline-block size-2.5 shrink-0 rounded-sm border" style={{ backgroundColor: colorForValue(metric.value(region), max, metric.hasData(region)) }} /><strong>{region.regency_name}</strong><span className="ml-1 text-muted-foreground">{region.document_code}</span></button></td>
                <td className="px-3 py-2 text-muted-foreground">{region.province_name}</td>
                <td className="px-3 py-2 text-right tabular-nums">{metric.format(region)}</td>
                <td className="px-3 py-2 text-right tabular-nums">{numberFormat.format(region.recipients)}</td>
                <td className="px-3 py-2 text-right tabular-nums">{`${region.slots_completed}/${progressDenominator(region)}`}</td>
                <td className="px-3 py-2 text-right tabular-nums">{numberFormat.format(region.evidence_complete)}</td>
              </tr>;
            })}
          </tbody>
        </table>
      </div>
    </section>
  </div>;
}
