import { useEffect, useMemo, useRef, useState } from 'react';
import { InlineSignatureCanvas } from './InlineSignatureCanvas';
import { PlacedMark } from './PlacedMark';
import { base64ToBytes, renderPdfRegion, type RegionMM } from './pdfRegion';
import { markOverflow, signatoryHeadings, type MarkPlacement, type SignatoryKind, type SignatoryRole, type SignatureLayout } from './signatures';

const minPxPerMM = 2.2;

export type EditorMark = { src: string; placement: MarkPlacement };
export type EditorItem = { signature?: EditorMark; stamp?: EditorMark };
export type EditorSelection = { role: SignatoryRole; kind?: SignatoryKind };
/** Kolom yang sedang dicoret tanda tangannya langsung di tabel. */
export type EditorDrawing = { role: SignatoryRole; onDone: (png: string, placement: MarkPlacement) => void; onCancel: () => void };

type Props = {
  layout: SignatureLayout;
  items: Partial<Record<SignatoryRole, EditorItem>>;
  selection: EditorSelection | null;
  editable: boolean;
  zoom: number;
  drawing?: EditorDrawing | null;
  onSelect: (selection: EditorSelection) => void;
  onPlacement: (role: SignatoryRole, kind: SignatoryKind, placement: MarkPlacement) => void;
  onAspect: (role: SignatoryRole, kind: SignatoryKind, aspect: number) => void;
};

/** Region halaman yang ditampilkan: tabel tanda tangan beserta header dan baris nama. */
export function signatureRegion(layout: SignatureLayout): RegionMM {
  const left = Math.max(0, Math.min(...layout.boxes.map((box) => box.x)) - 3);
  const right = Math.min(layout.page_width, Math.max(...layout.boxes.map((box) => box.x + box.w)) + 3);
  const top = Math.max(0, Math.min(...layout.boxes.map((box) => box.y)) - 20);
  const bottom = Math.min(layout.page_height, Math.max(...layout.boxes.map((box) => box.y + box.h)) + 16);
  return { left, top, width: right - left, height: bottom - top };
}

/**
 * Editor tabel tanda tangan: latar adalah halaman tanda tangan BA yang
 * dirender server (tanpa tanda tangan/stempel), dipotong ke tabelnya. Setiap
 * kolom adalah area satu pihak; tanda tangan dan stempel ditaruh pada
 * koordinat yang sama seperti saat dicetak.
 */
export function SignatureTableEditor({ layout, items, selection, editable, zoom, drawing, onSelect, onPlacement, onAspect }: Props) {
  const container = useRef<HTMLDivElement>(null);
  const canvas = useRef<HTMLCanvasElement>(null);
  const [containerWidth, setContainerWidth] = useState(0);
  const [background, setBackground] = useState<'loading' | 'ready' | 'error'>('loading');
  const region = useMemo(() => signatureRegion(layout), [layout]);
  const pdfBytes = useMemo(() => base64ToBytes(layout.pdf), [layout.pdf]);
  // Di layar sempit tabel tidak diperkecil di bawah ~2,2 px/mm agar tetap
  // terbaca; editor bisa digeser ke samping.
  const pxPerMM = Math.max((containerWidth || 720) / region.width, minPxPerMM) * zoom;

  useEffect(() => {
    const host = container.current;
    if (!host) return;
    setContainerWidth(host.clientWidth);
    if (typeof ResizeObserver === 'undefined') return;
    const observer = new ResizeObserver(() => setContainerWidth(host.clientWidth));
    observer.observe(host);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    const target = canvas.current;
    if (!target) return;
    const controller = new AbortController();
    setBackground('loading');
    renderPdfRegion(target, pdfBytes, layout.page, region, pxPerMM, controller.signal)
      .then(() => { if (!controller.signal.aborted) setBackground('ready'); })
      .catch(() => { if (!controller.signal.aborted) setBackground('error'); });
    return () => controller.abort();
  }, [pdfBytes, layout.page, region, pxPerMM]);

  const width = region.width * pxPerMM;
  const height = region.height * pxPerMM;

  return <div ref={container} className="overflow-auto rounded-lg border bg-muted/30">
    <div className="relative mx-auto bg-white" style={{ width, height }}>
      <canvas ref={canvas} aria-hidden="true" className="absolute inset-0 block" style={{ width, height }} />
      {background === 'loading' && <span className="absolute inset-x-0 top-2 text-center text-xs text-muted-foreground">Menyiapkan tabel tanda tangan...</span>}
      {layout.boxes.map((box) => {
        const item = items[box.role] ?? {};
        const boxWidth = box.w * pxPerMM;
        const boxHeight = box.h * pxPerMM;
        const margin = markOverflow * boxHeight;
        const isSelected = selection?.role === box.role;
        const isDrawing = drawing?.role === box.role;
        return <div
          key={box.role}
          role="group"
          aria-label={`Kolom ${signatoryHeadings[box.role]}`}
          data-selected={isSelected}
          className={`absolute ${isSelected ? 'bg-primary/5 ring-2 ring-primary ring-inset' : 'hover:bg-primary/5'}`}
          style={{ left: (box.x - region.left) * pxPerMM, top: (box.y - region.top) * pxPerMM, width: boxWidth, height: boxHeight }}
          onPointerDown={() => onSelect({ role: box.role })}
        >
          {background === 'error' && <span className="absolute inset-0 grid place-items-center border border-dashed px-1 text-center text-[10px] text-muted-foreground">{signatoryHeadings[box.role]}</span>}
          <div className="pointer-events-none absolute overflow-hidden" style={{ inset: -margin }}>
            {(['signature', 'stamp'] as const).map((kind) => {
              const mark = item[kind];
              // Tanda tangan lama disembunyikan selama kolom dicoret ulang.
              if (!mark || (isDrawing && kind === 'signature')) return null;
              return <PlacedMark
                key={kind}
                kind={kind}
                src={mark.src}
                alt={`${kind === 'signature' ? 'Tanda tangan' : 'Stempel'} ${signatoryHeadings[box.role]}`}
                placement={mark.placement}
                boxWidth={boxWidth}
                boxHeight={boxHeight}
                editable={editable && !drawing}
                selected={isSelected && selection?.kind === kind}
                onSelect={() => onSelect({ role: box.role, kind })}
                onChange={(placement) => onPlacement(box.role, kind, placement)}
                onAspect={(aspect) => onAspect(box.role, kind, aspect)}
              />;
            })}
          </div>
          {isDrawing && drawing && <InlineSignatureCanvas
            width={boxWidth}
            height={boxHeight}
            label={`Kanvas tanda tangan ${signatoryHeadings[box.role]}`}
            onDone={drawing.onDone}
            onCancel={drawing.onCancel}
          />}
        </div>;
      })}
    </div>
  </div>;
}
