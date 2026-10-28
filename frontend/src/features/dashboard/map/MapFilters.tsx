import { RotateCcw, SlidersHorizontal } from 'lucide-react';
import { Button } from '../../../components/ui/button';
import { Card, CardContent, CardHeader } from '../../../components/ui/card';
import { Label } from '../../../components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../../components/ui/select';
import { allocationStatusLabel } from '../types';

type Option = { value: string; label: string };

const evidenceLabels: Record<string, string> = {
  complete: 'Lengkap', partial: 'Sebagian', empty: 'Belum ada', 'not-configured': 'Belum diatur',
};

type Props = {
  params: URLSearchParams;
  regencies: Option[];
  programs: Option[];
  schedules: Option[];
  /** Zones of the selected program; the zone filter is shown only when there are some. */
  zones: Option[];
  onChange: (key: string, value: string) => void;
  onReset: () => void;
  activeCount: number;
};

function FilterSelect({ id, label, placeholder, value, options, onChange }: { id: string; label: string; placeholder: string; value: string; options: Option[]; onChange: (value: string) => void }) {
  return <div className="grid gap-2">
    <Label id={id}>{label}</Label>
    <Select value={value} onValueChange={(next) => onChange(next ?? '')}>
      <SelectTrigger className="w-full" aria-labelledby={id}><SelectValue placeholder={placeholder} /></SelectTrigger>
      <SelectContent>
        <SelectItem value="">{placeholder}</SelectItem>
        {options.map((option) => <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>)}
      </SelectContent>
    </Select>
  </div>;
}

/** The Data Penerima filters; values live in the URL so the map, table and links agree. */
export function MapFilters({ params, regencies, programs, schedules, zones, onChange, onReset, activeCount }: Props) {
  const field = (key: string) => ({ value: params.get(key) ?? '', onChange: (value: string) => onChange(key, value) });
  return <Card className="gap-0 py-0 shadow-none">
    <CardHeader className="border-b py-4"><div className="flex items-center gap-3"><SlidersHorizontal className="size-5 text-primary" aria-hidden="true" /><h2 className="text-base leading-snug font-medium">Filter peta</h2></div></CardHeader>
    <CardContent className="space-y-4 py-5">
      <FilterSelect id="map-filter-program" label="Program" placeholder="Semua program" options={programs} {...field('program_id')} />
      {zones.length > 0 && <FilterSelect id="map-filter-zone" label="Zona" placeholder="Semua zona" options={zones} {...field('zone_id')} />}
      <FilterSelect id="map-filter-regency" label="Kabupaten" placeholder="Semua kabupaten" options={regencies} {...field('regency_id')} />
      <FilterSelect id="map-filter-schedule" label="Jadwal" placeholder="Semua jadwal" options={schedules} {...field('schedule_id')} />
      <FilterSelect id="map-filter-evidence" label="Kelengkapan bukti" placeholder="Semua kelengkapan" options={Object.entries(evidenceLabels).map(([value, label]) => ({ value, label }))} {...field('evidence_status')} />
      <FilterSelect id="map-filter-allocation" label="Status alokasi" placeholder="Aktif (bukan dibatalkan)" options={Object.entries(allocationStatusLabel).map(([value, label]) => ({ value, label }))} {...field('allocation_status')} />
      <Button type="button" variant="outline" className="w-full" disabled={activeCount === 0} onClick={onReset}><RotateCcw aria-hidden="true" />Reset filter{activeCount > 0 ? ` (${activeCount})` : ''}</Button>
      <p className="text-xs text-muted-foreground">Angka peta memakai filter yang sama dengan Data Penerima, sehingga keduanya selalu sinkron.</p>
    </CardContent>
  </Card>;
}
