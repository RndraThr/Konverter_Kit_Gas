import { useEffect, useState } from 'react';
import { Lock, PackageCheck, Pencil, TriangleAlert } from 'lucide-react';
import { useMutation } from '@tanstack/react-query';
import { apiRequest, ApiError } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { formatDate } from '../programs/types';
import type { DataResponse, DistributionSlot } from './types';
import { DocumentationSlot } from './DocumentationSlot';
import { PosSectionShell } from './PosSectionShell';
import { RevisionDialog } from './RevisionDialog';
import { Button } from '@/components/ui/button';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { AlertDialog, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog';

export function SlotPenyerahanSection({ slot, onChanged }: { slot: DistributionSlot; onChanged: (slot: DistributionSlot) => void }) {
  const canComplete = useCan('distribution.pos_penyerahan');
  const [currentSlot, setCurrentSlot] = useState(slot);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [revisionOpen, setRevisionOpen] = useState(false);
  useEffect(() => setCurrentSlot(slot), [slot]);

  const publish = (next: DistributionSlot) => { setCurrentSlot(next); onChanged(next); };
  const documentation = currentSlot.documentation.filter((item) => item.stage === 'penyerahan');
  const editable = canComplete && currentSlot.status !== 'completed';
  const updateDocumentation = (next: DistributionSlot['documentation'][number]) => publish({ ...currentSlot, documentation: currentSlot.documentation.map((item) => item.code === next.code ? next : item) });
  const required = currentSlot.documentation.filter((item) => item.required);
  const requiredMedia = required.flatMap((item) => (item.files ?? []).filter((file) => file.status === 'accepted'));
  const hasFailedMove = requiredMedia.some((file) => file.storage_state === 'move_failed');
  const hasPendingMove = requiredMedia.some((file) => file.storage_state === 'staging' || file.storage_state === 'moving');
  const missingDocumentation = required.some((item) => item.status !== 'complete');
  const blocked = missingDocumentation || hasPendingMove || hasFailedMove;

  const complete = useMutation({
    mutationFn: () => apiRequest<DataResponse<DistributionSlot>>(`/api/v1/distribution/slots/${currentSlot.slot_number}/complete?schedule_id=${encodeURIComponent(currentSlot.schedule_id)}`, { method: 'POST' }),
    onSuccess: ({ data }) => { setConfirmOpen(false); publish(data); },
  });

  const docGrid = documentation.length > 0 && <div className="grid gap-4 sm:grid-cols-2">{documentation.map((item) => <DocumentationSlot key={item.code} slot={item} canManage={editable} onChanged={updateDocumentation} />)}</div>;

  if (currentSlot.status === 'completed') {
    return <PosSectionShell label="POS Penyerahan" badge="POS Penyerahan" icon={<PackageCheck aria-hidden="true" />} title="Distribusi selesai" state="done" status={currentSlot.distributed_at ? formatDate(currentSlot.distributed_at) : 'Selesai'}>
      {currentSlot.needs_recompletion && <Alert variant="destructive"><TriangleAlert aria-hidden="true" /><AlertDescription>Perlu diselesaikan ulang</AlertDescription></Alert>}
      {canComplete && <div className="flex justify-end"><Button type="button" variant="outline" onClick={() => setRevisionOpen(true)}><Pencil aria-hidden="true" />Buka revisi POS Penyerahan</Button></div>}
      {docGrid}
      <RevisionDialog slot={currentSlot} stage="penyerahan" open={revisionOpen} onOpenChange={setRevisionOpen} onReopened={publish} />
    </PosSectionShell>;
  }

  if (currentSlot.status !== 'linked') {
    return <PosSectionShell label="POS Penyerahan" badge="POS Penyerahan" icon={<Lock aria-hidden="true" />} title="Menunggu dokumen selesai" state="locked" />;
  }

  const readinessCopy = hasFailedMove
    ? 'Pemindahan media gagal. Coba lagi dari POS Dokumen sebelum menyelesaikan distribusi.'
    : hasPendingMove
      ? 'Media masih dipindahkan ke folder final. Tunggu hingga proses selesai.'
      : missingDocumentation
        ? 'Lengkapi seluruh bukti wajib sebelum konfirmasi.'
        : 'Semua bukti wajib telah terpenuhi dan tersimpan di folder final.';

  return <PosSectionShell label="POS Penyerahan" badge="POS Penyerahan" icon={<PackageCheck aria-hidden="true" />} title="Siap diserahkan" state="active">
    {currentSlot.needs_recompletion && <Alert variant="destructive"><TriangleAlert aria-hidden="true" /><AlertDescription>Perlu diselesaikan ulang</AlertDescription></Alert>}
    {docGrid}
    {canComplete && <footer className="flex flex-col gap-3 border-t pt-4 sm:flex-row sm:items-center sm:justify-between">
      <div className="space-y-0.5"><strong className="block text-sm font-medium">Konfirmasi penyerahan</strong><span className="text-xs text-muted-foreground">{readinessCopy}</span></div>
      <Button className="shrink-0" disabled={blocked} onClick={() => setConfirmOpen(true)}>Selesaikan Distribusi</Button>
    </footer>}
    <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}><AlertDialogContent aria-label="Konfirmasi distribusi">
      <AlertDialogHeader><AlertDialogTitle>Konfirmasi distribusi</AlertDialogTitle><AlertDialogDescription>Pastikan penerima dan bukti penyerahan sudah benar. Aksi ini tidak dapat dibatalkan dari halaman ini.</AlertDialogDescription></AlertDialogHeader>
      {complete.isError && <p className="text-sm text-destructive" role="alert">{complete.error instanceof ApiError ? complete.error.message : 'Distribusi belum dapat diselesaikan.'}</p>}
      <AlertDialogFooter><AlertDialogCancel>Periksa lagi</AlertDialogCancel><Button disabled={complete.isPending} onClick={() => complete.mutate()}>{complete.isPending ? 'Menyelesaikan...' : 'Konfirmasi Penyerahan'}</Button></AlertDialogFooter>
    </AlertDialogContent></AlertDialog>
    <RevisionDialog slot={currentSlot} stage="penyerahan" open={revisionOpen} onOpenChange={setRevisionOpen} onReopened={publish} />
  </PosSectionShell>;
}
