import { TriangleAlert } from 'lucide-react';
import { REVIEW_COLOR, MILESTONES } from './milestones';
import { needsReview } from './layerData';
import type { PointFeature } from './types';

/** Recipients at one handover site. This list is also the non-map path to the same data. */
export function SiteList({ members, onPick }: { members: PointFeature[]; onPick: (slotId: string) => void }) {
  const sorted = [...members].sort((a, b) => a.properties.stage - b.properties.stage || a.properties.code.localeCompare(b.properties.code, 'id', { numeric: true }));
  return <ul aria-label="Daftar nomor bagi" className="divide-y rounded-md border">
    {sorted.map((member) => {
      const { slot_id, code, stage } = member.properties;
      const milestone = MILESTONES[Math.min(5, Math.max(1, stage)) - 1];
      return <li key={slot_id}>
        <button type="button" onClick={() => onPick(slot_id)} className="flex min-h-11 w-full items-center gap-2 px-3 py-2 text-left text-sm hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none">
          <span aria-hidden="true" className="size-3 shrink-0 rounded-full border" style={{ backgroundColor: milestone.color }} />
          <span className="font-medium">Nomor bagi {code}</span>
          <span className="ml-auto text-xs text-muted-foreground">{milestone.label}</span>
          {needsReview(member) && <span className="inline-flex items-center gap-1 text-xs font-medium" style={{ color: REVIEW_COLOR }}><TriangleAlert className="size-3.5" aria-hidden="true" />Perlu ditinjau</span>}
        </button>
      </li>;
    })}
  </ul>;
}
