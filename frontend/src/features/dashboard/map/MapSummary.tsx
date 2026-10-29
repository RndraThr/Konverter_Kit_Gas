import { MILESTONES, REVIEW_COLOR } from './milestones';
import { needsReview } from './layerData';
import type { PointCollection } from './types';

const numberFormat = new Intl.NumberFormat('id-ID');

/** Header numbers. Milestone KPIs count located points; "Titik terdata" is every slot (meta.total). */
export function KpiStrip({ points }: { points: PointCollection | undefined }) {
  const features = points?.features ?? [];
  const atLeast = (stage: number) => features.filter((point) => point.properties.stage >= stage).length;
  const meta = points?.meta;
  const items = [
    { label: 'Titik terdata', value: meta?.total ?? features.length, hint: meta ? `${numberFormat.format(meta.located)} berlokasi · ${numberFormat.format(meta.unlocated)} belum ada lokasi` : undefined },
    { label: 'Diserahkan', value: atLeast(3) },
    { label: 'Bukti lengkap', value: atLeast(4) },
    { label: 'BA ditandatangani', value: atLeast(5) },
    { label: 'Perlu ditinjau', value: features.filter(needsReview).length, review: true },
  ];
  return <div role="group" aria-label="Ringkasan titik" className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
    {items.map((item) => <div key={item.label} className="rounded-lg border bg-card px-4 py-3">
      <span className="block text-sm text-muted-foreground">{item.label}</span>
      <strong className="text-2xl font-semibold tabular-nums" style={item.review && item.value > 0 ? { color: REVIEW_COLOR } : undefined}>{numberFormat.format(item.value)}</strong>
      {item.hint && <span className="block text-xs text-muted-foreground">{item.hint}</span>}
    </div>)}
  </div>;
}

/** Milestone colours plus the review ring, overlaid on the map. */
export function MilestoneLegend({ zones }: { zones?: Array<{ name: string; color: string }> }) {
  return <div className="space-y-1.5 rounded-md border bg-background/90 p-2 text-xs shadow-sm backdrop-blur" aria-label="Legenda peta" role="group">
    <ul className="space-y-1">
      {MILESTONES.map((m) => <li key={m.stage} className="flex items-center gap-2"><span className="size-3 rounded-full border border-white" style={{ backgroundColor: m.color }} aria-hidden="true" />{`M${m.stage} ${m.label}`}</li>)}
      <li className="flex items-center gap-2"><span className="size-3 rounded-full border-2 bg-white" style={{ borderColor: REVIEW_COLOR }} aria-hidden="true" />Perlu ditinjau</li>
    </ul>
    {zones && zones.length > 0 && <ul className="space-y-1 border-t pt-1.5">
      {zones.map((zone, index) => <li key={`${index}-${zone.name}`} className="flex items-center gap-2"><span className="size-3 rounded-sm" style={{ backgroundColor: zone.color }} aria-hidden="true" />{zone.name}</li>)}
    </ul>}
  </div>;
}
