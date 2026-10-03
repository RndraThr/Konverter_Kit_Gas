import { useEffect, useState } from 'react';
import { DataState } from '@/components/DataState';

type Props = {
  blob?: Blob;
  isPending: boolean;
  isError: boolean;
  label: string;
};

/**
 * Menampilkan pratinjau PDF langsung di halaman (iframe), bukan lewat tombol
 * yang membuka tab baru. Dipakai bersama oleh seluruh panel Berita Acara
 * (BA Perorangan, DP3, Rekap Harian, Closing Titik Serah).
 */
export function PdfPreview({ blob, isPending, isError, label }: Props) {
  const [url, setUrl] = useState('');

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
  if (isError) return <DataState kind="error" title="Preview belum dapat dimuat" description="Lengkapi konfigurasi dokumen, lalu coba lagi." />;
  if (!url) return <DataState kind="empty" title="Preview belum tersedia" description="Lengkapi data agar pratinjau dapat ditampilkan." />;
  return <iframe title={`Preview ${label}`} src={url} className="h-[75vh] w-full rounded-lg border bg-muted/10" />;
}
