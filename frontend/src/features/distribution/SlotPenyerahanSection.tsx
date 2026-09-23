import { useState } from 'react';
import { CheckCircle2, Lock, PackageCheck } from 'lucide-react';
import { useMutation } from '@tanstack/react-query';
import { apiRequest, ApiError } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { formatDate } from '../programs/types';
import type { DataResponse, DistributionSlot } from './types';
import { DocumentationSlot } from './DocumentationSlot';
import styles from './Distribution.module.css';
import { Badge } from '@/components/ui/badge';
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

  if (slot.status === 'completed') {
    return <section className={styles.slotSection} data-state="done" aria-label="POS Penyerahan">
      <header className={styles.slotSectionHeader}>
        <div><Badge variant="outline">POS Penyerahan</Badge><h3><PackageCheck aria-hidden="true" />Distribusi selesai</h3></div>
        <span className={styles.slotSectionStatus}><CheckCircle2 aria-hidden="true" />{slot.distributed_at ? formatDate(slot.distributed_at) : 'Selesai'}</span>
      </header>
      <div className={styles.sectionDocumentation}>{documentation.map((item) => <DocumentationSlot key={item.code} slot={item} onChanged={updateDocumentation} />)}</div>
    </section>;
  }

  if (slot.status !== 'linked') {
    return <section className={styles.slotSection} data-state="locked" aria-label="POS Penyerahan"><header className={styles.slotSectionHeader}><div><Badge variant="outline">POS Penyerahan</Badge><h3><Lock aria-hidden="true" />Menunggu dokumen selesai</h3></div></header></section>;
  }

  const required = documentation.filter((item) => item.required);
  const blocked = required.some((item) => item.status !== 'complete');

  return <section className={styles.slotSection} data-state="active" aria-label="POS Penyerahan">
    <header className={styles.slotSectionHeader}><div><Badge variant="outline">POS Penyerahan</Badge><h3><PackageCheck aria-hidden="true" />Siap diserahkan</h3></div></header>
    <div className={styles.sectionDocumentation}>{documentation.map((item) => <DocumentationSlot key={item.code} slot={item} onChanged={updateDocumentation} />)}</div>
    {canComplete && <footer className={styles.completion}>
      <div><strong>Konfirmasi penyerahan</strong><span>{blocked ? 'Lengkapi seluruh bukti wajib sebelum konfirmasi.' : 'Semua bukti wajib telah terpenuhi.'}</span></div>
      <Button disabled={blocked} onClick={() => setConfirmOpen(true)}>Selesaikan Distribusi</Button>
    </footer>}
    <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}><AlertDialogContent aria-label="Konfirmasi distribusi">
      <AlertDialogHeader><AlertDialogTitle>Konfirmasi distribusi</AlertDialogTitle><AlertDialogDescription>Pastikan penerima dan bukti penyerahan sudah benar. Aksi ini tidak dapat dibatalkan dari halaman ini.</AlertDialogDescription></AlertDialogHeader>
      {complete.isError && <p role="alert">{complete.error instanceof ApiError ? complete.error.message : 'Distribusi belum dapat diselesaikan.'}</p>}
      <AlertDialogFooter><AlertDialogCancel>Periksa lagi</AlertDialogCancel><Button disabled={complete.isPending} onClick={() => complete.mutate()}>{complete.isPending ? 'Menyelesaikan...' : 'Konfirmasi Penyerahan'}</Button></AlertDialogFooter>
    </AlertDialogContent></AlertDialog>
  </section>;
}
