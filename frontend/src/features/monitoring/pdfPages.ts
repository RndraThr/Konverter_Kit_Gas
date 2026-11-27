import { keepPreviousData, useQuery, type QueryClient } from '@tanstack/react-query';
import { apiBlobRequest } from '@/lib/api';
import type { BAItem } from './types';

/** Longest side of a rendered page, in pixels: sharp when zoomed in. */
const pageSide = 2400;

type PdfSource = Pick<BAItem, 'content_url' | 'individual'>;

const pdfPagesKey = (source: PdfSource) => ['pdf-pages', source.content_url, source.individual ? JSON.stringify(source.individual) : ''] as const;

/** Every page of a PDF as a JPEG data URL (pdf.js, loaded only when needed). */
async function renderPages(blob: Blob): Promise<string[]> {
  const pdfjs = await import('pdfjs-dist');
  const worker = await import('pdfjs-dist/build/pdf.worker.min.mjs?url');
  pdfjs.GlobalWorkerOptions.workerSrc = worker.default;
  const task = pdfjs.getDocument({ data: new Uint8Array(await blob.arrayBuffer()) });
  try {
    const pdf = await task.promise;
    const pages: string[] = [];
    for (let number = 1; number <= pdf.numPages; number++) {
      const page = await pdf.getPage(number);
      const base = page.getViewport({ scale: 1 });
      const viewport = page.getViewport({ scale: pageSide / Math.max(base.width, base.height) });
      const canvas = document.createElement('canvas');
      canvas.width = Math.floor(viewport.width);
      canvas.height = Math.floor(viewport.height);
      const context = canvas.getContext('2d');
      if (context) { context.fillStyle = '#fff'; context.fillRect(0, 0, canvas.width, canvas.height); }
      await page.render({ canvas, viewport }).promise;
      pages.push(canvas.toDataURL('image/jpeg', 0.9));
    }
    return pages;
  } finally {
    void task.destroy();
  }
}

const loadPages = (source: PdfSource) => () => (source.individual
  ? apiBlobRequest('/api/v1/bast/individual/bundles/preview', { method: 'POST', body: JSON.stringify({ ...source.individual }) })
  : apiBlobRequest(source.content_url)).then(renderPages);

/** The pages of a PDF to show in the zoomable viewer; null source: nothing to read. */
export function usePdfPages(source: PdfSource | null) {
  return useQuery({
    queryKey: pdfPagesKey(source ?? { content_url: '' }),
    queryFn: loadPages(source ?? { content_url: '' }),
    enabled: source !== null,
    staleTime: 5 * 60_000,
    // A rotated PDF: the old pages stay until the new ones are drawn.
    placeholderData: keepPreviousData,
  });
}

/** Draws a PDF ahead (e.g. just rotated) so showing it is instant. */
export const prefetchPdfPages = (queryClient: QueryClient, contentURL: string) =>
  queryClient.prefetchQuery({ queryKey: pdfPagesKey({ content_url: contentURL }), queryFn: loadPages({ content_url: contentURL }), staleTime: 5 * 60_000 });
