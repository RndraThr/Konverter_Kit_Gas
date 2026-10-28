import { cn } from '../../../lib/utils';
import { MILESTONES } from './milestones';

const numberFormat = new Intl.NumberFormat('id-ID');

type Props = {
  counts: Record<number, number>;
  /** Selected milestone (1..5); undefined = all points. */
  selected?: number;
  onChange: (stage: number | undefined) => void;
};

const cardClass = 'flex min-h-11 flex-col items-start gap-1 rounded-lg border px-3 py-2 text-left text-sm transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50';

export function MilestoneFilter({ counts, selected, onChange }: Props) {
  const total = MILESTONES.reduce((sum, milestone) => sum + (counts[milestone.stage] ?? 0), 0);
  return <div role="group" aria-label="Filter milestone" className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-6">
    {MILESTONES.map((milestone) => {
      const count = counts[milestone.stage] ?? 0;
      const active = selected === milestone.stage;
      return <button
        key={milestone.stage}
        type="button"
        aria-pressed={active}
        aria-label={`M${milestone.stage} ${milestone.label}: ${numberFormat.format(count)} titik`}
        className={cn(cardClass, active ? 'border-primary bg-primary/5' : 'bg-card')}
        onClick={() => onChange(active ? undefined : milestone.stage)}
      >
        <span className="flex items-center gap-2 text-xs text-muted-foreground">
          <span className="size-2.5 rounded-full border" style={{ backgroundColor: milestone.color }} aria-hidden="true" />
          {`M${milestone.stage}`}
        </span>
        <strong className="text-lg font-semibold tabular-nums">{numberFormat.format(count)}</strong>
        <span className="text-xs leading-tight">{milestone.label}</span>
      </button>;
    })}
    <button
      type="button"
      aria-pressed={selected === undefined}
      aria-label={`Semua titik: ${numberFormat.format(total)} titik`}
      className={cn(cardClass, selected === undefined ? 'border-primary bg-primary/5' : 'bg-card')}
      onClick={() => onChange(undefined)}
    >
      <span className="text-xs text-muted-foreground">Semua</span>
      <strong className="text-lg font-semibold tabular-nums">{numberFormat.format(total)}</strong>
      <span className="text-xs leading-tight">Semua titik</span>
    </button>
  </div>;
}
