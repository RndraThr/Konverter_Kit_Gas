import { useState } from 'react';
import { Check, Palette, PenLine, Trash2, X } from 'lucide-react';
import { useMutation } from '@tanstack/react-query';
import { apiBlobRequest, apiRequest, ApiError } from '../../lib/api';
import type { DataResponse, DistributionSlot } from './types';
import { Button } from '@/components/ui/button';
import { InkPicker, recolorSignature, rememberInk, SignaturePad } from '@/components/SignaturePad';

// Changing the colour of a saved signature: the stored PNG is tinted, so the
// recipient does not sign again (signatures from before the ink picker).
type Recolor = { image: Blob; color: string; png: string };

const padHeight = 220;

// Recipient signature in POS Dokumen: drawn with finger, pen or mouse on a
// transparent canvas, trimmed to the strokes and saved as PNG. Berita Acara
// perorangan and the Training attendance list use it.
export function RecipientSignature({ slot, editable, onChanged }: { slot: DistributionSlot; editable: boolean; onChanged: (slot: DistributionSlot) => void }) {
  const [drawing, setDrawing] = useState(!slot.recipient_signed_at);
  const path = `/api/v1/distribution/slots/${slot.slot_number}/signature?schedule_id=${encodeURIComponent(slot.schedule_id)}`;
  const save = useMutation({
    mutationFn: (imagePNG: string) => apiRequest<DataResponse<DistributionSlot>>(path, { method: 'PUT', body: JSON.stringify({ image_png: imagePNG }) }),
    onSuccess: ({ data }) => { setDrawing(false); setRecolor(null); onChanged(data); },
  });
  const [recolor, setRecolor] = useState<Recolor | null>(null);
  const [recolorError, setRecolorError] = useState('');
  const startRecolor = useMutation({
    mutationFn: () => apiBlobRequest(path),
    onSuccess: (image) => { setRecolorError(''); setRecolor({ image, color: '', png: '' }); },
    onError: () => setRecolorError('Tanda tangan belum dapat dibuka untuk diganti warnanya.'),
  });
  const pickColor = (color: string) => {
    if (!recolor) return;
    rememberInk(color);
    setRecolor({ ...recolor, color });
    recolorSignature(recolor.image, color)
      .then((png) => setRecolor((current) => current && current.color === color ? { ...current, png } : current))
      .catch(() => setRecolorError('Warna belum dapat diterapkan di browser ini.'));
  };
  const remove = useMutation({
    mutationFn: () => apiRequest<DataResponse<DistributionSlot>>(path, { method: 'DELETE' }),
    onSuccess: ({ data }) => { setDrawing(true); onChanged(data); },
  });
  const error = save.error ?? remove.error;
  const imageSrc = recolor?.png ? `data:image/png;base64,${recolor.png}` : `${path}&v=${encodeURIComponent(slot.recipient_signed_at ?? '')}`;

  return <section aria-labelledby={`signature-heading-${slot.id}`} className="space-y-3 rounded-lg border p-4">
    <div className="space-y-1">
      <h4 id={`signature-heading-${slot.id}`} className="font-medium">Tanda tangan penerima</h4>
      <p className="text-sm text-muted-foreground">Opsional. Dipakai di Berita Acara perorangan dan daftar hadir Training; bila kosong, kolom tanda tangan dibiarkan kosong untuk tanda tangan basah.</p>
    </div>
    {slot.recipient_signed_at && !drawing
      ? <div className="space-y-3">
        <img src={imageSrc} alt={`Tanda tangan ${slot.full_name ?? 'penerima'}`} className="block w-full rounded-md border border-dashed bg-white object-contain p-4" style={{ height: padHeight }} />
        <p className="text-xs text-muted-foreground">Ditandatangani {new Date(slot.recipient_signed_at).toLocaleString('id-ID')}</p>
        {editable && recolor && <div className="space-y-3 rounded-md border bg-muted/30 p-3">
          <p className="text-sm">Pilih warna tinta baru. Bentuk tanda tangan tetap sama; penerima tidak perlu tanda tangan lagi.</p>
          <InkPicker value={recolor.color || '#000000'} onChange={pickColor} />
          <div className="flex flex-wrap gap-2">
            <Button type="button" disabled={!recolor.png || save.isPending} onClick={() => save.mutate(recolor.png)}><Check aria-hidden="true" />{save.isPending ? 'Menyimpan...' : 'Simpan warna'}</Button>
            <Button type="button" variant="ghost" disabled={save.isPending} onClick={() => setRecolor(null)}><X aria-hidden="true" />Batal</Button>
          </div>
        </div>}
        {editable && !recolor && <div className="flex flex-wrap gap-2">
          <Button type="button" variant="outline" disabled={startRecolor.isPending} onClick={() => startRecolor.mutate()}><Palette aria-hidden="true" />Ganti warna</Button>
          <Button type="button" variant="outline" onClick={() => setDrawing(true)}><PenLine aria-hidden="true" />Tanda tangan ulang</Button>
          <Button type="button" variant="outline" className="text-destructive" disabled={remove.isPending} onClick={() => remove.mutate()}><Trash2 aria-hidden="true" />Hapus</Button>
        </div>}
      </div>
      : editable
        ? <SignaturePad saving={save.isPending} onCancel={slot.recipient_signed_at ? () => setDrawing(false) : undefined} onSave={(png) => save.mutate(png)} />
        : <p className="text-sm text-muted-foreground">Belum ada tanda tangan.</p>}
    {recolorError && <p role="alert" className="text-sm text-destructive">{recolorError}</p>}
    {error && <p role="alert" className="text-sm text-destructive">{error instanceof ApiError ? error.message : 'Tanda tangan belum dapat disimpan.'}</p>}
  </section>;
}
