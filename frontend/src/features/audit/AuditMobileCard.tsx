import { CalendarClock, UserRound } from 'lucide-react';
import { Badge } from '../../components/ui/badge';
import { Button } from '../../components/ui/button';
import { AuditEntry, presentActor, presentAuditAction, presentResource } from './auditPresentation';

export function AuditMobileCard({ entry, onOpen }: { entry: AuditEntry; onOpen: () => void }) {
  const action = presentAuditAction(entry.action);
  return <article className="min-w-0 rounded-lg border bg-card p-4">
    <div className="flex min-w-0 items-start justify-between gap-3">
      <div className="min-w-0"><h3 className="font-semibold leading-5">{action.label}</h3><p className="mt-1 break-all text-xs text-muted-foreground">{action.code}</p></div>
      <Badge variant="outline" className="max-w-[45%] shrink-0 truncate">{presentResource(entry.resource_type)}</Badge>
    </div>
    <dl className="mt-4 grid gap-2 text-sm">
      <div className="flex min-w-0 items-center gap-2"><UserRound className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" /><dt className="sr-only">Pelaku</dt><dd className="truncate">{presentActor(entry.actor_name)}</dd></div>
      <div className="flex min-w-0 items-center gap-2"><CalendarClock className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" /><dt className="sr-only">Waktu</dt><dd className="tabular-nums">{new Date(entry.created_at).toLocaleString('id-ID')}</dd></div>
    </dl>
    {entry.resource_id ? <p className="mt-3 break-all rounded-md bg-muted/40 px-3 py-2 text-xs text-muted-foreground">ID: {entry.resource_id}</p> : null}
    <Button className="mt-4 w-full" type="button" variant="outline" onClick={onOpen}>Lihat detail</Button>
  </article>;
}
