import { useEffect, useRef, useState } from 'react';
import * as pdfjs from 'pdfjs-dist';
import workerUrl from 'pdfjs-dist/build/pdf.worker.min.mjs?url';
import { DataState } from '@/components/DataState';

pdfjs.GlobalWorkerOptions.workerSrc = workerUrl;

type Props = {
  blob: Blob;
  label: string;
  page?: number;
  className?: string;
  /** Classes of the outer wrapper; give it a size when the pages must fill a box. */
  wrapperClassName?: string;
  /** width: each page as wide as the box. contain: each whole page fits in the box, centred. */
  fit?: 'width' | 'contain';
};

/**
 * Preview PDF untuk browser tanpa penampil PDF bawaan (mis. Chrome Android):
 * setiap halaman digambar ke canvas selebar wadah memakai pdf.js. Dimuat
 * terpisah (lazy) agar bundle desktop tidak bertambah.
 */
export default function PdfCanvasPreview({ blob, label, page, className = 'grid max-h-[75vh] gap-3 overflow-y-auto rounded-lg border bg-muted/30 p-2', wrapperClassName = 'grid gap-2', fit = 'width' }: Props) {
  const container = useRef<HTMLDivElement>(null);
  const [state, setState] = useState<'loading' | 'ready' | 'error'>('loading');

  useEffect(() => {
    let cancelled = false;
    let task: pdfjs.PDFDocumentLoadingTask | undefined;
    const host = container.current;
    if (!host) return;
    host.replaceChildren();
    setState('loading');
    (async () => {
      try {
        task = pdfjs.getDocument({ data: new Uint8Array(await blob.arrayBuffer()) });
        const pdf = await task.promise;
        const style = getComputedStyle(host);
        const width = (host.clientWidth - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight)) || 360;
        const height = host.clientHeight - parseFloat(style.paddingTop) - parseFloat(style.paddingBottom);
        const ratio = Math.min(window.devicePixelRatio || 1, 2);
        for (let number = 1; number <= pdf.numPages && !cancelled; number++) {
          const pdfPage = await pdf.getPage(number);
          const base = pdfPage.getViewport({ scale: 1 });
          const scale = fit === 'contain' && height > 0 ? Math.min(width / base.width, height / base.height) : width / base.width;
          const viewport = pdfPage.getViewport({ scale: scale * ratio });
          const canvas = document.createElement('canvas');
          canvas.width = Math.floor(viewport.width);
          canvas.height = Math.floor(viewport.height);
          canvas.style.width = fit === 'contain' ? `${Math.floor(base.width * scale)}px` : '100%';
          canvas.dataset.page = String(number);
          canvas.setAttribute('aria-label', `${label} halaman ${number}`);
          canvas.className = fit === 'contain' ? 'mx-auto block rounded-md border bg-white shadow-xs' : 'block rounded-md border bg-white shadow-xs';
          host.appendChild(canvas);
          await pdfPage.render({ canvas, viewport }).promise;
        }
        if (!cancelled) setState('ready');
      } catch {
        if (!cancelled) setState('error');
      }
    })();
    return () => { cancelled = true; void task?.destroy(); };
  }, [blob, label, fit]);

  useEffect(() => {
    if (state !== 'ready' || !page) return;
    container.current?.querySelector(`canvas[data-page="${page}"]`)?.scrollIntoView({ block: 'start', behavior: 'smooth' });
  }, [page, state]);

  return <div className={wrapperClassName}>
    {state === 'loading' && <DataState kind="loading" title="Menyiapkan preview" description={`Menggambar halaman ${label}.`} />}
    {state === 'error' && <DataState kind="error" title="Preview belum dapat ditampilkan" description="Unduh PDF untuk melihat dokumen." />}
    <div ref={container} title={`Preview ${label}`} className={className} />
  </div>;
}
