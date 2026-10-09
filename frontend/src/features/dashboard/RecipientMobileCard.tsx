import { Ban, ChevronDown, ChevronUp, MapPin, Pencil, RotateCcw, UserRound } from 'lucide-react';
import { useState } from 'react';
import { Badge } from '../../components/ui/badge';
import { Button } from '../../components/ui/button';
import { summarizeEvidence } from './recipientTable';
import { allocationStatusLabel, distributionStatusLabel, type Recipient } from './types';

function evidenceTone(state: ReturnType<typeof summarizeEvidence>['state']) {
  if (state === 'complete') return 'bg-primary';
  if (state === 'partial') return 'bg-amber-500';
  return 'bg-muted-foreground/45';
}

function Detail({ label, value }: { label: string; value?: string }) {
  return <div className="min-w-0"><dt className="text-xs text-muted-foreground">{label}</dt><dd className="mt-0.5 break-words font-medium">{value || '-'}</dd></div>;
}

export function RecipientMobileCard({ recipient, canManage, onEdit, onCancel, onRestore }: {
  recipient: Recipient;
  canManage: boolean;
  onEdit: (recipient: Recipient) => void;
  onCancel: (recipient: Recipient) => void;
  onRestore: (recipient: Recipient) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const evidence = summarizeEvidence(recipient.evidence_slots ?? []);
  const requiredLabel = evidence.state === 'not-configured' ? 'Belum diatur' : `${evidence.requiredComplete}/${evidence.requiredTotal} wajib`;
  const progress = evidence.requiredTotal === 0 ? 0 : Math.round((evidence.requiredComplete / evidence.requiredTotal) * 100);
  const location = [recipient.village, recipient.district].filter(Boolean).join(', ') || 'Wilayah belum dilengkapi';

  return <li aria-label={recipient.full_name} className="overflow-hidden rounded-2xl border bg-card shadow-sm">
    <article>
      <div className="border-b bg-muted/30 p-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <span className="text-xs font-medium text-muted-foreground">No. pembagian</span>
            <div className="mt-0.5 flex items-center gap-2"><strong className="text-xl tabular-nums">{recipient.distribution_number ?? '—'}</strong><span className="h-4 w-px bg-border" aria-hidden="true" /><span className="truncate text-xs text-muted-foreground">{recipient.regency_document_code}</span></div>
          </div>
          <div className="flex max-w-[58%] flex-wrap justify-end gap-1.5">
            <Badge variant={recipient.allocation_status === 'cancelled' ? 'destructive' : recipient.allocation_status === 'distributed' ? 'default' : 'secondary'}>{allocationStatusLabel[recipient.allocation_status] ?? recipient.allocation_status}</Badge>
            {recipient.distribution_status && <Badge variant="outline">{distributionStatusLabel[recipient.distribution_status] ?? recipient.distribution_status}</Badge>}
          </div>
        </div>
      </div>

      <div className="space-y-4 p-4">
        <div className="flex items-start gap-3">
          <span className="flex size-10 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary"><UserRound aria-hidden="true" className="size-5" /></span>
          <div className="min-w-0"><h2 className="truncate text-base font-semibold">Penerima: {recipient.full_name}</h2><p className="mt-0.5 font-mono text-xs text-muted-foreground">{recipient.nik || 'NIK belum tersedia'}</p></div>
        </div>

        <div className="rounded-xl border bg-background/70 p-3">
          <div className="flex items-start gap-2"><MapPin aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-primary" /><div className="min-w-0"><strong className="block text-sm">{recipient.zone_name || 'Zona belum diatur'}</strong><span className="block text-xs text-muted-foreground">{recipient.regency_name} · {location}</span></div></div>
        </div>

        <div>
          <div className="flex items-center justify-between gap-3 text-sm"><span className="font-medium">Dokumentasi</span><strong className="tabular-nums">{requiredLabel}</strong></div>
          <div className="mt-2 h-2 overflow-hidden rounded-full bg-muted" aria-label={`Progres dokumentasi ${progress}%`} role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={progress}><span className={`block h-full rounded-full ${evidenceTone(evidence.state)}`} style={{ width: `${progress}%` }} /></div>
        </div>

        {expanded && <div className="space-y-4 border-t pt-4">
          <dl className="grid grid-cols-2 gap-x-4 gap-y-3 text-sm">
            <Detail label="No. kartu/KUSUKA" value={recipient.sector_identifier} />
            <Detail label="Telepon" value={recipient.phone_number} />
            <Detail label="Program" value={recipient.program_name} />
            <Detail label="Jadwal" value={recipient.schedule_name} />
            <div className="col-span-2"><Detail label="Alamat" value={recipient.address} /></div>
          </dl>
          {(recipient.replaced_by || recipient.replaces) && <div className="rounded-lg bg-amber-50 p-3 text-xs text-amber-900 dark:bg-amber-950/35 dark:text-amber-200">
            {recipient.replaced_by ? `Digantikan oleh ${recipient.replaced_by.full_name}. ${recipient.replaced_by.reason}` : `Menggantikan ${recipient.replaces?.full_name}. ${recipient.replaces?.reason}`}
          </div>}
          {recipient.evidence_slots.length > 0 && <ul className="space-y-2" aria-label={`Rincian dokumentasi ${recipient.full_name}`}>{recipient.evidence_slots.map((slot) => <li key={slot.slot_code} className="flex items-center justify-between gap-3 text-xs"><span>{slot.label}{slot.is_required ? ' · Wajib' : ' · Opsional'}</span><strong className="tabular-nums">{slot.accepted_files}/{slot.min_files}</strong></li>)}</ul>}
        </div>}

        <div className="flex flex-wrap items-center gap-2 border-t pt-3">
          <Button type="button" variant="outline" className="min-h-11 flex-1" aria-expanded={expanded} aria-label={`${expanded ? 'Tutup' : 'Lihat'} detail ${recipient.full_name}`} onClick={() => setExpanded((value) => !value)}>{expanded ? <ChevronUp /> : <ChevronDown />}{expanded ? 'Tutup detail' : 'Lihat detail'}</Button>
          {canManage && <>
            <Button type="button" variant="outline" size="icon" className="size-11" aria-label={`Edit ${recipient.full_name}`} onClick={() => onEdit(recipient)}><Pencil /></Button>
            {recipient.allocation_status === 'cancelled'
              ? <Button type="button" variant="outline" size="icon" className="size-11" aria-label={`Pulihkan ${recipient.full_name}`} onClick={() => onRestore(recipient)}><RotateCcw /></Button>
              : <Button type="button" variant="outline" size="icon" className="size-11 text-destructive" aria-label={`Batalkan ${recipient.full_name}`} onClick={() => onCancel(recipient)}><Ban /></Button>}
          </>}
        </div>
      </div>
    </article>
  </li>;
}
