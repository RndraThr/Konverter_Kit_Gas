import { Loader2, Maximize, RotateCcw, RotateCw, ZoomIn, ZoomOut } from 'lucide-react';
import { forwardRef, useCallback, useEffect, useImperativeHandle, useRef, useState } from 'react';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

export const minScale = 1;
export const maxScale = 8;
const step = 1.25;

type View = { scale: number; x: number; y: number };
const reset: View = { scale: 1, x: 0, y: 0 };

/**
 * Zoom around a point of the container (cursor or center): the point under it
 * stays where it is. The image is translated, then scaled from its top left.
 */
export function zoomAt(view: View, factor: number, px: number, py: number): View {
  const scale = Math.min(maxScale, Math.max(minScale, view.scale * factor));
  if (scale === minScale) return reset;
  const ratio = scale / view.scale;
  return { scale, x: px - (px - view.x) * ratio, y: py - (py - view.y) * ratio };
}

/**
 * CSS turn of a photo shown with object-contain in box, scaled so the turned
 * photo still fits (a preview until the rotated file is saved).
 */
export function rotationFit(rotation: number, box?: { width: number; height: number }, natural?: { w: number; h: number } | null): string | undefined {
  const deg = ((rotation % 360) + 360) % 360;
  if (deg === 0) return undefined;
  if (deg === 180 || !box || !natural || !box.width || !box.height) return `rotate(${rotation}deg)`;
  const fit = Math.min(box.width / natural.w, box.height / natural.h);
  const w = natural.w * fit, h = natural.h * fit;
  const scale = Math.min(box.width / h, box.height / w);
  return `rotate(${rotation}deg) scale(${scale})`;
}

/** Same file, maybe another version: the URL without its query. */
const sameFile = (a: string, b: string) => !a.startsWith('data:') && a.split('?')[0] === b.split('?')[0];

export type ZoomHandle = { zoomIn: () => void; zoomOut: () => void; reset: () => void };

/**
 * A photo that zooms with the mouse wheel toward the cursor, with buttons
 * (+, −, reset) and drag to pan when zoomed. A new src starts unzoomed.
 */
export const ZoomableImage = forwardRef<ZoomHandle, { src: string; alt: string; className?: string; rotation?: number; onRotate?: (delta: number) => void; rotating?: boolean }>(function ZoomableImage({ src, alt, className, rotation = 0, onRotate, rotating }, ref) {
  const box = useRef<HTMLDivElement>(null);
  const [view, setView] = useState<View>(reset);
  const [panning, setPanning] = useState<{ x: number; y: number } | null>(null);
  const [natural, setNatural] = useState<{ w: number; h: number; box: { width: number; height: number } } | null>(null);
  // What is on screen. A new version of the same file (?v=, e.g. just
  // rotated) replaces it only once loaded, with the turn it came with, so the
  // turned preview never jumps back meanwhile. Another file shows at once.
  const [shown, setShown] = useState({ src, rotation });
  const latest = useRef({ src, rotation });
  useEffect(() => { latest.current = { src, rotation }; }, [src, rotation]);
  const waiting = src !== shown.src && sameFile(src, shown.src);
  if (!waiting && (src !== shown.src || rotation !== shown.rotation)) setShown({ src, rotation });
  const measure = (img: HTMLImageElement) => setNatural({ w: img.naturalWidth, h: img.naturalHeight, box: { width: box.current?.clientWidth ?? 0, height: box.current?.clientHeight ?? 0 } });

  useEffect(() => { setView(reset); }, [shown.src, shown.rotation]);
  const turn = rotationFit(shown.rotation, natural?.box, natural);

  const center = useCallback((factor: number) => {
    const rect = box.current?.getBoundingClientRect();
    setView((v) => zoomAt(v, factor, (rect?.width ?? 0) / 2, (rect?.height ?? 0) / 2));
  }, []);
  useImperativeHandle(ref, () => ({ zoomIn: () => center(step), zoomOut: () => center(1 / step), reset: () => setView(reset) }), [center]);

  // A non-passive listener, so the wheel zooms instead of scrolling the page.
  useEffect(() => {
    const el = box.current;
    if (!el) return;
    const onWheel = (event: WheelEvent) => {
      event.preventDefault();
      const rect = el.getBoundingClientRect();
      // One wheel notch (~100 px; Firefox counts lines) zooms about 1.5×.
      const pixels = event.deltaMode === 1 ? event.deltaY * 33 : event.deltaMode === 2 ? event.deltaY * 400 : event.deltaY;
      const factor = Math.exp(-Math.max(-250, Math.min(250, pixels)) * 0.004);
      setView((v) => zoomAt(v, factor, event.clientX - rect.left, event.clientY - rect.top));
    };
    el.addEventListener('wheel', onWheel, { passive: false });
    return () => el.removeEventListener('wheel', onWheel);
  }, []);

  const zoomed = view.scale > 1;
  return <div ref={box}
    className={cn('relative size-full overflow-hidden', zoomed ? (panning ? 'cursor-grabbing' : 'cursor-grab') : 'cursor-zoom-in', className)}
    onDoubleClick={(e) => {
      const rect = e.currentTarget.getBoundingClientRect();
      setView((v) => (v.scale > 1 ? reset : zoomAt(v, 2.5, e.clientX - rect.left, e.clientY - rect.top)));
    }}
    onPointerDown={(e) => { if (zoomed && e.button === 0) { e.currentTarget.setPointerCapture(e.pointerId); setPanning({ x: e.clientX, y: e.clientY }); } }}
    onPointerMove={(e) => {
      if (!panning) return;
      const dx = e.clientX - panning.x, dy = e.clientY - panning.y;
      setPanning({ x: e.clientX, y: e.clientY });
      setView((v) => ({ ...v, x: v.x + dx, y: v.y + dy }));
    }}
    onPointerUp={() => setPanning(null)}
    onPointerCancel={() => setPanning(null)}
  >
    <div className={cn('size-full', !panning && 'transition-transform duration-100 ease-out')}
      style={{ transform: `translate(${view.x}px, ${view.y}px) scale(${view.scale})`, transformOrigin: '0 0' }}>
      {/* A new file (e.g. just rotated) is a new element: no turn animation on it. */}
      <img key={shown.src} src={shown.src} alt={alt} draggable={false} onLoad={(e) => measure(e.currentTarget)}
        className="size-full select-none object-contain transition-transform duration-200 ease-out"
        style={turn ? { transform: turn } : undefined} />
    </div>
    {waiting ? <img key={src} src={src} alt="" aria-hidden="true" draggable={false} className="pointer-events-none absolute inset-0 size-full object-contain opacity-0"
      onLoad={(e) => { measure(e.currentTarget); setShown(latest.current); }} onError={() => setShown(latest.current)} /> : null}
    <div className="absolute right-3 bottom-3 flex items-center gap-1 rounded-lg bg-black/60 p-1 text-white backdrop-blur" onPointerDown={(e) => e.stopPropagation()} onDoubleClick={(e) => e.stopPropagation()}>
      <Button variant="ghost" size="icon-sm" className="text-white hover:bg-white/20 hover:text-white" aria-label="Perkecil" disabled={!zoomed} onClick={() => center(1 / step)}><ZoomOut /></Button>
      <span className="w-12 text-center text-xs tabular-nums">{Math.round(view.scale * 100)}%</span>
      <Button variant="ghost" size="icon-sm" className="text-white hover:bg-white/20 hover:text-white" aria-label="Perbesar" disabled={view.scale >= maxScale} onClick={() => center(step)}><ZoomIn /></Button>
      <Button variant="ghost" size="icon-sm" className="text-white hover:bg-white/20 hover:text-white" aria-label="Reset zoom" disabled={!zoomed} onClick={() => setView(reset)}><Maximize /></Button>
      {onRotate ? <>
        <span className="mx-0.5 h-5 w-px bg-white/30" aria-hidden="true" />
        <Button variant="ghost" size="icon-sm" className="text-white hover:bg-white/20 hover:text-white" aria-label="Putar ke kiri" title="Putar ke kiri (Shift+R), tersimpan ke file" onClick={() => onRotate(-90)}><RotateCcw /></Button>
        <Button variant="ghost" size="icon-sm" className="text-white hover:bg-white/20 hover:text-white" aria-label="Putar ke kanan" title="Putar ke kanan (R), tersimpan ke file" onClick={() => onRotate(90)}>{rotating ? <Loader2 className="animate-spin" /> : <RotateCw />}</Button>
      </> : null}
    </div>
  </div>;
});
