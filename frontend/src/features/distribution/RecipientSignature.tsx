import { type PointerEvent, useEffect, useRef, useState } from 'react';
import { Eraser, PenLine, Trash2 } from 'lucide-react';
import { useMutation } from '@tanstack/react-query';
import { apiRequest, ApiError } from '../../lib/api';
import type { DataResponse, DistributionSlot } from './types';
import { Button } from '@/components/ui/button';

const padHeight = 220;
const trimPadding = 12;

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
        <img src={`${path}&v=${encodeURIComponent(slot.recipient_signed_at)}`} alt={`Tanda tangan ${slot.full_name ?? 'penerima'}`} className="h-28 max-w-full rounded-md border bg-white object-contain p-2" />
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

type Bounds = { minX: number; minY: number; maxX: number; maxY: number };

function SignaturePad({ saving, onSave, onCancel }: { saving: boolean; onSave: (png: string) => void; onCancel?: () => void }) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const last = useRef<{ x: number; y: number } | null>(null);
  const bounds = useRef<Bounds | null>(null);
  const [empty, setEmpty] = useState(true);

  // Backing store at device resolution so strokes stay sharp.
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ratio = window.devicePixelRatio || 1;
    canvas.width = Math.round(canvas.clientWidth * ratio);
    canvas.height = Math.round(padHeight * ratio);
    const context = canvas.getContext('2d');
    if (!context) return;
    context.scale(ratio, ratio);
    context.lineCap = 'round';
    context.lineJoin = 'round';
    context.lineWidth = 2.6;
    context.strokeStyle = '#1e2d6e';
  }, []);

  const point = (event: PointerEvent<HTMLCanvasElement>) => {
    const rect = event.currentTarget.getBoundingClientRect();
    return { x: event.clientX - rect.left, y: event.clientY - rect.top };
  };
  const extend = (x: number, y: number) => {
    const b = bounds.current;
    bounds.current = b ? { minX: Math.min(b.minX, x), minY: Math.min(b.minY, y), maxX: Math.max(b.maxX, x), maxY: Math.max(b.maxY, y) } : { minX: x, minY: y, maxX: x, maxY: y };
  };
  const start = (event: PointerEvent<HTMLCanvasElement>) => {
    event.currentTarget.setPointerCapture(event.pointerId);
    const p = point(event);
    last.current = p;
    extend(p.x, p.y);
    const context = canvasRef.current?.getContext('2d');
    context?.beginPath();
    context?.arc(p.x, p.y, 1.3, 0, Math.PI * 2);
    context?.fill();
    setEmpty(false);
  };
  const move = (event: PointerEvent<HTMLCanvasElement>) => {
    if (!last.current) return;
    const p = point(event);
    const context = canvasRef.current?.getContext('2d');
    if (!context) return;
    context.beginPath();
    context.moveTo(last.current.x, last.current.y);
    context.lineTo(p.x, p.y);
    context.stroke();
    last.current = p;
    extend(p.x, p.y);
  };
  const end = () => { last.current = null; };

  const clear = () => {
    const canvas = canvasRef.current;
    canvas?.getContext('2d')?.clearRect(0, 0, canvas.width, canvas.height);
    bounds.current = null;
    setEmpty(true);
  };

  // Crop to the strokes (plus a margin) so the signature fills the Berita Acara box.
  const save = () => {
    const canvas = canvasRef.current;
    const b = bounds.current;
    if (!canvas || !b) return;
    const ratio = canvas.width / canvas.clientWidth;
    const x = Math.max(0, (b.minX - trimPadding) * ratio);
    const y = Math.max(0, (b.minY - trimPadding) * ratio);
    const width = Math.max(60, Math.min(canvas.width - x, (b.maxX - b.minX + trimPadding * 2) * ratio));
    const height = Math.max(60, Math.min(canvas.height - y, (b.maxY - b.minY + trimPadding * 2) * ratio));
    const output = document.createElement('canvas');
    output.width = Math.round(width);
    output.height = Math.round(height);
    output.getContext('2d')?.drawImage(canvas, x, y, width, height, 0, 0, output.width, output.height);
    onSave(output.toDataURL('image/png').split(',')[1] ?? '');
  };

  return <div className="space-y-2">
    <canvas
      ref={canvasRef}
      aria-label="Area tanda tangan penerima"
      className="block w-full touch-none rounded-md border border-dashed bg-white"
      style={{ height: padHeight }}
      onPointerDown={start}
      onPointerMove={move}
      onPointerUp={end}
      onPointerCancel={end}
      onPointerLeave={end}
    />
    <p className="text-xs text-muted-foreground">Minta penerima menandatangani di kotak putih.</p>
    <div className="flex flex-wrap gap-2">
      <Button type="button" disabled={empty || saving} onClick={save}>{saving ? 'Menyimpan...' : 'Simpan tanda tangan'}</Button>
      <Button type="button" variant="outline" disabled={empty || saving} onClick={clear}><Eraser aria-hidden="true" />Ulangi</Button>
      {onCancel && <Button type="button" variant="ghost" disabled={saving} onClick={onCancel}>Batal</Button>}
    </div>
  </div>;
}
