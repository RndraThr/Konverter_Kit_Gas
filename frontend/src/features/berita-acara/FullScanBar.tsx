import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Eye, FileUp, Printer, ScanLine, Trash2 } from 'lucide-react';
import { lazy, Suspense, useId, useState } from 'react';
import { toast } from 'sonner';
import { DataState } from '@/components/DataState';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { apiBlobRequest, ApiError, apiRequest } from '@/lib/api';
import { useCan } from '@/lib/permissions';
import { fullScanDocuments, fullScanKey, fullScanPath, type FullScanStatus } from './fullScan';

type DataResponse<T> = { data: T };

// The camera scanner loads OpenCV.js: only when a scan opens.
const DocumentScanner = lazy(() => import('../activities/DocumentScanner'));

export function useFullScanStatus(scheduleID: string, document: string, date: string) {
  return useQuery({
    queryKey: fullScanKey(scheduleID, document, date),
    queryFn: () => apiRequest<DataResponse<FullScanStatus>>(fullScanPath(scheduleID, document, date)),
    enabled: scheduleID !== '' && date !== '' && fullScanDocuments.has(document.split('@')[0]),
  });
}

type Props = { scheduleID: string; document: string; date: string; label: string };

/**
 * Kartu "Pindai lengkap" sebuah BA, seperti kartu arsip Training: BA lengkap
 * dicetak, ditandatangani dan diparaf di kertas, lalu semua halamannya
 * dipindai dengan kamera atau PDF hasil pindai diunggah. Selama ada
 * pindaian, pindaian itulah preview dan PDF final BA; hapus pindaian untuk
 * kembali ke BA buatan sistem (digital atau tanda tangan basah).
 */
export function FullScanCard({ scheduleID, document, date, label }: Props) {
  const canManage = useCan('bast.manage');
  const queryClient = useQueryClient();
  const inputID = useId();
  const [printing, setPrinting] = useState(false);
  const [scanning, setScanning] = useState(false);
  const status = useFullScanStatus(scheduleID, document, date);

  const refresh = () => queryClient.invalidateQueries({ predicate: (query) => ['preview', 'overview', 'signatures'].some((key) => query.queryKey.includes(key)) });
  const upload = useMutation({
    mutationFn: (form: FormData) => apiRequest<DataResponse<FullScanStatus>>(fullScanPath(scheduleID, document, date, 'scan'), { method: 'PUT', body: form }),
    onSuccess: () => { refresh(); toast.success(`Pindaian ${label} tersimpan dan menjadi PDF final.`); },
    onError: (error) => toast.error(errorMessage(error)),
  });
  const remove = useMutation({
    mutationFn: () => apiRequest(fullScanPath(scheduleID, document, date, 'scan'), { method: 'DELETE' }),
    onSuccess: () => { refresh(); toast.success('Pindaian dihapus.'); },
    onError: (error) => toast.error(errorMessage(error)),
  });

  const data = status.data?.data;
  if (!data?.supported) return null;

  // Tab dibuka lebih dulu (masih dalam klik pengguna) agar tidak diblokir
  // pemblokir pop-up, lalu diarahkan ke PDF setelah selesai dibuat.
  const openPDF = async (suffix: string) => {
    const tab = window.open('', '_blank');
    setPrinting(suffix === 'print');
    try {
      const blob = await apiBlobRequest(fullScanPath(scheduleID, document, date, suffix));
      const url = URL.createObjectURL(blob);
      if (tab) tab.location.href = url;
      else window.location.assign(url);
      window.setTimeout(() => URL.revokeObjectURL(url), 60_000);
    } catch (error) {
      tab?.close();
      toast.error(errorMessage(error));
    } finally {
      setPrinting(false);
    }
  };
  const uploadPDF = (file: File) => {
    const form = new FormData();
    form.append('file', file, file.name);
    upload.mutate(form);
  };

  const scan = data.scan;
  const busy = upload.isPending || remove.isPending;
  const pages = data.page_count ?? 0;
  return <>
    <Card>
      <CardHeader className="border-b">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="min-w-0">
            <CardTitle className="flex items-center gap-2 text-base"><ScanLine className="size-4 text-primary" aria-hidden="true" />Pindai lengkap</CardTitle>
            <p className="mt-1 text-sm text-muted-foreground">Untuk BA yang ditandatangani dan diparaf di kertas: cetak, lalu pindai semua {pages > 0 ? `${pages} ` : ''}halamannya atau unggah PDF-nya. Pindaian menjadi PDF final {label}.</p>
          </div>
          <div className="flex flex-wrap items-center gap-2">
              <Button variant="outline" disabled={printing} onClick={() => void openPDF('print')}><Printer aria-hidden="true" />{printing ? 'Menyiapkan...' : 'Cetak BA'}</Button>
              {canManage && <>
                <input id={inputID} type="file" accept="application/pdf" className="sr-only" disabled={busy} onChange={(event) => { const file = event.target.files?.[0]; event.target.value = ''; if (file) uploadPDF(file); }} />
                <Button variant="outline" disabled={busy} nativeButton={false} render={<label htmlFor={inputID} />}><FileUp aria-hidden="true" />{upload.isPending ? 'Mengunggah...' : 'Unggah PDF'}</Button>
                <Button disabled={busy} onClick={() => setScanning(true)}><ScanLine aria-hidden="true" />{scan ? 'Pindai ulang' : 'Pindai'}</Button>
              </>}
          </div>
        </div>
      </CardHeader>
      <CardContent className="pt-5">
        {!scan
          ? <DataState kind="empty" title="Belum ada pindaian" description={`Tanpa pindaian, PDF final ${label} dibuat sistem: tanda tangan digital, atau halaman tanda tangan basah bila dipilih di Penandatangan.`} />
          : <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border p-3">
            <div className="min-w-0">
              <span className="flex flex-wrap items-center gap-2">
                <strong className="text-sm">Pindaian {label}</strong>
                {scan.valid ? <Badge className="bg-emerald-600 text-white">Dipakai sebagai PDF final</Badge> : <Badge variant="destructive">Perlu dipindai ulang</Badge>}
              </span>
              <span className="text-xs text-muted-foreground">{scan.page_count} halaman · {new Intl.DateTimeFormat('id-ID', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(scan.uploaded_at))}{data.code ? ` · Kode ${data.code}` : ''}</span>
              {!scan.valid && <span className="block text-xs text-destructive">Isi {label} berubah setelah dipindai. Cetak ulang dan pindai lagi, atau hapus pindaian agar BA dibuat sistem.</span>}
            </div>
            <div className="flex gap-1">
              <Button variant="ghost" size="sm" onClick={() => void openPDF('scan')}><Eye aria-hidden="true" />Lihat</Button>
              {canManage && <Button variant="ghost" size="icon-sm" aria-label={`Hapus pindaian ${label}`} disabled={busy} onClick={() => remove.mutate()}><Trash2 /></Button>}
            </div>
          </div>}
      </CardContent>
    </Card>
    {scanning && <Suspense fallback={null}>
      <DocumentScanner
        title={`Pindai ${label}`}
        description={`Pindai semua ${pages > 0 ? `${pages} ` : ''}halaman ${label} yang sudah ditandatangani, berurutan.`}
        maxPages={pages > 0 ? pages : undefined}
        onSubmit={async (blobs) => {
          if (pages > 0 && blobs.length !== pages) throw new ApiError(400, 'full_scan_page_count', `Baru ${blobs.length} dari ${pages} halaman. Pindai semua halaman BA.`);
          const form = new FormData();
          blobs.forEach((blob, index) => form.append('page', blob, `page-${index + 1}.jpg`));
          await upload.mutateAsync(form);
          setScanning(false);
        }}
        onClose={() => setScanning(false)}
      />
    </Suspense>}
  </>;
}

function errorMessage(error: unknown) {
  return error instanceof ApiError || error instanceof Error ? error.message : 'Permintaan belum dapat diproses.';
}
