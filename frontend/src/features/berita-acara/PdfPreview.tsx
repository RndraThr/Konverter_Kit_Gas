import { Download } from 'lucide-react';
import { lazy, Suspense, useEffect, useState } from 'react';
import { DataState } from '@/components/DataState';
import { Button } from '@/components/ui/button';

const PdfCanvasPreview = lazy(() => import('./PdfCanvasPreview'));

type Props = {
  blob?: Blob;
  isPending: boolean;
  isError: boolean;
  label: string;
  errorDescription?: string;
  /** Halaman awal yang ditampilkan (fragment #page=N pada viewer PDF browser). */
  page?: number;
};

// Browser tanpa penampil PDF bawaan (Chrome Android) atau layar ponsel memakai
// pdf.js; iframe di ponsel hanya menampilkan tombol "Open" atau halaman pertama.
function needsCanvasPreview() {
  if (typeof window === 'undefined') return false;
  const noViewer = typeof navigator !== 'undefined' && navigator.pdfViewerEnabled === false;
  const narrow = typeof window.matchMedia === 'function' && window.matchMedia('(max-width: 767px)').matches;
  return noViewer || narrow;
}

/**
 * Menampilkan pratinjau PDF langsung di halaman, bukan lewat tombol yang
 * membuka tab baru. Desktop memakai penampil PDF browser (iframe); ponsel
 * memakai pdf.js. Dipakai bersama oleh seluruh panel Berita Acara.
 */
export function PdfPreview({ blob, isPending, isError, label, errorDescription, page }: Props) {
  const [url, setUrl] = useState('');
  const [canvasMode] = useState(needsCanvasPreview);

  useEffect(() => {
    if (!blob) {
      setUrl('');
      return;
    }
    const next = URL.createObjectURL(blob);
    setUrl(next);
    return () => URL.revokeObjectURL(next);
  }, [blob]);

  if (isPending) return <DataState kind="loading" title="Menyiapkan preview" description={`Membuat pratinjau ${label}.`} />;
  if (isError) return <DataState kind="error" title="Preview belum dapat dimuat" description={errorDescription || 'Lengkapi konfigurasi dokumen, lalu coba lagi.'} />;
  if (!url || !blob) return <DataState kind="empty" title="Preview belum tersedia" description="Lengkapi data agar pratinjau dapat ditampilkan." />;
  if (canvasMode) return <div className="grid gap-2">
    <Suspense fallback={<DataState kind="loading" title="Menyiapkan preview" description={`Memuat penampil ${label}.`} />}>
      <PdfCanvasPreview blob={blob} label={label} page={page} />
    </Suspense>
    <Button nativeButton={false} variant="outline" className="justify-self-start" render={<a href={url} download={`${label}.pdf`} />}><Download aria-hidden="true" />Unduh PDF</Button>
  </div>;
  // key memaksa iframe dimuat ulang saat halaman berganti; viewer PDF browser
  // tidak selalu bereaksi pada perubahan fragment saja.
  return <iframe key={page ?? 0} title={`Preview ${label}`} src={page ? `${url}#page=${page}` : url} className="h-[75vh] w-full rounded-lg border bg-muted/10" />;
}
