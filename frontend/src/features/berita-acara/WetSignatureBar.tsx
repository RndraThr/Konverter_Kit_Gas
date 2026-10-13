import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Eye, FileUp, Printer, Trash2 } from 'lucide-react';
import { useId, useState } from 'react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { apiBlobRequest, ApiError, apiRequest } from '@/lib/api';
import { useCan } from '@/lib/permissions';
import { wetScanFileToPNGBase64, wetSignatureKey, wetSignaturePath, type WetSignatureStatus } from './wetScan';

type DataResponse<T> = { data: T };

export function useWetSignatureStatus(scheduleID: string, document: string, date: string) {
  return useQuery({
    queryKey: wetSignatureKey(scheduleID, document, date),
    queryFn: () => apiRequest<DataResponse<WetSignatureStatus>>(wetSignaturePath(scheduleID, document, date)),
    enabled: scheduleID !== '' && date !== '',
  });
}

/** Finalisasi ditahan bila mode tanda tangan basah aktif tanpa pindaian yang cocok. */
export function useWetSignatureBlocked(scheduleID: string, document: string, date: string) {
  const status = useWetSignatureStatus(scheduleID, document, date).data?.data;
  return Boolean(status?.enabled && !status.scan?.valid);
}

type Props = { scheduleID: string; document: string; date: string; label: string };

/**
 * Langkah tanda tangan basah di panel BA: cetak halaman tanda tangan, lalu
 * unggah hasil pindai/foto halaman yang sudah ditandatangani. Kode dokumen
 * (tampil di sini) mencocokkan pindaian dengan isi dokumen. Tampil hanya bila
 * mode tanda tangan basah aktif.
 */
export function WetSignatureBar({ scheduleID, document, date, label }: Props) {
  const canManage = useCan('bast.manage');
  const queryClient = useQueryClient();
  const inputID = useId();
  const [printing, setPrinting] = useState(false);
  const status = useWetSignatureStatus(scheduleID, document, date);

  const refresh = () => queryClient.invalidateQueries({ predicate: (query) => query.queryKey.includes('preview') });
  const upload = useMutation({
    mutationFn: async (file: File) => {
      if (file.size > 20 << 20) throw new Error('Berkas terlalu besar (maksimal 20 MB sebelum diproses).');
      const image = await wetScanFileToPNGBase64(file);
      return apiRequest<DataResponse<WetSignatureStatus>>(wetSignaturePath(scheduleID, document, date, 'scan'), { method: 'PUT', body: JSON.stringify({ image_png: image }) });
    },
    onSuccess: () => { refresh(); toast.success('Halaman tanda tangan tersimpan dan dipakai di preview.'); },
    onError: (error) => toast.error(errorMessage(error)),
  });
  const remove = useMutation({
    mutationFn: () => apiRequest(wetSignaturePath(scheduleID, document, date, 'scan'), { method: 'DELETE' }),
    onSuccess: () => { refresh(); toast.success('Pindaian dihapus.'); },
    onError: (error) => toast.error(errorMessage(error)),
  });

  const data = status.data?.data;
  if (!data?.enabled) return null;

  // Tab dibuka lebih dulu (masih dalam klik pengguna) agar tidak diblokir
  // pemblokir pop-up, lalu diarahkan ke PDF setelah selesai dibuat.
  const openPDF = async (suffix: string) => {
    const tab = window.open('', '_blank');
    setPrinting(true);
    try {
      const blob = await apiBlobRequest(wetSignaturePath(scheduleID, document, date, suffix));
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
  const viewScan = () => window.open(`${wetSignaturePath(scheduleID, document, date, 'scan')}&v=${encodeURIComponent(data.scan?.uploaded_at ?? '')}`, '_blank', 'noopener');

  const scan = data.scan;
  const busy = upload.isPending || remove.isPending;
  return <div className="grid gap-3 rounded-lg border border-sky-300/60 bg-sky-50/60 p-3 text-sm dark:bg-sky-950/20">
    <div className="flex flex-wrap items-center justify-between gap-2">
      <div className="flex flex-wrap items-center gap-2">
        <strong>Tanda tangan basah</strong>
        {data.code && <Badge variant="outline" className="font-mono">Kode {data.code}</Badge>}
        {!scan && <Badge variant="secondary">Belum diunggah</Badge>}
        {scan?.valid && <Badge className="bg-emerald-600 text-white">Sudah ditandatangani</Badge>}
        {scan && !scan.valid && <Badge variant="destructive">Perlu ditandatangani ulang</Badge>}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <Button type="button" size="sm" variant="outline" disabled={printing} onClick={() => void openPDF('page')}><Printer aria-hidden="true" />{printing ? 'Menyiapkan...' : 'Cetak halaman tanda tangan'}</Button>
        {canManage && <>
          <Button type="button" size="sm" disabled={busy} nativeButton={false} render={<label htmlFor={inputID} />}><FileUp aria-hidden="true" />{upload.isPending ? 'Mengunggah...' : scan ? 'Unggah ulang' : 'Unggah hasil pindai'}</Button>
          <input id={inputID} type="file" accept="application/pdf,image/png,image/jpeg" className="sr-only" disabled={busy} onChange={(event) => { const file = event.target.files?.[0]; event.target.value = ''; if (file) upload.mutate(file); }} />
        </>}
        {scan && <Button type="button" size="sm" variant="ghost" onClick={viewScan}><Eye aria-hidden="true" />Lihat</Button>}
        {scan && canManage && <Button type="button" size="sm" variant="ghost" disabled={busy} onClick={() => remove.mutate()}><Trash2 aria-hidden="true" />Hapus</Button>}
      </div>
    </div>
    <p className="text-xs text-muted-foreground">{scan && !scan.valid
      ? `Isi ${label} berubah setelah halaman dipindai. Cetak ulang halaman tanda tangan, tandatangani, lalu unggah lagi.`
      : scan
        ? `Halaman pindaian menggantikan halaman tanda tangan di preview dan PDF final ${label}.`
        : `Cetak halaman tanda tangan, tandatangani dan stempel basah, lalu unggah hasil pindai (PDF/JPG/PNG atau foto). Finalisasi menunggu halaman ini.`}</p>
  </div>;
}

function errorMessage(error: unknown) {
  return error instanceof ApiError || error instanceof Error ? error.message : 'Permintaan belum dapat diproses.';
}
