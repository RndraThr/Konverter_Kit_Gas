import { AuditEntry, metadataRows, presentActor, presentAuditAction, presentResource, readableMetadataValue, stableAuditJSON } from './auditPresentation';
import { Badge } from '../../components/ui/badge';
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '../../components/ui/sheet';

export function AuditDetailSheet({ entry, onClose }: { entry: AuditEntry | null; onClose: () => void }) {
  if (!entry) return null;
  const action = presentAuditAction(entry.action);
  const metadata = metadataRows(entry.metadata);
  return <Sheet open onOpenChange={(open) => { if (!open) onClose(); }}>
    <SheetContent className="w-full max-w-full gap-0 overflow-hidden sm:max-w-xl" aria-label="Detail aktivitas">
      <SheetHeader className="border-b p-5 pr-14">
        <SheetTitle>Detail aktivitas</SheetTitle>
        <SheetDescription>Informasi lengkap event audit yang bersifat read-only.</SheetDescription>
      </SheetHeader>
      <div className="min-h-0 flex-1 space-y-6 overflow-y-auto p-5">
        <section aria-labelledby="audit-detail-summary"><div className="flex items-start justify-between gap-3"><div><h3 id="audit-detail-summary" className="font-semibold">{action.label}</h3><p className="mt-1 break-all text-xs text-muted-foreground">{action.code}</p></div><Badge variant="outline">{presentResource(entry.resource_type)}</Badge></div></section>
        <dl className="grid gap-4 text-sm sm:grid-cols-2">
          <div><dt className="text-xs text-muted-foreground">Waktu lengkap</dt><dd className="mt-1 font-medium tabular-nums">{new Date(entry.created_at).toLocaleString('id-ID')}</dd></div>
          <div><dt className="text-xs text-muted-foreground">Pelaku</dt><dd className="mt-1 font-medium">{presentActor(entry.actor_name)}</dd></div>
          <div><dt className="text-xs text-muted-foreground">Jenis objek</dt><dd className="mt-1 break-words font-medium">{entry.resource_type}</dd></div>
          <div><dt className="text-xs text-muted-foreground">ID objek</dt><dd className="mt-1 break-all font-medium">{entry.resource_id || '—'}</dd></div>
          <div><dt className="text-xs text-muted-foreground">Alamat IP</dt><dd className="mt-1 break-all font-medium">{entry.ip_address || 'Tidak tercatat'}</dd></div>
          <div><dt className="text-xs text-muted-foreground">User-agent</dt><dd className="mt-1 break-all font-medium">{entry.user_agent || 'Tidak tercatat'}</dd></div>
        </dl>
        <section aria-labelledby="audit-metadata-title"><h3 id="audit-metadata-title" className="font-semibold">Metadata</h3>{metadata.length > 0 ? <dl className="mt-3 divide-y rounded-lg border">{metadata.map(([key, value]) => <div className="grid gap-1 px-3 py-2.5 sm:grid-cols-[9rem_minmax(0,1fr)]" key={key}><dt className="break-all text-xs font-medium text-muted-foreground">{key}</dt><dd className="m-0 break-all text-sm">{readableMetadataValue(value)}</dd></div>)}</dl> : <p className="mt-2 text-sm text-muted-foreground">Tidak ada metadata tambahan.</p>}</section>
        <details className="rounded-lg border"><summary className="cursor-pointer px-3 py-3 font-medium focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50">Metadata mentah</summary><pre className="max-w-full overflow-x-auto border-t bg-muted/30 p-3 text-xs whitespace-pre-wrap break-all">{stableAuditJSON(entry.metadata)}</pre></details>
      </div>
    </SheetContent>
  </Sheet>;
}
