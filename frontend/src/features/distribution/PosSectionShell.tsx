import type { ReactNode } from 'react';
import { CheckCircle2 } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';

type PosState = 'locked' | 'active' | 'done';

// Shared shell for the three distribution POS stations (Mesin/Dokumen/Penyerahan) so their
// card, header, and status treatment stay identical. `state` drives the visual emphasis:
// active gets a primary ring, locked is dimmed, done is the resting card.
export function PosSectionShell({
  label,
  badge,
  icon,
  title,
  state,
  status,
  children,
}: {
  label: string;
  badge: string;
  icon: ReactNode;
  title: string;
  state: PosState;
  status?: string;
  children?: ReactNode;
}) {
  return (
    <section
      aria-label={label}
      data-state={state}
      className={cn(
        'space-y-4 rounded-xl border bg-card p-4 ring-1 ring-foreground/5 transition-colors sm:p-5',
        state === 'active' && 'border-primary/40 ring-primary/20',
        state === 'locked' && 'opacity-65',
      )}
    >
      <header className="flex items-start justify-between gap-3">
        <div className="min-w-0 space-y-1.5">
          <Badge variant="outline">{badge}</Badge>
          <h3 className="flex items-center gap-2 text-base font-medium [&>svg]:size-4 [&>svg]:shrink-0 [&>svg]:text-muted-foreground">
            {icon}
            <span className="min-w-0 truncate">{title}</span>
          </h3>
        </div>
        {status && (
          <span className="inline-flex shrink-0 items-center gap-1.5 text-xs font-medium text-primary">
            <CheckCircle2 className="size-4" aria-hidden="true" />
            {status}
          </span>
        )}
      </header>
      {children}
    </section>
  );
}
