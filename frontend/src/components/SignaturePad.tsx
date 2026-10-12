import { type PointerEvent, useCallback, useEffect, useRef, useState } from 'react';
import { Eraser } from 'lucide-react';
import { Button } from '@/components/ui/button';

const trimPadding = 12;

type Bounds = { minX: number; minY: number; maxX: number; maxY: number };

/**
 * Logika kanvas tanda tangan: digambar dengan jari, pena, atau tetikus di
 * kanvas transparan, lalu diekspor sebagai PNG base64 yang dipotong sesuai
 * coretan. Dipakai pad POS Dokumen dan kartu penandatangan Berita Acara.
 */
export function useSignatureCanvas(height: number, onStroke?: () => void) {
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
    canvas.height = Math.round(height * ratio);
    const context = canvas.getContext('2d');
    if (!context) return;
    context.scale(ratio, ratio);
    context.lineCap = 'round';
    context.lineJoin = 'round';
    context.lineWidth = 2.6;
    context.strokeStyle = '#1e2d6e';
    context.fillStyle = '#1e2d6e';
  }, [height]);

  const point = (event: PointerEvent<HTMLCanvasElement>) => {
    const rect = event.currentTarget.getBoundingClientRect();
    return { x: event.clientX - rect.left, y: event.clientY - rect.top };
  };
  const extend = (x: number, y: number) => {
    const b = bounds.current;
    bounds.current = b ? { minX: Math.min(b.minX, x), minY: Math.min(b.minY, y), maxX: Math.max(b.maxX, x), maxY: Math.max(b.maxY, y) } : { minX: x, minY: y, maxX: x, maxY: y };
  };
  const onPointerDown = (event: PointerEvent<HTMLCanvasElement>) => {
    event.currentTarget.setPointerCapture(event.pointerId);
    const p = point(event);
    last.current = p;
    extend(p.x, p.y);
    const context = canvasRef.current?.getContext('2d');
    context?.beginPath();
    context?.arc(p.x, p.y, 1.3, 0, Math.PI * 2);
    context?.fill();
    if (empty) onStroke?.();
    setEmpty(false);
  };
  const onPointerMove = (event: PointerEvent<HTMLCanvasElement>) => {
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

  const clear = useCallback(() => {
    const canvas = canvasRef.current;
    canvas?.getContext('2d')?.clearRect(0, 0, canvas.width, canvas.height);
    bounds.current = null;
    setEmpty(true);
  }, []);

  // Crop to the strokes (plus a margin) so the signature fills the Berita Acara box.
  const exportPNG = useCallback(() => {
    const canvas = canvasRef.current;
    const b = bounds.current;
    if (!canvas || !b) return '';
    const ratio = canvas.width / canvas.clientWidth;
    const x = Math.max(0, (b.minX - trimPadding) * ratio);
    const y = Math.max(0, (b.minY - trimPadding) * ratio);
    const width = Math.max(60, Math.min(canvas.width - x, (b.maxX - b.minX + trimPadding * 2) * ratio));
    const outHeight = Math.max(60, Math.min(canvas.height - y, (b.maxY - b.minY + trimPadding * 2) * ratio));
    const output = document.createElement('canvas');
    output.width = Math.round(width);
    output.height = Math.round(outHeight);
    output.getContext('2d')?.drawImage(canvas, x, y, width, outHeight, 0, 0, output.width, output.height);
    return output.toDataURL('image/png').split(',')[1] ?? '';
  }, []);

  return {
    empty,
    clear,
    exportPNG,
    canvasProps: { ref: canvasRef, onPointerDown, onPointerMove, onPointerUp: end, onPointerCancel: end, onPointerLeave: end },
  };
}

type Props = {
  saving: boolean;
  onSave: (png: string) => void;
  onCancel?: () => void;
  /** Label aksesibel kanvas. */
  label?: string;
  hint?: string;
  height?: number;
};

/** Pad tanda tangan berdiri sendiri dengan tombol Simpan/Ulangi (POS Dokumen). */
export function SignaturePad({ saving, onSave, onCancel, label = 'Area tanda tangan penerima', hint = 'Minta penerima menandatangani di kotak putih.', height = 220 }: Props) {
  const pad = useSignatureCanvas(height);
  return <div className="space-y-2">
    <canvas
      {...pad.canvasProps}
      aria-label={label}
      className="block w-full touch-none rounded-md border border-dashed bg-white"
      style={{ height }}
    />
    <p className="text-xs text-muted-foreground">{hint}</p>
    <div className="flex flex-wrap gap-2">
      <Button type="button" disabled={pad.empty || saving} onClick={() => onSave(pad.exportPNG())}>{saving ? 'Menyimpan...' : 'Simpan tanda tangan'}</Button>
      <Button type="button" variant="outline" disabled={pad.empty || saving} onClick={pad.clear}><Eraser aria-hidden="true" />Ulangi</Button>
      {onCancel && <Button type="button" variant="ghost" disabled={saving} onClick={onCancel}>Batal</Button>}
    </div>
  </div>;
}
