import { useMutation } from '@tanstack/react-query';
import { Download, Minus, Plus, X } from 'lucide-react';
import { lazy, Suspense, useCallback, useEffect, useRef, useState, type ComponentProps, type PointerEvent, type ReactNode } from 'react';
import { toast } from 'sonner';
import { DataState } from '@/components/DataState';
import { Button } from '@/components/ui/button';
import { Dialog, DialogClose, DialogContent, DialogTitle } from '@/components/ui/dialog';
import { cn } from '@/lib/utils';
import { fetchFile } from './api';
import { canPrintPdf, printPdf } from './printPdf';

const PdfCanvasPreview = lazy(() => import('@/features/berita-acara/PdfCanvasPreview'));

const pdfZoomStep = 25;
const pdfZoomMin = 50;
const pdfZoomMax = 300;
const imageZoomMin = 1;
const imageZoomMax = 8;
const imageZoomStep = 1.25;
const wheelSpeed = 0.0015;
const toolbarButton = 'text-white hover:bg-white/10 hover:text-white disabled:text-white/40';

type Preview = { url: string; blob: Blob; pdf: boolean };
type ViewerProps = { preview: Preview; filename: string };
type HeaderProps = ViewerProps & { percent: number; canZoomOut: boolean; canZoomIn: boolean; onZoomOut: () => void; onZoomIn: () => void; onReset: () => void };

function ViewerHeader({ preview, filename, percent, canZoomOut, canZoomIn, onZoomOut, onZoomIn, onReset }: HeaderProps) {
  return <header className="flex items-center gap-2 border-b border-white/10 px-3 py-2 sm:px-4">
    <DialogTitle className="min-w-0 flex-1 truncate text-sm font-medium">{filename}</DialogTitle>
    <div className="flex items-center" role="group" aria-label="Perbesar">
      <Button variant="ghost" size="icon-sm" className={toolbarButton} aria-label="Perkecil" disabled={!canZoomOut} onClick={onZoomOut}><Minus /></Button>
      <button type="button" className="w-12 rounded-sm py-1 text-center text-xs tabular-nums hover:bg-white/10" aria-label="Kembali ke ukuran awal" onClick={onReset}>{percent}%</button>
      <Button variant="ghost" size="icon-sm" className={toolbarButton} aria-label="Perbesar" disabled={!canZoomIn} onClick={onZoomIn}><Plus /></Button>
    </div>
    <Button variant="ghost" size="sm" className={toolbarButton} nativeButton={false} render={<a href={preview.url} download={filename} />}><Download /><span className="hidden sm:inline">Unduh</span></Button>
    <DialogClose render={<Button variant="ghost" size="icon-sm" className={toolbarButton} aria-label="Tutup" />}><X /></DialogClose>
  </header>;
}

/** Gambar pas di layar; scroll atau cubit untuk zoom, geser untuk pindah. */
function ImagePane({ preview, filename }: ViewerProps) {
  const stage = useRef<HTMLDivElement>(null);
  const image = useRef<HTMLImageElement>(null);
  const pointers = useRef(new Map<number, { x: number; y: number }>());
  const [view, setView] = useState({ scale: 1, x: 0, y: 0 });
  const [animated, setAnimated] = useState(false);
  const [dragging, setDragging] = useState(false);

  const limit = useCallback((scale: number, x: number, y: number) => {
    const roomX = Math.max(0, ((image.current?.clientWidth ?? 0) * scale - (stage.current?.clientWidth ?? 0)) / 2);
    const roomY = Math.max(0, ((image.current?.clientHeight ?? 0) * scale - (stage.current?.clientHeight ?? 0)) / 2);
    return { scale, x: Math.min(roomX, Math.max(-roomX, x)), y: Math.min(roomY, Math.max(-roomY, y)) };
  }, []);

  // Titik di bawah kursor tetap di tempatnya; px dan py diukur dari tengah layar.
  const zoomAt = useCallback((factor: number, px = 0, py = 0, smooth = false) => {
    setAnimated(smooth);
    setView((current) => {
      const scale = Math.min(imageZoomMax, Math.max(imageZoomMin, current.scale * factor));
      const ratio = scale / current.scale;
      return limit(scale, px - (px - current.x) * ratio, py - (py - current.y) * ratio);
    });
  }, [limit]);

  const fromCentre = (x: number, y: number) => {
    const box = stage.current?.getBoundingClientRect();
    return box ? { px: x - box.left - box.width / 2, py: y - box.top - box.height / 2 } : { px: 0, py: 0 };
  };

  useEffect(() => {
    const element = stage.current;
    if (!element) return;
    // Listener React bersifat pasif, sehingga tidak bisa mencegah scroll halaman.
    const onWheel = (event: WheelEvent) => {
      event.preventDefault();
      const box = element.getBoundingClientRect();
      zoomAt(Math.exp(-event.deltaY * wheelSpeed), event.clientX - box.left - box.width / 2, event.clientY - box.top - box.height / 2);
    };
    element.addEventListener('wheel', onWheel, { passive: false });
    return () => element.removeEventListener('wheel', onWheel);
  }, [zoomAt]);

  const onPointerDown = (event: PointerEvent<HTMLDivElement>) => {
    event.currentTarget.setPointerCapture?.(event.pointerId);
    pointers.current.set(event.pointerId, { x: event.clientX, y: event.clientY });
    setDragging(true);
  };
  const onPointerMove = (event: PointerEvent<HTMLDivElement>) => {
    const last = pointers.current.get(event.pointerId);
    if (!last) return;
    const next = { x: event.clientX, y: event.clientY };
    const other = [...pointers.current].find(([id]) => id !== event.pointerId)?.[1];
    pointers.current.set(event.pointerId, next);
    if (other) {
      const before = Math.hypot(last.x - other.x, last.y - other.y);
      const after = Math.hypot(next.x - other.x, next.y - other.y);
      const { px, py } = fromCentre((next.x + other.x) / 2, (next.y + other.y) / 2);
      if (before > 0) zoomAt(after / before, px, py);
      return;
    }
    setAnimated(false);
    setView((current) => limit(current.scale, current.x + next.x - last.x, current.y + next.y - last.y));
  };
  const onPointerEnd = (event: PointerEvent<HTMLDivElement>) => {
    pointers.current.delete(event.pointerId);
    if (pointers.current.size === 0) setDragging(false);
  };

  return <>
    <ViewerHeader
      preview={preview}
      filename={filename}
      percent={Math.round(view.scale * 100)}
      canZoomOut={view.scale > imageZoomMin}
      canZoomIn={view.scale < imageZoomMax}
      onZoomOut={() => zoomAt(1 / imageZoomStep, 0, 0, true)}
      onZoomIn={() => zoomAt(imageZoomStep, 0, 0, true)}
      onReset={() => { setAnimated(true); setView({ scale: 1, x: 0, y: 0 }); }}
    />
    <div
      ref={stage}
      role="group"
      aria-label="Area gambar"
      className={cn('flex h-full min-h-0 touch-none items-center justify-center overflow-hidden p-4 select-none sm:p-6', view.scale > 1 ? (dragging ? 'cursor-grabbing' : 'cursor-grab') : 'cursor-zoom-in')}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerEnd}
      onPointerCancel={onPointerEnd}
      onDoubleClick={(event) => {
        if (view.scale > 1) {
          setAnimated(true);
          setView({ scale: 1, x: 0, y: 0 });
          return;
        }
        const { px, py } = fromCentre(event.clientX, event.clientY);
        zoomAt(2.5, px, py, true);
      }}
    >
      <img
        ref={image}
        src={preview.url}
        alt={filename}
        draggable={false}
        className="max-h-full max-w-full rounded-md object-contain shadow-2xl"
        style={{ transform: `translate(${view.x}px, ${view.y}px) scale(${view.scale})`, transition: animated ? 'transform 160ms ease-out' : 'none' }}
      />
    </div>
  </>;
}

/** PDF untuk browser yang tidak bisa mencetak dari halaman. */
function PdfPane({ preview, filename }: ViewerProps) {
  const [zoom, setZoom] = useState(100);
  return <>
    <ViewerHeader
      preview={preview}
      filename={filename}
      percent={zoom}
      canZoomOut={zoom > pdfZoomMin}
      canZoomIn={zoom < pdfZoomMax}
      onZoomOut={() => setZoom(zoom - pdfZoomStep)}
      onZoomIn={() => setZoom(zoom + pdfZoomStep)}
      onReset={() => setZoom(100)}
    />
    <div className="min-h-0 overflow-auto">
      <div className="mx-auto p-3 sm:p-6" style={{ width: `calc(min(100%, 60rem) * ${zoom / 100})` }}>
        <Suspense fallback={<DataState kind="loading" title="Menyiapkan preview" description="Memuat penampil PDF." />}>
          <PdfCanvasPreview blob={preview.blob} label={filename} className="grid gap-4" />
        </Suspense>
      </div>
    </div>
  </>;
}

/** Penampil layar penuh, untuk gambar dan browser yang tidak bisa mencetak PDF. */
function FileViewer({ preview, filename, onClose }: ViewerProps & { onClose: () => void }) {
  return <Dialog open onOpenChange={(next) => { if (!next) onClose(); }}>
    <DialogContent showCloseButton={false} className="h-dvh max-h-dvh w-screen max-w-none grid-rows-[auto_minmax(0,1fr)] gap-0 overflow-hidden rounded-none bg-neutral-900 p-0 text-white ring-0 sm:max-w-none">
      {preview.pdf ? <PdfPane preview={preview} filename={filename} /> : <ImagePane preview={preview} filename={filename} />}
    </DialogContent>
  </Dialog>;
}

/** PDF langsung ke dialog cetak; gambar dan browser HP memakai penampil. */
export function useFilePreview(filename: string) {
  const [preview, setPreview] = useState<Preview | null>(null);
  const show = (blob: Blob) => {
    const pdf = blob.type === 'application/pdf';
    if (pdf && canPrintPdf()) printPdf(blob);
    else setPreview({ url: URL.createObjectURL(blob), blob, pdf });
  };
  const close = () => {
    if (preview) URL.revokeObjectURL(preview.url);
    setPreview(null);
  };
  return { show, viewer: preview && <FileViewer preview={preview} filename={filename} onClose={close} /> };
}

type FilePreviewButtonProps = Omit<ComponentProps<typeof Button>, 'onClick'> & { path: string; filename: string; children: ReactNode };

export function FilePreviewButton({ path, filename, children, ...button }: FilePreviewButtonProps) {
  const { show, viewer } = useFilePreview(filename);
  const load = useMutation({
    mutationFn: () => fetchFile(path),
    onSuccess: show,
    onError: () => toast.error('Berkas belum dapat dibuka. Periksa koneksi lalu coba lagi.'),
  });
  return <>
    <Button type="button" {...button} disabled={load.isPending} onClick={() => load.mutate()}>{children}</Button>
    {viewer}
  </>;
}
