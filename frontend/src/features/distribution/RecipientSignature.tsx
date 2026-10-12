import { useState } from 'react';
import { PenLine, Trash2 } from 'lucide-react';
import { useMutation } from '@tanstack/react-query';
import { apiRequest, ApiError } from '../../lib/api';
import type { DataResponse, DistributionSlot } from './types';
import { Button } from '@/components/ui/button';
import { SignaturePad } from '@/components/SignaturePad';

const padHeight = 220;

// Recipient signature in POS Dokumen: drawn with finger, pen or mouse on a
// transparent canvas, trimmed to the strokes and saved as PNG. Berita Acara
// perorangan and the Training attendance list use it.
export function RecipientSignature({ slot, editable, onChanged }: { slot: DistributionSlot; editable: boolean; onChanged: (slot: DistributionSlot) => void }) {
  const [drawing, setDrawing] = useState(!slot.recipient_signed_at);
  const path = `/api/v1/distribution/slots/${slot.slot_number}/signature?schedule_id=${encodeURIComponent(slot.schedule_id)}`;
  const save = useMutation({
    mutationFn: (imagePNG: string) => apiRequest<DataResponse<DistributionSlot>>(path, { method: 'PUT', body: JSON.stringify({ image_png: imagePNG }) }),
    onSuccess: ({ data }) => { setDrawing(false); onChanged(data); },
  });
  const remove = useMutation({
    mutationFn: () => apiRequest<DataResponse<DistributionSlot>>(path, { method: 'DELETE' }),
    onSuccess: ({ data }) => { setDrawing(true); onChanged(data); },
  });
  const error = save.error ?? remove.error;

  return <section aria-labelledby={`signature-heading-${slot.id}`} className="space-y-3 rounded-lg border p-4">
    <div className="space-y-1">
      <h4 id={`signature-heading-${slot.id}`} className="font-medium">Tanda tangan penerima</h4>
      <p className="text-sm text-muted-foreground">Dipakai di Berita Acara perorangan dan daftar hadir Training. Wajib sebelum distribusi diselesaikan.</p>
    </div>
    {slot.recipient_signed_at && !drawing
      ? <div className="space-y-3">
        <img src={`${path}&v=${encodeURIComponent(slot.recipient_signed_at)}`} alt={`Tanda tangan ${slot.full_name ?? 'penerima'}`} className="block w-full rounded-md border border-dashed bg-white object-contain p-4" style={{ height: padHeight }} />
        <p className="text-xs text-muted-foreground">Ditandatangani {new Date(slot.recipient_signed_at).toLocaleString('id-ID')}</p>
        {editable && <div className="flex flex-wrap gap-2">
          <Button type="button" variant="outline" onClick={() => setDrawing(true)}><PenLine aria-hidden="true" />Tanda tangan ulang</Button>
          <Button type="button" variant="outline" className="text-destructive" disabled={remove.isPending} onClick={() => remove.mutate()}><Trash2 aria-hidden="true" />Hapus</Button>
        </div>}
      </div>
      : editable
        ? <SignaturePad saving={save.isPending} onCancel={slot.recipient_signed_at ? () => setDrawing(false) : undefined} onSave={(png) => save.mutate(png)} />
        : <p className="text-sm text-muted-foreground">Belum ada tanda tangan.</p>}
    {error && <p role="alert" className="text-sm text-destructive">{error instanceof ApiError ? error.message : 'Tanda tangan belum dapat disimpan.'}</p>}
  </section>;
}
