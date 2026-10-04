import { useEffect, useState } from 'react';
import { ChevronLeft, ChevronRight, Minus, Plus, RotateCcw } from 'lucide-react';
import type { MediaFile } from './types';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog';

const MIN_ZOOM = 25;
const MAX_ZOOM = 400;
const ZOOM_STEP = 25;

function clampZoom(value: number) {
  return Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, value));
}

type Props = { files: MediaFile[]; index: number | null; onIndexChange: (index: number | null) => void };

export function ImagePreviewDialog({ files, index, onIndexChange }: Props) {
  const [zoom, setZoom] = useState(100);
  const open = index !== null;
  const file = index !== null ? (files[index] ?? null) : null;

  useEffect(() => { if (open) setZoom(100); }, [open, file?.id]);
  useEffect(() => {
    if (!open || files.length < 2) return;
    const handleKey = (event: KeyboardEvent) => {
      if (event.key === 'ArrowRight') onIndexChange(index! < files.length - 1 ? index! + 1 : 0);
      else if (event.key === 'ArrowLeft') onIndexChange(index! > 0 ? index! - 1 : files.length - 1);
    };
    window.addEventListener('keydown', handleKey);
    return () => window.removeEventListener('keydown', handleKey);
  }, [open, index, files.length, onIndexChange]);

  if (!file || index === null) return null;

  const updateZoom = (value: number) => setZoom(clampZoom(value));
  // React attaches onWheel as a passive listener, so preventDefault() inside
  // a synthetic handler is a silent no-op (and logs a console warning on
  // every tick). A native non-passive listener is required instead. Base UI
  // mounts the dialog's portaled content a tick after `open` flips true, so
  // a ref callback (which fires exactly when the node appears/disappears)
  // is used rather than a useEffect keyed on `open`, which would attach
  // before the node exists and never retry.
  const attachScrollNode = (node: HTMLDivElement | null) => {
    if (!node) return;
    const handleWheel = (event: globalThis.WheelEvent) => {
      event.preventDefault();
      setZoom((current) => clampZoom(current + (event.deltaY < 0 ? ZOOM_STEP : -ZOOM_STEP)));
    };
    node.addEventListener('wheel', handleWheel, { passive: false });
    return () => node.removeEventListener('wheel', handleWheel);
  };
  const goPrev = () => onIndexChange(index > 0 ? index - 1 : files.length - 1);
  const goNext = () => onIndexChange(index < files.length - 1 ? index + 1 : 0);
  const hasMultiple = files.length > 1;

  return <Dialog open={open} onOpenChange={(next) => { if (!next) onIndexChange(null); }}>
    <DialogContent aria-label={`Preview ${file.original_filename}`} className="h-[calc(100dvh-2rem)] max-w-[calc(100vw-2rem)] grid-rows-[auto_auto_minmax(0,1fr)] overflow-hidden p-0 sm:max-w-6xl">
      <DialogHeader className="border-b px-5 py-4 pr-16">
        <DialogTitle>{`Preview ${file.original_filename}`}</DialogTitle>
        {hasMultiple && <p className="text-sm text-muted-foreground">({index + 1} dari {files.length})</p>}
        <DialogDescription>Gunakan kontrol zoom atau roda mouse untuk memeriksa detail foto{hasMultiple ? ', dan panah kiri/kanan untuk pindah foto' : ''}.</DialogDescription>
      </DialogHeader>
      <div className="flex flex-wrap items-center justify-center gap-2 border-b bg-muted/40 px-4 py-2">
        <Button type="button" variant="outline" size="icon" aria-label="Perkecil foto" disabled={zoom <= MIN_ZOOM} onClick={() => updateZoom(zoom - ZOOM_STEP)}><Minus aria-hidden="true" /></Button>
        <label className="flex min-w-48 flex-1 items-center gap-3 sm:max-w-md">
          <span className="sr-only">Tingkat zoom foto</span>
          <input className="w-full accent-primary" type="range" aria-label="Tingkat zoom foto" min={MIN_ZOOM} max={MAX_ZOOM} step={ZOOM_STEP} value={zoom} onChange={(event) => updateZoom(Number(event.target.value))} />
          <output className="w-12 text-right text-sm font-medium tabular-nums">{zoom}%</output>
        </label>
        <Button type="button" variant="outline" size="icon" aria-label="Perbesar foto" disabled={zoom >= MAX_ZOOM} onClick={() => updateZoom(zoom + ZOOM_STEP)}><Plus aria-hidden="true" /></Button>
        <Button type="button" variant="outline" aria-label="Reset zoom" onClick={() => setZoom(100)}><RotateCcw aria-hidden="true" />Reset</Button>
      </div>
      <div ref={attachScrollNode} aria-label="Area preview foto" className="relative min-h-0 overflow-auto bg-black/90 p-4">
        {hasMultiple && <>
          <Button type="button" variant="outline" size="icon" aria-label="Foto sebelumnya" className="absolute left-3 top-1/2 z-10 -translate-y-1/2" onClick={goPrev}><ChevronLeft aria-hidden="true" /></Button>
          <Button type="button" variant="outline" size="icon" aria-label="Foto berikutnya" className="absolute right-3 top-1/2 z-10 -translate-y-1/2" onClick={goNext}><ChevronRight aria-hidden="true" /></Button>
        </>}
        <div className="flex min-h-full min-w-full items-center justify-center">
          <div className="flex shrink-0 items-center justify-center transition-[width,height] duration-100 motion-reduce:transition-none" style={{ width: `${zoom}%`, height: `${zoom}%` }}>
            <img src={file.content_url} alt={file.original_filename} className="max-h-full max-w-full object-contain" />
          </div>
        </div>
      </div>
    </DialogContent>
  </Dialog>;
}
