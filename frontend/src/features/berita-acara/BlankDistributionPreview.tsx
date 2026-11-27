import { useQuery } from '@tanstack/react-query';
import { CalendarDays } from 'lucide-react';
import { useState } from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { ApiError, apiBlobRequest } from '@/lib/api';
import { PdfPreview } from './PdfPreview';

type Props = {
  /** Kunci query unik per dokumen, mis. 'dp3'. */
  documentKey: string;
  label: string;
  regencyName: string;
  endpoint: string;
  /** Isi permintaan preview untuk tanggal terpilih. */
  body: (date: string) => Record<string, unknown>;
};

const today = () => new Date().toLocaleDateString('en-CA', { timeZone: 'Asia/Jakarta' });

/**
 * Preview format kosong untuk BA per tanggal distribusi sebelum ada
 * distribusi selesai, agar dokumen bisa diperiksa dan dicetak lebih dulu.
 * Finalisasi baru tersedia setelah penyerahan diselesaikan.
 */
export function BlankDistributionPreview({ documentKey, label, regencyName, endpoint, body }: Props) {
  const [date, setDate] = useState(today);
  const valid = /^\d{4}-\d{2}-\d{2}$/.test(date);
  const preview = useQuery({
    queryKey: ['bast', documentKey, 'blank-preview', endpoint, JSON.stringify(body(date))],
    queryFn: () => apiBlobRequest(endpoint, { method: 'POST', body: JSON.stringify(body(date)) }),
    enabled: valid,
  });
  return <div className="grid gap-5 xl:grid-cols-[19rem_minmax(0,1fr)]">
    <Card className="self-start">
      <CardHeader><CardTitle className="flex items-center gap-2 text-base"><CalendarDays className="size-4 text-primary" />Tanggal dokumen</CardTitle></CardHeader>
      <CardContent className="grid gap-2">
        <Input type="date" aria-label="Tanggal dokumen" value={date} onChange={(e) => setDate(e.target.value)} />
        <p className="text-xs leading-5 text-muted-foreground">Belum ada distribusi selesai di {regencyName}: preview berupa format kosong. Data penerima terisi dan finalisasi tersedia setelah penyerahan diselesaikan.</p>
      </CardContent>
    </Card>
    <Card>
      <CardHeader className="border-b"><CardTitle>{label} · format kosong</CardTitle></CardHeader>
      <CardContent className="pt-5">
        {valid && <PdfPreview blob={preview.data} isPending={preview.isPending} isError={preview.isError} errorDescription={preview.error instanceof ApiError || preview.error instanceof Error ? preview.error.message : undefined} label={label} />}
      </CardContent>
    </Card>
  </div>;
}
