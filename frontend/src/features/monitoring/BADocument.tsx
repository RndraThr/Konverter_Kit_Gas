import { ChevronLeft, ChevronRight } from 'lucide-react';
import { forwardRef, useState } from 'react';
import { Button } from '@/components/ui/button';
import { usePdfPages } from './pdfPages';
import type { BAItem } from './types';
import { ZoomableImage, type ZoomHandle } from './ZoomableImage';

type Props = {
  item: BAItem;
  /** Turn not saved yet (degrees), shown on the page meanwhile. */
  rotation?: number;
  /** Rotate buttons in the viewer (null: not allowed). */
  onRotate?: (delta: number) => void;
  rotating?: boolean;
};

/**
 * One uploaded BA or document as the reviewer reads it, right in the page,
 * in the same viewer as photos (wheel zoom, drag, rotate): PDF pages are
 * drawn as large images (pdf.js), one page at a time with page buttons.
 * BA Perorangan is the recipient's page of the day's BA (its wet-signed scan
 * once uploaded, else the digital BA).
 */
export const BADocument = forwardRef<ZoomHandle, Props>(function BADocument({ item, rotation = 0, onRotate, rotating }, ref) {
  const image = item.mime_type.startsWith('image/') && !item.individual;
  const pages = usePdfPages(image ? null : item);
  const [page, setPage] = useState(0);
  const label = `${item.label} ${item.individual ? `#${item.individual.slot_number}` : item.date}`;
  if (image) return <ZoomableImage ref={ref} src={item.content_url} alt={label} rotation={rotation} onRotate={onRotate} rotating={rotating} />;
  if (pages.isPending) return <p className="m-auto text-sm text-neutral-400">Membuka dokumen…</p>;
  if (pages.isError || !pages.data?.length) return <p className="m-auto text-sm text-red-400">Dokumen belum dapat dibuka.</p>;
  const list = pages.data;
  const at = Math.min(page, list.length - 1);
  return <div className="relative size-full">
    <ZoomableImage ref={ref} src={list[at]} alt={`${label} halaman ${at + 1}`} rotation={rotation} onRotate={onRotate} rotating={rotating} />
    {list.length > 1 ? <div className="absolute bottom-3 left-1/2 flex -translate-x-1/2 items-center gap-1 rounded-lg bg-black/60 p-1 text-white backdrop-blur">
      <Button variant="ghost" size="icon-sm" className="text-white hover:bg-white/20 hover:text-white" aria-label="Halaman sebelumnya" disabled={at === 0} onClick={() => setPage(at - 1)}><ChevronLeft /></Button>
      <span className="px-1 text-xs tabular-nums">Hal. {at + 1}/{list.length}</span>
      <Button variant="ghost" size="icon-sm" className="text-white hover:bg-white/20 hover:text-white" aria-label="Halaman berikutnya" disabled={at >= list.length - 1} onClick={() => setPage(at + 1)}><ChevronRight /></Button>
    </div> : null}
  </div>;
});
