import { Check, Eraser, X } from 'lucide-react';
import { useSignatureCanvas } from '@/components/SignaturePad';
import { Button } from '@/components/ui/button';
import type { MarkPlacement } from './signatures';

type Props = {
  /** Ukuran kotak tanda tangan (CSS px). */
  width: number;
  height: number;
  label: string;
  onDone: (png: string, placement: MarkPlacement) => void;
  onCancel: () => void;
};

// Lebar cetak kotak ~45-90 mm; resolusi 4x layar cukup tajam untuk cetak.
const resolution = 4;

/**
 * Kanvas tanda tangan langsung di kotak tanda tangan yang dipilih. Coretan
 * dipotong rapi lalu ditaruh tepat di tempat dicoret (posisi dalam satuan
 * tinggi kotak, sama seperti cetak).
 */
export function InlineSignatureCanvas({ width, height, label, onDone, onCancel }: Props) {
  // Tebal garis mengikuti tinggi kotak agar hasil cetak sama di setiap zoom.
  const pad = useSignatureCanvas(height, undefined, { resolution, lineWidth: Math.max(1.5, height / 45) });
  const finish = () => {
    const crop = pad.exportCrop();
    if (!crop) return;
    onDone(crop.png, {
      x: Math.round(((crop.left + crop.width / 2 - width / 2) / height) * 1000) / 1000,
      y: Math.round(((crop.top + crop.height / 2 - height / 2) / height) * 1000) / 1000,
      size: Math.round((Math.max(crop.width, crop.height) / height) * 1000) / 1000,
      rotation: 0,
    });
  };

  return <>
    <div className="absolute inset-0 z-20 bg-white/85 ring-2 ring-primary ring-inset">
      {pad.empty && <span className="pointer-events-none absolute inset-0 grid place-items-center px-2 text-center text-xs text-muted-foreground">Coret tanda tangan di sini</span>}
      <canvas {...pad.canvasProps} aria-label={label} className="absolute inset-0 block cursor-crosshair touch-none" style={{ width, height }} />
    </div>
    <div className="absolute top-full left-1/2 z-30 mt-1 flex -translate-x-1/2 gap-1 rounded-md border bg-background p-1 shadow-md" onPointerDown={(event) => event.stopPropagation()}>
      <Button type="button" size="sm" disabled={pad.empty} onClick={finish}><Check aria-hidden="true" />Selesai</Button>
      <Button type="button" size="sm" variant="ghost" disabled={pad.empty} onClick={pad.clear}><Eraser aria-hidden="true" />Ulangi</Button>
      <Button type="button" size="sm" variant="ghost" onClick={onCancel}><X aria-hidden="true" />Batal</Button>
    </div>
  </>;
}
