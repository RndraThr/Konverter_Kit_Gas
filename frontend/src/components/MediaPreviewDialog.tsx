import { useEffect, useRef, useState, type MouseEvent as ReactMouseEvent, type PointerEvent as ReactPointerEvent } from 'react';
import { ChevronLeft, ChevronRight, Minus, Plus, RotateCcw } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog';
import { cn } from '@/lib/utils';

const MIN_ZOOM = 25;
const MAX_ZOOM = 400;
const ZOOM_STEP = 25;

function clampZoom(value: number) {
  return Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, value));
}

export type PreviewMediaItem = { id: string; url: string; title: string; mediaType?: 'image' | 'video' };
type Props = { items: PreviewMediaItem[]; index: number | null; onIndexChange: (index: number | null) => void };
type PointSnapshot = { x: number; y: number };

// Converts a pointer's viewport coordinates into a position relative to the
// gesture surface's own center — the origin `translate()` pans around.
function focalPoint(node: HTMLElement, clientX: number, clientY: number): PointSnapshot {
  const rect = node.getBoundingClientRect();
  return { x: clientX - (rect.left + rect.width / 2), y: clientY - (rect.top + rect.height / 2) };
}

/**
 * Full-screen lightbox used for every photo/video documentation gallery in
 * the app (distribution slots, activity documentation). Images support
 * wheel/pinch/drag zoom and pan; video plays with native controls instead —
 * zoom gestures on a playing video fight with scrubbing, so they're skipped.
 */
export function MediaPreviewDialog({ items, index, onIndexChange }: Props) {
  const [zoom, setZoom] = useState(100);
  const [pan, setPan] = useState<PointSnapshot>({ x: 0, y: 0 });
  const [isGesturing, setIsGesturing] = useState(false);
  const open = index !== null;
  const item = index !== null ? (items[index] ?? null) : null;
  const isVideo = item?.mediaType === 'video';
  const pointersRef = useRef(new Map<number, PointSnapshot>());
  const panOriginRef = useRef<{ pan: PointSnapshot; x: number; y: number } | null>(null);
  const pinchOriginRef = useRef<{ distance: number; zoom: number; focal: PointSnapshot } | null>(null);

  useEffect(() => {
    if (!open) return;
    setZoom(100);
    setPan({ x: 0, y: 0 });
    pointersRef.current.clear();
    panOriginRef.current = null;
    pinchOriginRef.current = null;
    setIsGesturing(false);
  }, [open, item?.id]);
  useEffect(() => {
    if (!open || items.length < 2) return;
    const handleKey = (event: KeyboardEvent) => {
      if (event.key === 'ArrowRight') onIndexChange(index! < items.length - 1 ? index! + 1 : 0);
      else if (event.key === 'ArrowLeft') onIndexChange(index! > 0 ? index! - 1 : items.length - 1);
    };
    window.addEventListener('keydown', handleKey);
    return () => window.removeEventListener('keydown', handleKey);
  }, [open, index, items.length, onIndexChange]);

  if (!item || index === null) return null;

  // Zooms while keeping whichever point of the image sits under `focal`
  // (container-center-relative) visually fixed in place — the same anchored
  // feel as a native photo app, instead of always zooming from the middle.
  const applyZoom = (nextZoom: number, focal: PointSnapshot = { x: 0, y: 0 }) => {
    const clamped = clampZoom(nextZoom);
    const ratio = clamped / zoom;
    setPan((current) => ({ x: focal.x * (1 - ratio) + current.x * ratio, y: focal.y * (1 - ratio) + current.y * ratio }));
    setZoom(clamped);
  };
  const resetZoom = () => { setZoom(100); setPan({ x: 0, y: 0 }); };

  // React attaches onWheel as a passive listener, so preventDefault() inside
  // a synthetic handler is a silent no-op (and logs a console warning on
  // every tick). A native non-passive listener is required instead. Base UI
  // mounts the dialog's portaled content a tick after `open` flips true, so
  // a ref callback (which fires exactly when the node appears/disappears)
  // is used rather than a useEffect keyed on `open`, which would attach
  // before the node exists and never retry.
  const attachGestureNode = (node: HTMLDivElement | null) => {
    if (!node || isVideo) return;
    const handleWheel = (event: globalThis.WheelEvent) => {
      event.preventDefault();
      applyZoom(zoom + (event.deltaY < 0 ? ZOOM_STEP : -ZOOM_STEP), focalPoint(node, event.clientX, event.clientY));
    };
    node.addEventListener('wheel', handleWheel, { passive: false });
    return () => node.removeEventListener('wheel', handleWheel);
  };

  // One pointer drags to pan; two pointers pinch-zoom anchored at their
  // midpoint. Pointer Events (unlike Touch Events) aren't forced passive by
  // React, so no native-listener workaround is needed here.
  const handlePointerDown = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (isVideo) return;
    const node = event.currentTarget;
    node.setPointerCapture?.(event.pointerId);
    pointersRef.current.set(event.pointerId, { x: event.clientX, y: event.clientY });
    setIsGesturing(true);
    if (pointersRef.current.size === 2) {
      panOriginRef.current = null;
      const [a, b] = Array.from(pointersRef.current.values());
      pinchOriginRef.current = { distance: Math.hypot(a.x - b.x, a.y - b.y), zoom, focal: focalPoint(node, (a.x + b.x) / 2, (a.y + b.y) / 2) };
    } else if (pointersRef.current.size === 1) {
      panOriginRef.current = { pan, x: event.clientX, y: event.clientY };
    }
  };
  const handlePointerMove = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (!pointersRef.current.has(event.pointerId)) return;
    pointersRef.current.set(event.pointerId, { x: event.clientX, y: event.clientY });
    if (pointersRef.current.size === 2 && pinchOriginRef.current) {
      const [a, b] = Array.from(pointersRef.current.values());
      const distance = Math.hypot(a.x - b.x, a.y - b.y);
      applyZoom(Math.round(pinchOriginRef.current.zoom * (distance / pinchOriginRef.current.distance)), pinchOriginRef.current.focal);
      return;
    }
    if (panOriginRef.current && pointersRef.current.size === 1) {
      setPan({ x: panOriginRef.current.pan.x + (event.clientX - panOriginRef.current.x), y: panOriginRef.current.pan.y + (event.clientY - panOriginRef.current.y) });
    }
  };
  const endPointer = (event: ReactPointerEvent<HTMLDivElement>) => {
    pointersRef.current.delete(event.pointerId);
    if (pointersRef.current.size < 2) pinchOriginRef.current = null;
    if (pointersRef.current.size === 0) { panOriginRef.current = null; setIsGesturing(false); }
  };
  const handleDoubleClick = (event: ReactMouseEvent<HTMLDivElement>) => {
    if (isVideo) return;
    if (zoom > 100) { resetZoom(); return; }
    applyZoom(200, focalPoint(event.currentTarget, event.clientX, event.clientY));
  };
  const goPrev = () => onIndexChange(index > 0 ? index - 1 : items.length - 1);
  const goNext = () => onIndexChange(index < items.length - 1 ? index + 1 : 0);
  const hasMultiple = items.length > 1;

  return <Dialog open={open} onOpenChange={(next) => { if (!next) onIndexChange(null); }}>
    <DialogContent
      aria-label={`Preview ${item.title}`}
      className="flex h-dvh max-h-dvh w-screen max-w-none! flex-col gap-0 rounded-none border-0 bg-black p-0 ring-0 sm:max-w-none **:data-[slot=dialog-close]:text-white **:data-[slot=dialog-close]:hover:bg-white/15 **:data-[slot=dialog-close]:hover:text-white"
    >
      <DialogTitle className="sr-only">{`Preview ${item.title}`}</DialogTitle>
      <DialogDescription className="sr-only">
        {isVideo ? 'Gunakan kontrol pemutar video.' : 'Gunakan kontrol zoom, roda mouse, atau cubit layar untuk memeriksa detail foto; geser untuk menggeser tampilan saat diperbesar.'}
        {hasMultiple ? ' Gunakan panah kiri/kanan untuk pindah berkas.' : ''}
      </DialogDescription>

      <div className="pointer-events-none absolute inset-x-0 top-0 z-20 flex items-center justify-between gap-3 bg-linear-to-b from-black/70 to-transparent px-4 py-3 pr-14 text-white">
        <div className="pointer-events-auto min-w-0 truncate text-sm font-medium">
          {item.title}
          {hasMultiple && <span className="ml-2 text-xs font-normal text-white/70">{index + 1}/{items.length}</span>}
        </div>
        {!isVideo && <div className="pointer-events-auto flex shrink-0 items-center gap-1">
          <Button type="button" variant="ghost" size="icon-sm" className="text-white hover:bg-white/15 hover:text-white" aria-label="Perkecil foto" disabled={zoom <= MIN_ZOOM} onClick={() => applyZoom(zoom - ZOOM_STEP)}><Minus aria-hidden="true" /></Button>
          <span className="w-11 text-center text-xs font-medium tabular-nums" aria-label="Tingkat zoom foto">{zoom}%</span>
          <Button type="button" variant="ghost" size="icon-sm" className="text-white hover:bg-white/15 hover:text-white" aria-label="Perbesar foto" disabled={zoom >= MAX_ZOOM} onClick={() => applyZoom(zoom + ZOOM_STEP)}><Plus aria-hidden="true" /></Button>
          {(zoom !== 100 || pan.x !== 0 || pan.y !== 0) && <Button type="button" variant="ghost" size="icon-sm" className="text-white hover:bg-white/15 hover:text-white" aria-label="Reset zoom" onClick={resetZoom}><RotateCcw aria-hidden="true" /></Button>}
        </div>}
      </div>

      <div
        ref={attachGestureNode}
        aria-label="Area preview foto"
        className={cn('relative min-h-0 flex-1 touch-none overflow-hidden select-none', isVideo ? 'cursor-default' : isGesturing ? 'cursor-grabbing' : 'cursor-grab')}
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={endPointer}
        onPointerCancel={endPointer}
        onDoubleClick={handleDoubleClick}
      >
        {hasMultiple && <>
          <Button type="button" variant="ghost" size="icon" className="absolute left-2 top-1/2 z-10 -translate-y-1/2 text-white hover:bg-white/15 hover:text-white" aria-label="Foto sebelumnya" onClick={goPrev}><ChevronLeft aria-hidden="true" /></Button>
          <Button type="button" variant="ghost" size="icon" className="absolute right-2 top-1/2 z-10 -translate-y-1/2 text-white hover:bg-white/15 hover:text-white" aria-label="Foto berikutnya" onClick={goNext}><ChevronRight aria-hidden="true" /></Button>
        </>}
        <div className="flex size-full items-center justify-center">
          {isVideo
            ? <video src={item.url} controls autoPlay preload="metadata" className="max-h-dvh max-w-[100vw] object-contain" />
            : <img
                src={item.url}
                alt={item.title}
                draggable={false}
                onDragStart={(event) => event.preventDefault()}
                className={cn('max-h-dvh max-w-[100vw] object-contain', !isGesturing && 'transition-transform duration-100 motion-reduce:transition-none')}
                style={{ transform: `translate(${pan.x}px, ${pan.y}px) scale(${zoom / 100})` }}
              />}
        </div>
      </div>
    </DialogContent>
  </Dialog>;
}
