import { Maximize, ZoomIn, ZoomOut } from 'lucide-react';
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

export type ZoomHandle = { zoomIn: () => void; zoomOut: () => void; reset: () => void };

/**
 * A photo that zooms with the mouse wheel toward the cursor, with buttons
 * (+, −, reset) and drag to pan when zoomed. A new src starts unzoomed.
 */
export const ZoomableImage = forwardRef<ZoomHandle, { src: string; alt: string; className?: string }>(function ZoomableImage({ src, alt, className }, ref) {
  const box = useRef<HTMLDivElement>(null);
  const [view, setView] = useState<View>(reset);
  const [panning, setPanning] = useState<{ x: number; y: number } | null>(null);

  useEffect(() => { setView(reset); }, [src]);

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
      const factor = Math.exp(-event.deltaY * 0.0015);
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
    <img src={src} alt={alt} draggable={false}
      className={cn('size-full select-none object-contain', !panning && 'transition-transform duration-100 ease-out')}
      style={{ transform: `translate(${view.x}px, ${view.y}px) scale(${view.scale})`, transformOrigin: '0 0' }} />
    <div className="absolute right-3 bottom-3 flex items-center gap-1 rounded-lg bg-black/60 p-1 text-white backdrop-blur" onPointerDown={(e) => e.stopPropagation()} onDoubleClick={(e) => e.stopPropagation()}>
      <Button variant="ghost" size="icon-sm" className="text-white hover:bg-white/20 hover:text-white" aria-label="Perkecil" disabled={!zoomed} onClick={() => center(1 / step)}><ZoomOut /></Button>
      <span className="w-12 text-center text-xs tabular-nums">{Math.round(view.scale * 100)}%</span>
      <Button variant="ghost" size="icon-sm" className="text-white hover:bg-white/20 hover:text-white" aria-label="Perbesar" disabled={view.scale >= maxScale} onClick={() => center(step)}><ZoomIn /></Button>
      <Button variant="ghost" size="icon-sm" className="text-white hover:bg-white/20 hover:text-white" aria-label="Reset zoom" disabled={!zoomed} onClick={() => setView(reset)}><Maximize /></Button>
    </div>
  </div>;
});
