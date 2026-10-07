import { useMutation } from '@tanstack/react-query';
import { type FormEvent, useEffect, useState } from 'react';
import { Button } from '@/components/ui/button';
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { ApiError, apiRequest } from '../../lib/api';
import type { DataResponse, DistributionSlot, RevisionStage } from './types';

type Props = {
  slot: DistributionSlot;
  stage: RevisionStage;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onReopened: (slot: DistributionSlot) => void;
};

export function RevisionDialog({ slot, stage, open, onOpenChange, onReopened }: Props) {
  const [reason, setReason] = useState('');
  const [error, setError] = useState('');
  useEffect(() => {
    if (open) {
      setReason('');
      setError('');
    }
  }, [open]);

  const reopen = useMutation({
    mutationFn: () => apiRequest<DataResponse<DistributionSlot>>(`/api/v1/distribution/slots/${slot.id}/reopen`, {
      method: 'POST',
      body: JSON.stringify({ stage, reason: reason.trim() }),
    }),
    onSuccess: ({ data }) => {
      onReopened(data);
      onOpenChange(false);
    },
    onError: (cause) => setError(cause instanceof ApiError || cause instanceof Error ? cause.message : 'Slot belum dapat dibuka untuk revisi.'),
  });

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (reason.trim() === '') return;
    setError('');
    reopen.mutate();
  };

  return <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent aria-label="Buka revisi" showCloseButton={false} className="max-w-lg p-0">
      <form onSubmit={submit}>
        <DialogHeader className="border-b p-5">
          <DialogTitle>Buka revisi</DialogTitle>
          <DialogDescription>Slot harus diselesaikan ulang setelah perubahan.</DialogDescription>
        </DialogHeader>
        <div className="grid gap-2 p-5">
          <label className="text-sm font-medium" htmlFor="revision-reason">Alasan revisi</label>
          <textarea id="revision-reason" className="min-h-28 rounded-lg border border-input bg-transparent px-3 py-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50" value={reason} onChange={(event) => setReason(event.target.value)} />
          {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        </div>
        <DialogFooter className="mx-0 mb-0">
          <DialogClose render={<Button type="button" variant="outline" disabled={reopen.isPending} />}>Batal</DialogClose>
          <Button type="submit" disabled={reason.trim() === '' || reopen.isPending}>{reopen.isPending ? 'Membuka...' : 'Buka revisi'}</Button>
        </DialogFooter>
      </form>
    </DialogContent>
  </Dialog>;
}
