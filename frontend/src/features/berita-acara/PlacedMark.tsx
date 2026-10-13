import { RotateCw } from 'lucide-react';
import { useRef, useState, type KeyboardEvent, type PointerEvent as ReactPointerEvent } from 'react';
import { placementLimits, sizeLimits, type MarkPlacement, type SignatoryKind } from './signatures';

type Props = {
  kind: SignatoryKind;
  src: string;
  alt: string;
  placement: MarkPlacement;
  /** Ukuran kotak tanda tangan (CSS px); posisi memakai satuan tinggi kotak, sama seperti PDF. */
  boxWidth: number;
  boxHeight: number;
  editable: boolean;
  selected: boolean;
  onSelect: () => void;
  onChange: (placement: MarkPlacement) => void;
  /** Rasio lebar/tinggi gambar setelah dimuat. */
  onAspect?: (aspect: number) => void;
};

type Drag = { mode: 'move' | 'resize' | 'rotate'; startX: number; startY: number; start: MarkPlacement; centerX: number; centerY: number };

const clamp = (value: number, low: number, high: number) => Math.max(low, Math.min(high, value));
const round3 = (value: number) => Math.round(value * 1000) / 1000;

/**
 * Tanda tangan atau stempel di atas kotak tanda tangan editor. Ukuran gambar
 * mengikuti aturan cetak: sisi terpanjang = size x tinggi kotak, rasio gambar
 * dipertahankan. Saat dipilih, gambar bisa digeser, diubah ukurannya (handle
 * sudut), diputar (handle atas), atau digeser dengan tombol panah.
 */
export function PlacedMark({ kind, src, alt, placement, boxWidth, boxHeight, editable, selected, onSelect, onChange, onAspect }: Props) {
  const element = useRef<HTMLDivElement>(null);
  const drag = useRef<Drag | null>(null);
  const [aspect, setAspect] = useState(1);
  const side = placement.size * boxHeight;
  const width = aspect >= 1 ? side : side * aspect;
  const height = aspect >= 1 ? side / aspect : side;
  const maxX = Math.min(placementLimits.maxX, boxWidth / 2 / boxHeight);
  const active = editable && selected;

  const begin = (mode: Drag['mode']) => (event: ReactPointerEvent<HTMLElement>) => {
    event.stopPropagation();
    onSelect();
    if (!editable) return;
    event.preventDefault();
    event.currentTarget.setPointerCapture?.(event.pointerId);
    const rect = element.current?.getBoundingClientRect();
    drag.current = {
      mode, startX: event.clientX, startY: event.clientY, start: placement,
      centerX: rect ? rect.left + rect.width / 2 : event.clientX,
      centerY: rect ? rect.top + rect.height / 2 : event.clientY,
    };
  };
  const move = (event: ReactPointerEvent<HTMLElement>) => {
    const current = drag.current;
    if (!current) return;
    const { start } = current;
    if (current.mode === 'move') {
      onChange({
        ...start,
        x: round3(clamp(start.x + (event.clientX - current.startX) / boxHeight, -maxX, maxX)),
        y: round3(clamp(start.y + (event.clientY - current.startY) / boxHeight, -placementLimits.maxY, placementLimits.maxY)),
      });
    } else if (current.mode === 'resize') {
      // Jarak pusat ke sudut = setengah diagonal gambar.
      const distance = Math.hypot(event.clientX - current.centerX, event.clientY - current.centerY);
      const diagonalPerSide = Math.hypot(width, height) / Math.max(width, height, 1);
      onChange({ ...start, size: round3(clamp((2 * distance) / diagonalPerSide / boxHeight, sizeLimits[kind].min, sizeLimits[kind].max)) });
    } else {
      const angle = (Math.atan2(event.clientY - current.centerY, event.clientX - current.centerX) * 180) / Math.PI + 90;
      onChange({ ...start, rotation: Math.round(((angle + 540) % 360) - 180) });
    }
  };
  const end = () => { drag.current = null; };
  const pointer = { onPointerMove: move, onPointerUp: end, onPointerCancel: end };

  const nudge = (event: KeyboardEvent<HTMLDivElement>) => {
    if (!editable) return;
    const step = event.shiftKey ? 0.05 : 0.01;
    const delta: Record<string, [number, number]> = { ArrowLeft: [-step, 0], ArrowRight: [step, 0], ArrowUp: [0, -step], ArrowDown: [0, step] };
    const move = delta[event.key];
    if (!move) return;
    event.preventDefault();
    onChange({
      ...placement,
      x: round3(clamp(placement.x + move[0], -maxX, maxX)),
      y: round3(clamp(placement.y + move[1], -placementLimits.maxY, placementLimits.maxY)),
    });
  };

  return <div
    ref={element}
    role="button"
    tabIndex={0}
    aria-label={alt}
    aria-pressed={selected}
    data-testid={`mark-${kind}`}
    className={`pointer-events-auto absolute touch-none select-none focus-visible:outline-none ${editable ? 'cursor-move' : 'cursor-pointer'} ${active ? 'outline-2 outline-dashed outline-primary' : 'hover:outline-1 hover:outline-dashed hover:outline-primary/50'}`}
    style={{
      left: `calc(50% + ${placement.x * boxHeight}px)`,
      top: `calc(50% + ${placement.y * boxHeight}px)`,
      width,
      height,
      transform: `translate(-50%, -50%) rotate(${placement.rotation}deg)`,
      zIndex: selected ? 3 : kind === 'stamp' ? 2 : 1,
    }}
    onPointerDown={begin('move')}
    onKeyDown={nudge}
    {...pointer}
  >
    <img
      alt=""
      src={src}
      draggable={false}
      onLoad={(event) => {
        const { naturalWidth, naturalHeight } = event.currentTarget;
        if (naturalWidth > 0 && naturalHeight > 0) {
          setAspect(naturalWidth / naturalHeight);
          onAspect?.(naturalWidth / naturalHeight);
        }
      }}
      className={`pointer-events-none size-full object-contain ${kind === 'stamp' ? 'opacity-90 mix-blend-multiply' : ''}`}
    />
    {active && <>
      <span aria-hidden="true" className="absolute -top-5 left-1/2 h-5 w-px -translate-x-1/2 bg-primary" />
      <button
        type="button"
        aria-label={`Putar ${alt}`}
        className="absolute -top-8 left-1/2 grid size-6 -translate-x-1/2 cursor-grab touch-none place-items-center rounded-full border bg-background text-primary shadow"
        onPointerDown={begin('rotate')}
        {...pointer}
      ><RotateCw className="size-3.5" aria-hidden="true" /></button>
      <button
        type="button"
        aria-label={`Ubah ukuran ${alt}`}
        className="absolute -right-2 -bottom-2 size-4 cursor-nwse-resize touch-none rounded-sm border-2 border-primary bg-background shadow"
        onPointerDown={begin('resize')}
        {...pointer}
      />
    </>}
  </div>;
}
