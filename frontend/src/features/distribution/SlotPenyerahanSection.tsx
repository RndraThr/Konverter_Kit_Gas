import { useState } from 'react';
import { Lock, PackageCheck } from 'lucide-react';
import { useMutation } from '@tanstack/react-query';
import { apiRequest, ApiError } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { formatDate } from '../programs/types';
import type { DataResponse, DistributionSlot } from './types';
import { DocumentationSlot } from './DocumentationSlot';
import { PosSectionShell } from './PosSectionShell';
import { Button } from '@/components/ui/button';
import { AlertDialog, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog';

export function SlotPenyerahanSection({ slot, onChanged }: { slot: DistributionSlot; onChanged: (slot: DistributionSlot) => void }) {
  const canComplete = useCan('distribution.pos_penyerahan');
  const documentation = slot.documentation.filter((item) => item.stage === 'penyerahan');
  const updateDocumentation = (next: DistributionSlot['documentation'][number]) => onChanged({ ...slot, documentation: slot.documentation.map((item) => item.code === next.code ? next : item) });
  const [confirmOpen, setConfirmOpen] = useState(false);

  const complete = useMutation({
    mutationFn: () => apiRequest<DataResponse<DistributionSlot>>(`/api/v1/distribution/slots/${slot.slot_number}/complete?schedule_id=${encodeURIComponent(slot.schedule_id)}`, { method: 'POST' }),
    onSuccess: ({ data }) => { setConfirmOpen(false); onChanged(data); },
  });

  const docGrid = documentation.length > 0 && <div className="grid gap-4 sm:grid-cols-2">{documentation.map((item) => <DocumentationSlot key={item.code} slot={item} onChanged={updateDocumentation} />)}</div>;

  if (slot.status === 'completed') {
    return <PosSectionShell label="POS Penyerahan" badge="POS Penyerahan" icon={<PackageCheck aria-hidden="true" />} title="Distribusi selesai" state="done" status={slot.distributed_at ? formatDate(slot.distributed_at) : 'Selesai'}>
      {docGrid}
    </PosSectionShell>;
  }

  if (slot.status !== 'linked') {
    return <PosSectionShell label="POS Penyerahan" badge="POS Penyerahan" icon={<Lock aria-hidden="true" />} title="Menunggu dokumen selesai" state="locked" />;
  }

  const required = documentation.filter((item) => item.required);
  const blocked = required.some((item) => item.status !== 'complete');

  return <PosSectionShell label="POS Penyerahan" badge="POS Penyerahan" icon={<PackageCheck aria-hidden="true" />} title="Siap diserahkan" state="active">
    {docGrid}
    {canComplete && <footer className="flex flex-col gap-3 border-t pt-4 sm:flex-row sm:items-center sm:justify-between">
      <div className="space-y-0.5">
        <strong className="block text-sm font-medium">Konfirmasi penyerahan</strong>
        <span className="text-xs text-muted-foreground">{blocked ? 'Lengkapi seluruh bukti wajib sebelum konfirmasi.' : 'Semua bukti wajib telah terpenuhi.'}</span>
      </div>
      <Button className="shrink-0" disabled={blocked} onClick={() => setConfirmOpen(true)}>Selesaikan Distribusi</Button>
    </footer>}
    <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}><AlertDialogContent aria-label="Konfirmasi distribusi">
      <AlertDialogHeader><AlertDialogTitle>Konfirmasi distribusi</AlertDialogTitle><AlertDialogDescription>Pastikan penerima dan bukti penyerahan sudah benar. Aksi ini tidak dapat dibatalkan dari halaman ini.</AlertDialogDescription></AlertDialogHeader>
      {complete.isError && <p className="text-sm text-destructive" role="alert">{complete.error instanceof ApiError ? complete.error.message : 'Distribusi belum dapat diselesaikan.'}</p>}
      <AlertDialogFooter><AlertDialogCancel>Periksa lagi</AlertDialogCancel><Button disabled={complete.isPending} onClick={() => complete.mutate()}>{complete.isPending ? 'Menyelesaikan...' : 'Konfirmasi Penyerahan'}</Button></AlertDialogFooter>
    </AlertDialogContent></AlertDialog>
  </PosSectionShell>;
}
