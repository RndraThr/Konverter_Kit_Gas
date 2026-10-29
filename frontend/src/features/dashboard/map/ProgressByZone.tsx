import { MILESTONES } from './milestones';
import type { ProgressGroup, StageCounts } from './progress';

const numberFormat = new Intl.NumberFormat('id-ID');

function StackedBar({ counts, total }: { counts: StageCounts; total: number }) {
  const summary = MILESTONES.map((m, i) => `M${m.stage} ${numberFormat.format(counts[i])}`).join(', ');
  return <div className="flex h-3 w-full overflow-hidden rounded-full bg-muted" role="img" aria-label={summary}>
    {total > 0 && MILESTONES.map((m, i) => counts[i] > 0 && <span key={m.stage} style={{ width: `${(counts[i] / total) * 100}%`, backgroundColor: m.color }} />)}
  </div>;
}

type Props = {
  groups: ProgressGroup[];
  onSelectRegency: (regencyId: string) => void;
};

/** Non-map path to the same numbers: milestone mix per regency, grouped per zone with subtotals. */
export function ProgressByZone({ groups, onSelectRegency }: Props) {
  return <section aria-label="Progres milestone per kabupaten" className="space-y-3">
    <h2 className="text-base font-medium">Progres milestone per kabupaten</h2>
    {groups.length === 0 && <p className="rounded-lg border px-3 py-6 text-center text-sm text-muted-foreground">Belum ada wilayah dengan data pada filter ini.</p>}
    {groups.map((group) => <div key={group.key || group.name} role="group" aria-label={group.name} className="overflow-hidden rounded-lg border">
      <div className="flex flex-wrap items-center justify-between gap-2 bg-muted/50 px-3 py-2">
        <h3 className="text-sm font-semibold">{group.name}</h3>
        <span className="text-xs text-muted-foreground tabular-nums">{`Subtotal: ${numberFormat.format(group.total)} titik`}</span>
      </div>
      <ul>
        {group.rows.map((row) => <li key={row.regency_id} className="border-t">
          <button
            type="button"
            className="grid min-h-11 w-full grid-cols-[minmax(0,12rem)_minmax(0,1fr)_auto] items-center gap-3 px-3 py-2 text-left text-sm hover:bg-muted/40 focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
            aria-label={`Tampilkan ${row.name} di peta`}
            onClick={() => onSelectRegency(row.regency_id)}
          >
            <span className="truncate font-medium">{row.name}</span>
            <StackedBar counts={row.counts} total={row.total} />
            <span className="text-xs text-muted-foreground tabular-nums">{`${numberFormat.format(row.total)} titik`}</span>
          </button>
        </li>)}
      </ul>
    </div>)}
  </section>;
}
