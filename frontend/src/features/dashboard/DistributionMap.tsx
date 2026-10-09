import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Minus, Plus, RotateCcw } from 'lucide-react';
import type { MapRegion } from './types';
import { buildRegencyPaths, colorForValue, metricMax, normalizeRegencyName, type MetricDefinition, type RegencyAsset } from './mapRegions';

const VIEW_WIDTH = 1000;
const VIEW_HEIGHT = 420;
const MIN_SCALE = 1;
const MAX_SCALE = 12;

type View = { scale: number; x: number; y: number };
const RESET_VIEW: View = { scale: 1, x: 0, y: 0 };

type Props = {
  asset: RegencyAsset;
  regions: MapRegion[];
  metric: MetricDefinition;
  selectedKey: string | null;
  onSelect: (key: string | null) => void;
};

export function DistributionMap({ asset, regions, metric, selectedKey, onSelect }: Props) {
  const container = useRef<HTMLDivElement>(null);
  const svg = useRef<SVGSVGElement>(null);
  const [view, setView] = useState<View>(RESET_VIEW);
  const [hovered, setHovered] = useState<{ key: string; x: number; y: number } | null>(null);
  const drag = useRef<{ pointerId: number; startX: number; startY: number; originX: number; originY: number } | null>(null);

  const regionIndex = useMemo(() => {
    const index = new Map<string, MapRegion>();
    for (const region of regions) index.set(normalizeRegencyName(region.regency_name), region);
    return index;
  }, [regions]);
  const max = useMemo(() => metricMax(metric, regions), [metric, regions]);
  const paths = useMemo(() => buildRegencyPaths(asset.regencies, VIEW_WIDTH, VIEW_HEIGHT), [asset]);

  const hoveredRegion = hovered ? regionIndex.get(hovered.key) : undefined;
  const selectedRegion = selectedKey ? regionIndex.get(selectedKey) : undefined;

  // Zooming keeps the point under the cursor fixed, which is what makes wheel zoom feel attached to
  // the map instead of to the frame.
  const zoomAt = useCallback((factor: number, pointerX: number, pointerY: number) => {
    setView((current) => {
      const scale = Math.min(MAX_SCALE, Math.max(MIN_SCALE, current.scale * factor));
      if (scale === current.scale) return current;
      const ratio = scale / current.scale;
      return { scale, x: pointerX - (pointerX - current.x) * ratio, y: pointerY - (pointerY - current.y) * ratio };
    });
  }, []);

  const pointerPosition = (event: React.PointerEvent | React.MouseEvent) => {
    const box = container.current?.getBoundingClientRect();
    if (!box || box.width === 0) return { x: 0, y: 0, viewX: 0, viewY: 0 };
    const x = event.clientX - box.left;
    const y = event.clientY - box.top;
    return { x, y, viewX: (x / box.width) * VIEW_WIDTH, viewY: (y / box.height) * VIEW_HEIGHT };
  };

  // React registers wheel handlers as passive in modern browsers. A native non-passive listener is
  // required so zooming the map does not also scroll the surrounding dashboard.
  useEffect(() => {
    const node = svg.current;
    if (!node) return;
    const handleWheel = (event: WheelEvent) => {
      event.preventDefault();
      const box = container.current?.getBoundingClientRect();
      const viewX = box && box.width > 0 ? ((event.clientX - box.left) / box.width) * VIEW_WIDTH : 0;
      const viewY = box && box.height > 0 ? ((event.clientY - box.top) / box.height) * VIEW_HEIGHT : 0;
      zoomAt(event.deltaY < 0 ? 1.25 : 0.8, viewX, viewY);
    };
    node.addEventListener('wheel', handleWheel, { passive: false });
    return () => node.removeEventListener('wheel', handleWheel);
  }, [zoomAt]);

  const handlePointerDown = (event: React.PointerEvent) => {
    if (view.scale <= MIN_SCALE) return;
    const { viewX, viewY } = pointerPosition(event);
    drag.current = { pointerId: event.pointerId, startX: viewX, startY: viewY, originX: view.x, originY: view.y };
    (event.target as Element).setPointerCapture?.(event.pointerId);
  };

  const handlePointerMove = (event: React.PointerEvent) => {
    const { x, y, viewX, viewY } = pointerPosition(event);
    if (drag.current && drag.current.pointerId === event.pointerId) {
      setView((current) => ({ ...current, x: drag.current!.originX + (viewX - drag.current!.startX), y: drag.current!.originY + (viewY - drag.current!.startY) }));
    }
    setHovered((current) => (current ? { ...current, x, y } : current));
  };

  const endDrag = () => { drag.current = null; };
  const zoomButtons = [
    { label: 'Perbesar peta', icon: Plus, factor: 1.5 },
    { label: 'Perkecil peta', icon: Minus, factor: 1 / 1.5 },
  ];
  const licenseSource = asset.licenseSource
    ? (/^https?:\/\//i.test(asset.licenseSource) ? asset.licenseSource : `https://${asset.licenseSource}`)
    : null;

  return <div className="relative" ref={container}>
    <svg
      ref={svg}
      role="img"
      aria-label={`Peta distribusi berdasarkan ${metric.label}`}
      viewBox={`0 0 ${VIEW_WIDTH} ${VIEW_HEIGHT}`}
      className="h-[360px] w-full touch-none select-none rounded-lg bg-muted/30 sm:h-[420px]"
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      onPointerUp={endDrag}
      onPointerLeave={() => { endDrag(); setHovered(null); }}
      style={{ cursor: view.scale > MIN_SCALE ? (drag.current ? 'grabbing' : 'grab') : 'default' }}
    >
      <g transform={`translate(${view.x} ${view.y}) scale(${view.scale})`}>
        {paths.map((projected) => {
          const region = regionIndex.get(projected.key);
          const isSelected = selectedKey === projected.key;
          return <path
            key={projected.key}
            d={projected.path}
            data-regency={projected.key}
            aria-label={region ? `${region.regency_name}: ${metric.format(region)}` : projected.name}
            fill={colorForValue(region ? metric.value(region) : 0, max, region ? metric.hasData(region) : false)}
            stroke={isSelected ? 'var(--foreground)' : 'var(--background)'}
            strokeWidth={isSelected ? 1.6 / view.scale : 0.5 / view.scale}
            vectorEffect="non-scaling-stroke"
            className="transition-[fill] duration-150"
            onPointerEnter={() => {
              const box = container.current?.getBoundingClientRect();
              setHovered({ key: projected.key, x: box ? (projected.center[0] / VIEW_WIDTH) * box.width : 0, y: box ? (projected.center[1] / VIEW_HEIGHT) * box.height : 0 });
            }}
            onPointerLeave={() => setHovered((current) => (current?.key === projected.key ? null : current))}
            onClick={() => onSelect(region ? projected.key : null)}
          />;
        })}
      </g>
    </svg>

    <div className="absolute right-3 top-3 flex flex-col gap-1">
      {zoomButtons.map((button) => {
        const Icon = button.icon;
        return <button
          key={button.label}
          type="button"
          aria-label={button.label}
          title={button.label}
          className="grid size-11 place-items-center rounded-md border bg-background/95 shadow-sm hover:bg-muted focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
          onClick={() => zoomAt(button.factor, VIEW_WIDTH / 2, VIEW_HEIGHT / 2)}
        ><Icon className="size-4" aria-hidden="true" /></button>;
      })}
      <button
        type="button"
        aria-label="Reset tampilan peta"
        title="Reset tampilan peta"
        className="grid size-11 place-items-center rounded-md border bg-background/95 shadow-sm hover:bg-muted focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
        onClick={() => setView(RESET_VIEW)}
      ><RotateCcw className="size-4" aria-hidden="true" /></button>
    </div>

    {hoveredRegion && hovered && <div
      role="status"
      className="pointer-events-none absolute z-20 max-w-64 -translate-x-1/2 -translate-y-full rounded-md border bg-popover px-3 py-2 text-xs shadow-md"
      style={{ left: hovered.x, top: Math.max(hovered.y - 6, 4) }}
    >
      <p className="font-medium">{hoveredRegion.regency_name}</p>
      <p className="text-muted-foreground">{hoveredRegion.province_name}</p>
      <p className="mt-1">{metric.format(hoveredRegion)}</p>
    </div>}

    <div className="absolute bottom-2 left-3 max-w-[calc(100%-1.5rem)] space-y-0.5 rounded bg-background/90 px-2 py-1 text-[10px] leading-tight text-muted-foreground shadow-sm backdrop-blur-sm">
      <p>{licenseSource
        ? <a className="underline underline-offset-2 hover:text-foreground" href={licenseSource} target="_blank" rel="noreferrer">Batas wilayah: {asset.source} · {asset.license}</a>
        : <>Batas wilayah: {asset.source} · {asset.license}</>}</p>
      <p>{selectedRegion ? `Terpilih: ${selectedRegion.regency_name}` : 'Klik kabupaten untuk melihat rincian'}</p>
    </div>
  </div>;
}
