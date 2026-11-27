import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { lazy, Suspense } from 'react';
import { apiBlobRequest } from '@/lib/api';
import type { BAItem } from './types';

const PdfCanvasPreview = lazy(() => import('@/features/berita-acara/PdfCanvasPreview'));

/**
 * One uploaded BA as the reviewer reads it, right in the page: PDF pages are
 * drawn as images (pdf.js), images are shown as is. BA Perorangan is the
 * recipient's page of the day's BA (its wet-signed scan once uploaded, else
 * the digital BA).
 */
export function BADocument({ item, rotation = 0 }: { item: BAItem; rotation?: number }) {
  const image = item.mime_type.startsWith('image/') && !item.individual;
  const pdf = useQuery({
    queryKey: ['monitoring', 'ba', 'document', item.key, item.uploaded_at, item.content_url],
    queryFn: () => item.individual
      ? apiBlobRequest('/api/v1/bast/individual/bundles/preview', { method: 'POST', body: JSON.stringify({ ...item.individual }) })
      : apiBlobRequest(item.content_url),
    enabled: !image,
    // A rotated PDF: the old pages stay until the new ones are read.
    placeholderData: keepPreviousData,
    staleTime: 5 * 60_000,
  });
  const label = `${item.label} ${item.individual ? `#${item.individual.slot_number}` : item.date}`;
  if (image) {
    return <div className="flex size-full items-center justify-center overflow-auto p-2"><img key={item.content_url} src={item.content_url} alt={label} className="max-h-full max-w-full object-contain transition-transform duration-200" style={rotation ? { transform: `rotate(${rotation}deg)` } : undefined} /></div>;
  }
  if (pdf.isPending) return <p className="m-auto text-sm text-neutral-400">Membuka dokumen…</p>;
  if (pdf.isError) return <p className="m-auto text-sm text-red-400">Dokumen belum dapat dibuka.</p>;
  return <Suspense fallback={<p className="m-auto text-sm text-neutral-400">Menyiapkan penampil…</p>}>
    <PdfCanvasPreview blob={pdf.data} label={label} fit="contain" wrapperClassName="flex size-full min-h-0 flex-col"
      className="grid min-h-0 w-full flex-1 content-start gap-3 overflow-y-auto p-3" />
  </Suspense>;
}
