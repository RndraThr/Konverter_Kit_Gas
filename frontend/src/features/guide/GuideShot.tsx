import { Maximize2 } from 'lucide-react';
import { useState } from 'react';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import shots from './guideShots.json';

type Mark = { n: number; x: number; y: number; w: number; h: number };
type Shot = { src: string; width: number; height: number; marks: Mark[] };

const manifest = shots as Record<string, Shot>;

export type GuideShotSpec = {
  /** Kunci di guideShots.json (dibuat scripts/guide-screenshots.mjs). */
  id: string;
  title: string;
  /** Keterangan tiap nomor; indeks 0 = nomor 1. */
  legend: string[];
};

function Marks({ shot }: { shot: Shot }) {
  return <>
    {shot.marks.map((mark, index) => <span
      key={`${mark.n}-${index}`}
      aria-hidden="true"
      className="pointer-events-none absolute rounded-md border-2 border-primary bg-primary/10 shadow-[0_0_0_3px_rgb(255_255_255/0.7)]"
      style={{ left: `${mark.x}%`, top: `${mark.y}%`, width: `${mark.w}%`, height: `${mark.h}%` }}
    >
      <span className="absolute -top-3 -right-3 grid size-6 place-items-center rounded-full bg-primary text-xs font-bold text-primary-foreground shadow ring-2 ring-white">{mark.n}</span>
    </span>)}
  </>;
}

/**
 * Screenshot halaman asli dengan kotak sorot bernomor. Nomor dijelaskan di
 * bawah gambar; klik gambar untuk memperbesar.
 */
export function GuideShot({ spec }: { spec: GuideShotSpec }) {
  const [open, setOpen] = useState(false);
  const shot = manifest[spec.id];
  if (!shot) return null;
  const alt = `Contoh tampilan: ${spec.title}`;
  const legend = <ol className="grid gap-1.5 text-sm sm:grid-cols-2">
    {spec.legend.map((text, index) => <li key={text} className="flex items-start gap-2">
      <span className="grid size-5 shrink-0 place-items-center rounded-full bg-primary text-[11px] leading-none font-bold text-primary-foreground">{index + 1}</span>
      <span>{text}</span>
    </li>)}
  </ol>;

  return <figure className="grid min-w-0 gap-3">
    <button type="button" onClick={() => setOpen(true)} aria-label={`Perbesar contoh tampilan ${spec.title}`} className="group relative block w-full min-w-0 rounded-lg border bg-muted/30 p-3 text-left focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none">
      {/* Gambar kecil (mis. dialog) tidak diperbesar melebihi ukuran aslinya. */}
      <span className="relative mx-auto block" style={{ maxWidth: shot.width }}>
        <img src={shot.src} alt={alt} width={shot.width} height={shot.height} loading="lazy" className="block h-auto w-full max-w-full rounded-md border bg-background" />
        <Marks shot={shot} />
      </span>
      <span className="absolute right-4 bottom-4 inline-flex items-center gap-1 rounded-md bg-background/90 px-2 py-1 text-xs text-muted-foreground opacity-80 shadow-sm group-hover:opacity-100"><Maximize2 className="size-3.5" aria-hidden="true" />Perbesar</span>
    </button>
    <figcaption className="grid gap-2">
      <span className="text-xs font-medium text-muted-foreground">Contoh tampilan: {spec.title}</span>
      {legend}
    </figcaption>

    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-[min(96vw,1200px)]">
        <DialogHeader>
          <DialogTitle>{spec.title}</DialogTitle>
          <DialogDescription>Nomor pada gambar dijelaskan di bawahnya.</DialogDescription>
        </DialogHeader>
        <div className="relative mx-auto w-full" style={{ maxWidth: shot.width * 1.5 }}>
          <img src={shot.src} alt={alt} width={shot.width} height={shot.height} className="block h-auto w-full rounded-md border" />
          <Marks shot={shot} />
        </div>
        {legend}
      </DialogContent>
    </Dialog>
  </figure>;
}
