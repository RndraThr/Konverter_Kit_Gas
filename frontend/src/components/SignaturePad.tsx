import { type PointerEvent, useCallback, useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { Brush, ChevronDown, Eraser, Feather, Highlighter, type LucideIcon, Maximize2, Minimize2, Palette, Pen, Pencil, PenTool, Redo2, Undo2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { isSignaturePen, paintStroke, signaturePens, strokesBounds, type SignaturePen, type SignatureStroke } from './signaturePens';

const trimPadding = 12;

/** Hasil coretan yang dipotong: PNG base64 dan letaknya di kanvas (CSS px). */
export type SignatureCrop = { png: string; left: number; top: number; width: number; height: number };

type Options = {
  /** Skala minimum backing store; kanvas kecil (kotak tabel BA) perlu lebih tajam dari layar. */
  resolution?: number;
  /** Tebal garis (CSS px). */
  lineWidth?: number;
  /** Warna tinta untuk coretan berikutnya. */
  color?: string;
  /** Jenis pena untuk coretan berikutnya; tebalnya lineWidth. */
  pen?: SignaturePen;
};

/** Tinta yang lazim untuk tanda tangan (sama dengan aplikasi HP). */
export const signatureInks = [
  { color: '#111111', label: 'Hitam' },
  { color: '#1e2d6e', label: 'Biru tua' },
  { color: '#1e5bd8', label: 'Biru' },
  { color: '#0b3d91', label: 'Biru royal' },
  { color: '#4a2c82', label: 'Ungu' },
  { color: '#1f5130', label: 'Hijau tua' },
  { color: '#7a1f2b', label: 'Marun' },
  { color: '#3a4047', label: 'Abu arang' },
];

/** Warna dasar (paraf, catatan, atau selera penandatangan). */
export const basicInks = [
  { color: '#d32f2f', label: 'Merah' },
  { color: '#f57c00', label: 'Oranye' },
  { color: '#f9c80e', label: 'Kuning' },
  { color: '#2e9d46', label: 'Hijau' },
  { color: '#0097a7', label: 'Tosca' },
  { color: '#8e24aa', label: 'Ungu muda' },
  { color: '#d81b60', label: 'Merah muda' },
  { color: '#6d4c41', label: 'Cokelat' },
];

export const defaultInk = '#1e2d6e';
const inkKey = 'konkit.signature.ink';
const penKey = 'konkit.signature.pen';
const sizeKey = 'konkit.signature.size';
export const minPenSize = 1.5;
export const maxPenSize = 8;
/** Bawaan pad penerima, sama dengan HP: pena tinta, tebal 5. */
export const defaultPen: SignaturePen = 'tinta';
export const defaultPenSize = 5;

/** Tinta terakhir yang dipakai di browser ini. */
export function readInk() {
  try {
    const value = window.localStorage.getItem(inkKey) ?? '';
    return /^#[0-9a-f]{6}$/i.test(value) ? value.toLowerCase() : defaultInk;
  } catch {
    return defaultInk;
  }
}

export function rememberInk(color: string) {
  try {
    window.localStorage.setItem(inkKey, color);
  } catch {
    // Hanya kemudahan: pad tetap jalan tanpa penyimpanan.
  }
}

/** Pena dan tebal terakhir yang dipakai di browser ini. */
export function readPen(): { pen: SignaturePen; size: number } {
  try {
    const pen = window.localStorage.getItem(penKey);
    const size = Number(window.localStorage.getItem(sizeKey) ?? '');
    return {
      pen: isSignaturePen(pen) ? pen : defaultPen,
      size: Number.isFinite(size) && size >= minPenSize && size <= maxPenSize ? size : defaultPenSize,
    };
  } catch {
    return { pen: defaultPen, size: defaultPenSize };
  }
}

function rememberPen(pen: SignaturePen, size: number) {
  try {
    window.localStorage.setItem(penKey, pen);
    window.localStorage.setItem(sizeKey, String(size));
  } catch {
    // Hanya kemudahan: pad tetap jalan tanpa penyimpanan.
  }
}

/**
 * Logika kanvas tanda tangan: digambar dengan jari, pena, atau tetikus di
 * kanvas transparan, lalu diekspor sebagai PNG base64 yang dipotong sesuai
 * coretan. Coretan disimpan (jenis pena, warna, tebal, titik) agar bisa
 * dibatalkan/diulang dan digambar seperti di HP. Dipakai pad POS Dokumen dan
 * editor penandatangan Berita Acara.
 */
export function useSignatureCanvas(height: number, onStroke?: () => void, options: Options = {}) {
  const { resolution = 1, lineWidth = 2.6, color = defaultInk, pen = 'pulpen' } = options;
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const style = useRef({ color, pen, size: lineWidth });
  style.current = { color, pen, size: lineWidth };
  const strokes = useRef<SignatureStroke[]>([]);
  const undone = useRef<SignatureStroke[]>([]);
  const current = useRef<SignatureStroke | null>(null);
  // Coretan selesai digambar sekali ke kanvas offscreen, jadi saat menggerakkan
  // pena hanya coretan yang sedang dibuat yang digambar ulang.
  const committed = useRef<HTMLCanvasElement | null>(null);
  const [counts, setCounts] = useState({ strokes: 0, undone: 0 });
  const sync = () => setCounts({ strokes: strokes.current.length, undone: undone.current.length });

  const ratio = () => {
    const canvas = canvasRef.current;
    return canvas && canvas.clientWidth > 0 ? canvas.width / canvas.clientWidth : 1;
  };

  const show = useCallback(() => {
    const canvas = canvasRef.current;
    const context = canvas?.getContext('2d');
    if (!canvas || !context) return;
    context.setTransform(1, 0, 0, 1, 0, 0);
    context.clearRect(0, 0, canvas.width, canvas.height);
    if (committed.current) context.drawImage(committed.current, 0, 0);
    if (current.current) {
      const scale = ratio();
      context.setTransform(scale, 0, 0, scale, 0, 0);
      paintStroke(context, current.current);
    }
  }, []);

  const rebuild = useCallback(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const offscreen = document.createElement('canvas');
    offscreen.width = canvas.width;
    offscreen.height = canvas.height;
    const context = offscreen.getContext('2d');
    if (context) {
      const scale = ratio();
      context.setTransform(scale, 0, 0, scale, 0, 0);
      for (const stroke of strokes.current) paintStroke(context, stroke);
    }
    committed.current = context ? offscreen : null;
    show();
  }, [show]);

  // Backing store at device resolution so strokes stay sharp, refitted when
  // the box changes size (full screen, a turned phone). Strokes are kept in
  // CSS px, so they are redrawn in place; a new canvas element (moved to the
  // full-screen layer) is picked up through the callback ref.
  const [canvasElement, setCanvasElement] = useState<HTMLCanvasElement | null>(null);
  const attach = useCallback((element: HTMLCanvasElement | null) => {
    canvasRef.current = element;
    setCanvasElement(element);
  }, []);
  useEffect(() => {
    const canvas = canvasElement;
    if (!canvas) return;
    const fit = () => {
      const scale = Math.max(window.devicePixelRatio || 1, resolution);
      const width = Math.round(canvas.clientWidth * scale);
      const nextHeight = Math.round((canvas.clientHeight || height) * scale);
      if (canvas.width === width && canvas.height === nextHeight && committed.current) return;
      canvas.width = width;
      canvas.height = nextHeight;
      rebuild();
    };
    fit();
    if (typeof ResizeObserver === 'undefined') return;
    const observer = new ResizeObserver(fit);
    observer.observe(canvas);
    return () => observer.disconnect();
  }, [canvasElement, height, resolution, rebuild]);

  const point = (event: { clientX: number; clientY: number }, rect: DOMRect) => ({ x: event.clientX - rect.left, y: event.clientY - rect.top });
  const onPointerDown = (event: PointerEvent<HTMLCanvasElement>) => {
    event.currentTarget.setPointerCapture?.(event.pointerId);
    const rect = event.currentTarget.getBoundingClientRect();
    const { color: ink, pen: kind, size } = style.current;
    current.current = { pen: kind, color: ink, size, points: [point(event, rect)], times: [event.timeStamp] };
    if (strokes.current.length === 0) onStroke?.();
    show();
  };
  const onPointerMove = (event: PointerEvent<HTMLCanvasElement>) => {
    const stroke = current.current;
    if (!stroke) return;
    const rect = event.currentTarget.getBoundingClientRect();
    // Coalesced events keep fast strokes smooth on high-rate pens and screens.
    const events = event.nativeEvent.getCoalescedEvents?.() ?? [];
    for (const item of events.length > 0 ? events : [event]) {
      stroke.points.push(point(item, rect));
      stroke.times.push(item.timeStamp);
    }
    show();
  };
  const end = () => {
    const stroke = current.current;
    if (!stroke) return;
    current.current = null;
    strokes.current.push(stroke);
    undone.current = [];
    const context = committed.current?.getContext('2d');
    if (context) paintStroke(context, stroke);
    else rebuild();
    show();
    sync();
  };

  const undo = useCallback(() => {
    const stroke = strokes.current.pop();
    if (!stroke) return;
    undone.current.push(stroke);
    rebuild();
    sync();
  }, [rebuild]);

  const redo = useCallback(() => {
    const stroke = undone.current.pop();
    if (!stroke) return;
    strokes.current.push(stroke);
    rebuild();
    sync();
  }, [rebuild]);

  const clear = useCallback(() => {
    strokes.current = [];
    undone.current = [];
    current.current = null;
    rebuild();
    sync();
  }, [rebuild]);

  // Crop to the strokes (plus a margin) so the signature fills the Berita Acara box.
  const exportPNG = useCallback(() => {
    const canvas = canvasRef.current;
    const b = strokesBounds(strokes.current);
    if (!canvas || !b || canvas.clientWidth === 0) return '';
    const scale = canvas.width / canvas.clientWidth;
    const x = Math.max(0, (b.minX - trimPadding) * scale);
    const y = Math.max(0, (b.minY - trimPadding) * scale);
    const width = Math.max(60, Math.min(canvas.width - x, (b.maxX - b.minX + trimPadding * 2) * scale));
    const outHeight = Math.max(60, Math.min(canvas.height - y, (b.maxY - b.minY + trimPadding * 2) * scale));
    const output = document.createElement('canvas');
    output.width = Math.round(width);
    output.height = Math.round(outHeight);
    output.getContext('2d')?.drawImage(committed.current ?? canvas, x, y, width, outHeight, 0, 0, output.width, output.height);
    return output.toDataURL('image/png').split(',')[1] ?? '';
  }, []);

  // Seperti exportPNG tetapi tanpa ukuran minimum, dan mengembalikan letak
  // potongan agar tanda tangan bisa ditaruh tepat di tempat dicoret.
  const exportCrop = useCallback((padding = 4): SignatureCrop | null => {
    const canvas = canvasRef.current;
    const b = strokesBounds(strokes.current);
    if (!canvas || !b || canvas.clientWidth === 0) return null;
    const scale = canvas.width / canvas.clientWidth;
    const left = Math.max(0, b.minX - padding);
    const top = Math.max(0, b.minY - padding);
    const width = Math.max(1, Math.min(canvas.clientWidth - left, b.maxX - b.minX + padding * 2));
    const cropHeight = Math.max(1, Math.min(canvas.clientHeight - top, b.maxY - b.minY + padding * 2));
    const output = document.createElement('canvas');
    output.width = Math.round(width * scale);
    output.height = Math.round(cropHeight * scale);
    output.getContext('2d')?.drawImage(committed.current ?? canvas, left * scale, top * scale, width * scale, cropHeight * scale, 0, 0, output.width, output.height);
    return { png: output.toDataURL('image/png').split(',')[1] ?? '', left, top, width, height: cropHeight };
  }, []);

  return {
    empty: counts.strokes === 0,
    canUndo: counts.strokes > 0,
    canRedo: counts.undone > 0,
    undo,
    redo,
    clear,
    exportPNG,
    exportCrop,
    canvasProps: { ref: attach, onPointerDown, onPointerMove, onPointerUp: end, onPointerCancel: end, onPointerLeave: end },
  };
}

/** "#rrggbb" ke [r, g, b]. */
function hexRGB(color: string): [number, number, number] {
  const value = Number.parseInt(color.slice(1), 16);
  return [(value >> 16) & 255, (value >> 8) & 255, value & 255];
}

/**
 * Mewarnai ulang piksel tanda tangan: warna coretan diganti, transparansinya
 * (termasuk tepi halus) tetap, sehingga bentuk coretan sama persis.
 */
export function tintPixels(data: Uint8ClampedArray, color: string) {
  const [r, g, b] = hexRGB(color);
  for (let i = 0; i < data.length; i += 4) {
    if (data[i + 3] === 0) continue;
    data[i] = r;
    data[i + 1] = g;
    data[i + 2] = b;
  }
}

/** Tanda tangan tersimpan (PNG transparan) dengan warna tinta baru, sebagai PNG base64. */
export async function recolorSignature(image: Blob, color: string) {
  const bitmap = await createImageBitmap(image);
  const canvas = document.createElement('canvas');
  canvas.width = bitmap.width;
  canvas.height = bitmap.height;
  const context = canvas.getContext('2d');
  if (!context) throw new Error('Canvas tidak didukung');
  context.drawImage(bitmap, 0, 0);
  bitmap.close?.();
  const pixels = context.getImageData(0, 0, canvas.width, canvas.height);
  tintPixels(pixels.data, color);
  context.putImageData(pixels, 0, 0);
  return canvas.toDataURL('image/png').split(',')[1] ?? '';
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

function Swatch({ color, label, selected, onPick }: { color: string; label: string; selected: boolean; onPick: (color: string) => void }) {
  return <button
    type="button"
    title={label}
    aria-label={`Tinta ${label}`}
    aria-pressed={selected}
    className={cn('size-7 shrink-0 rounded-full border border-black/15 outline-offset-2 transition-transform hover:scale-110', selected && 'outline-2 outline-primary')}
    style={{ backgroundColor: color }}
    onClick={() => onPick(color)}
  />;
}

/** Pilihan warna tinta: tinta tanda tangan, warna dasar, dan warna sendiri. */
export function InkPicker({ value, onChange }: { value: string; onChange: (color: string) => void }) {
  const known = [...signatureInks, ...basicInks].find((ink) => ink.color === value);
  return <div className="space-y-2" role="group" aria-label="Warna tinta">
    <div className="flex flex-wrap items-center gap-1.5 sm:gap-2">
      <span className="w-full text-xs text-muted-foreground sm:w-28">Tinta tanda tangan</span>
      {signatureInks.map((ink) => <Swatch key={ink.color} {...ink} selected={ink.color === value} onPick={onChange} />)}
    </div>
    <div className="flex flex-wrap items-center gap-1.5 sm:gap-2">
      <span className="w-full text-xs text-muted-foreground sm:w-28">Warna dasar</span>
      {basicInks.map((ink) => <Swatch key={ink.color} {...ink} selected={ink.color === value} onPick={onChange} />)}
    </div>
    <div className="flex items-center gap-2 sm:pl-30">
      <label className="inline-flex cursor-pointer items-center gap-1.5 text-xs text-muted-foreground" title="Warna sendiri">
        <Palette className="size-4" aria-hidden="true" />
        <input type="color" aria-label="Warna sendiri" className="size-7 cursor-pointer rounded border-0 bg-transparent p-0" value={value} onChange={(event) => onChange(event.target.value.toLowerCase())} />
      </label>
      <span className="text-xs text-muted-foreground">{known?.label ?? 'Warna sendiri'}</span>
    </div>
  </div>;
}

const penIcons: Record<SignaturePen, LucideIcon> = {
  pulpen: Pen,
  tinta: PenTool,
  kaligrafi: Feather,
  kuas: Brush,
  pensil: Pencil,
  spidol: Highlighter,
};


/** Jenis pena dan tebalnya, seperti toolbar pena di HP. */
export function PenPicker({ pen, size, onChange }: { pen: SignaturePen; size: number; onChange: (pen: SignaturePen, size: number) => void }) {
  return <div className="space-y-2">
    <div className="grid grid-cols-3 gap-1.5 sm:grid-cols-6" role="group" aria-label="Jenis pena">
      {signaturePens.map((item) => {
        const Icon = penIcons[item.pen];
        const selected = item.pen === pen;
        return <button
          key={item.pen}
          type="button"
          aria-label={item.label}
          aria-pressed={selected}
          className={cn('flex min-h-11 flex-col items-center justify-center gap-0.5 rounded-md border px-1 text-xs leading-tight transition-colors', selected ? 'border-primary bg-primary text-primary-foreground' : 'bg-background hover:bg-muted')}
          onClick={() => onChange(item.pen, size)}
        >
          <Icon className="size-4" aria-hidden="true" /><span className="truncate">{item.label}</span>
        </button>;
      })}
    </div>
    <label className="flex items-center gap-2 text-xs text-muted-foreground">
      Tebal
      <input type="range" aria-label="Tebal pena" className="h-6 flex-1 accent-primary" min={minPenSize} max={maxPenSize} step={0.5} value={size} onChange={(event) => onChange(pen, Number(event.target.value))} />
      <span className="w-6 text-right tabular-nums">{size}</span>
    </label>
  </div>;
}

/**
 * Pad tanda tangan berdiri sendiri dengan tombol Simpan/Ulangi (POS Dokumen).
 * Satu baris alat yang ringkas agar kotak coretan tetap lega di HP; pena dan
 * warna dibuka bila perlu, dan pad bisa dibuat layar penuh seperti di HP.
 */
export function SignaturePad({ saving, onSave, onCancel, label = 'Area tanda tangan penerima', hint = 'Minta penerima menandatangani di kotak putih.', height = 220 }: Props) {
  const [ink, setInk] = useState(readInk);
  const [penStyle, setPenStyle] = useState(readPen);
  const [toolsOpen, setToolsOpen] = useState(false);
  const [fullscreen, setFullscreen] = useState(false);
  const pickInk = (color: string) => { setInk(color); rememberInk(color); };
  const pickPen = (pen: SignaturePen, size: number) => { setPenStyle({ pen, size }); rememberPen(pen, size); };
  const pad = useSignatureCanvas(height, undefined, { color: ink, pen: penStyle.pen, lineWidth: penStyle.size });
  const penLabel = signaturePens.find((item) => item.pen === penStyle.pen)?.label ?? '';
  const PenIcon = penIcons[penStyle.pen];

  // Layar penuh: halaman di belakang tidak ikut bergulir, Esc menutup.
  useEffect(() => {
    if (!fullscreen) return;
    const previous = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    const onKey = (event: KeyboardEvent) => { if (event.key === 'Escape') setFullscreen(false); };
    window.addEventListener('keydown', onKey);
    return () => { document.body.style.overflow = previous; window.removeEventListener('keydown', onKey); };
  }, [fullscreen]);

  const save = () => { onSave(pad.exportPNG()); setFullscreen(false); };
  const content = <>
    {fullscreen && <h2 className="text-base font-semibold [@media(max-height:500px)]:hidden">Tanda tangan penerima</h2>}
    <div className="flex items-center gap-1.5">
      <Button type="button" variant="outline" className="min-w-0 flex-1 justify-start gap-1.5 px-3 sm:flex-none" aria-label="Pena & warna" aria-expanded={toolsOpen} onClick={() => setToolsOpen((open) => !open)}>
        <PenIcon aria-hidden="true" />
        <span className="truncate text-sm">{penLabel}</span>
        <span className="size-4 shrink-0 rounded-full border border-black/15" style={{ backgroundColor: ink }} aria-hidden="true" />
        <ChevronDown className={cn('transition-transform max-[400px]:hidden', toolsOpen && 'rotate-180')} aria-hidden="true" />
      </Button>
      <div className="flex shrink-0 gap-1 sm:ml-auto">
        <Button type="button" variant="outline" size="icon" aria-label="Batalkan coretan" title="Batalkan coretan" disabled={!pad.canUndo || saving} onClick={pad.undo}><Undo2 aria-hidden="true" /></Button>
        <Button type="button" variant="outline" size="icon" aria-label="Kembalikan coretan" title="Kembalikan coretan" disabled={!pad.canRedo || saving} onClick={pad.redo}><Redo2 aria-hidden="true" /></Button>
        {fullscreen
          ? <Button type="button" variant="outline" size="icon" aria-label="Keluar layar penuh" title="Keluar layar penuh" onClick={() => setFullscreen(false)}><Minimize2 aria-hidden="true" /></Button>
          : <Button type="button" variant="outline" size="icon" aria-label="Layar penuh" title="Layar penuh" onClick={() => { setFullscreen(true); setToolsOpen(false); }}><Maximize2 aria-hidden="true" /></Button>}
      </div>
    </div>
    {toolsOpen && <div className={cn('space-y-3 rounded-md border bg-muted/30 p-3', fullscreen && 'max-h-[45dvh] overflow-y-auto')}>
      <PenPicker pen={penStyle.pen} size={penStyle.size} onChange={pickPen} />
      <InkPicker value={ink} onChange={pickInk} />
    </div>}
    <canvas
      {...pad.canvasProps}
      onPointerDown={(event) => { setToolsOpen(false); pad.canvasProps.onPointerDown(event); }}
      aria-label={label}
      className={cn('block w-full touch-none rounded-md border border-dashed bg-white', fullscreen && 'min-h-0 flex-1')}
      style={fullscreen ? undefined : { height }}
    />
    <p className="text-xs text-muted-foreground [@media(max-height:500px)]:hidden">{fullscreen ? 'Putar HP ke samping untuk kotak yang lebih lebar.' : hint}</p>
    <div className="flex gap-2">
      <Button type="button" className="flex-1 sm:flex-none" disabled={pad.empty || saving} onClick={save}>{saving ? 'Menyimpan...' : 'Simpan tanda tangan'}</Button>
      <Button type="button" variant="outline" aria-label="Ulangi" title="Hapus semua coretan" disabled={pad.empty || saving} onClick={pad.clear}><Eraser aria-hidden="true" /><span className="max-sm:hidden">Ulangi</span></Button>
      {onCancel && <Button type="button" variant="ghost" disabled={saving} onClick={() => { setFullscreen(false); onCancel(); }}>Batal</Button>}
    </div>
  </>;

  return fullscreen
    ? createPortal(<div role="dialog" aria-modal="true" aria-label="Tanda tangan layar penuh" className="fixed inset-0 z-50 flex flex-col gap-2 bg-background p-3 pb-[max(0.75rem,env(safe-area-inset-bottom))]">{content}</div>, document.body)
    : <div className="space-y-2">{content}</div>;
}
