import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Download, FileUp, ScanLine, Trash2 } from 'lucide-react';
import { useRef, useState } from 'react';
import { toast } from 'sonner';
import { DataState } from '@/components/DataState';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { ApiError, apiRequest } from '@/lib/api';
import { useCan } from '@/lib/permissions';
import { RakordaScanEditor } from './RakordaScanEditor';
import type { RakordaUpload } from './types';

type Props = { apiPath: string; name: string; scheduleID: string; date: string };
type DataResponse<T> = { data: T };

/** Arsip daftar hadir yang sudah diisi/ditandatangani: unggah berkas atau pindai dari foto, per tanggal. */
export function RakordaArchive({ apiPath, name, scheduleID, date }: Props) {
  const canManage = useCan('bast.manage');
  const queryClient = useQueryClient();
  const uploadInput = useRef<HTMLInputElement>(null);
  const [showScanner, setShowScanner] = useState(false);
  const apiBase = `/api/v1/bast/${apiPath}`;
  const uploads = useQuery({ queryKey: ['bast', apiPath, 'uploads', scheduleID, date], queryFn: () => apiRequest<DataResponse<RakordaUpload[]>>(`${apiBase}/uploads?schedule_id=${encodeURIComponent(scheduleID)}&date=${encodeURIComponent(date)}`), enabled: Boolean(date) });
  const upload = useMutation({
    mutationFn: async (files: File[]) => {
      for (const file of files) {
        const form = new FormData(); form.set('schedule_id', scheduleID); form.set('date', date); form.set('file', file);
        await apiRequest<DataResponse<RakordaUpload>>(`${apiBase}/uploads`, { method: 'POST', body: form });
      }
    },
    onSuccess: (_, files) => { queryClient.invalidateQueries({ queryKey: ['bast', apiPath, 'uploads', scheduleID] }); toast.success(`${files.length} berkas ${name} berhasil diunggah.`); },
    onError: (error) => toast.error(errorMessage(error)),
  });
  const remove = useMutation({
    mutationFn: (id: string) => apiRequest<void>(`${apiBase}/uploads/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['bast', apiPath, 'uploads', scheduleID] }); toast.success('Berkas dihapus.'); },
    onError: (error) => toast.error(errorMessage(error)),
  });

  return <>
    <Card>
      <CardHeader className="border-b"><div className="flex flex-wrap items-center justify-between gap-3"><div><CardTitle className="flex items-center gap-2 text-base"><FileUp className="size-4 text-primary" aria-hidden="true" />Arsip daftar hadir terisi</CardTitle><p className="mt-1 text-sm text-muted-foreground">Unggah PDF, JPG, atau PNG. Beberapa berkas dapat dipilih sekaligus.</p></div>{canManage && <div className="flex gap-2"><input ref={uploadInput} className="sr-only" type="file" multiple accept="application/pdf,image/jpeg,image/png" onChange={(event) => { const files = Array.from(event.target.files ?? []); if (files.length) upload.mutate(files); event.target.value = ''; }} /><Button variant="outline" disabled={!date || upload.isPending} onClick={() => uploadInput.current?.click()}><FileUp aria-hidden="true" />{upload.isPending ? 'Mengunggah...' : 'Unggah berkas'}</Button><Button aria-expanded={showScanner} onClick={() => setShowScanner((value) => !value)}><ScanLine aria-hidden="true" />{showScanner ? 'Tutup pemindai' : 'Pindai dari foto'}</Button></div>}</div></CardHeader>
      <CardContent className="pt-5">
        {uploads.isPending ? <DataState kind="loading" title="Memuat arsip" description={`Mengambil berkas ${name}.`} /> : uploads.isError ? <DataState kind="error" title="Arsip gagal dimuat" description="Coba muat ulang." action={{ label: 'Coba lagi', onClick: () => uploads.refetch() }} /> : (uploads.data?.data.length ?? 0) === 0 ? <DataState kind="empty" title="Belum ada dokumen terisi" description="Setelah daftar hadir ditandatangani, unggah hasil pindai atau fotonya di sini." /> : <div className="grid gap-2">{uploads.data!.data.map((item) => <div key={item.id} className="flex flex-wrap items-center justify-between gap-3 rounded-lg border p-3"><div className="min-w-0"><strong className="block truncate text-sm">{item.original_name}</strong><span className="text-xs text-muted-foreground">{formatBytes(item.byte_size)} · {new Intl.DateTimeFormat('id-ID', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(item.created_at))}</span></div><div className="flex gap-1"><Button nativeButton={false} render={<a href={`${apiBase}/uploads/${item.id}/content`} />} variant="ghost" size="sm"><Download aria-hidden="true" />Unduh</Button>{canManage && <Button variant="ghost" size="icon-sm" aria-label={`Hapus ${item.original_name}`} disabled={remove.isPending} onClick={() => remove.mutate(item.id)}><Trash2 /></Button>}</div></div>)}</div>}
      </CardContent>
    </Card>
    {showScanner && canManage && <RakordaScanEditor disabled={!date || upload.isPending} onPdfReady={async (file) => upload.mutateAsync([file])} />}
  </>;
}

const formatBytes = (size: number) => size >= 1_048_576 ? `${(size / 1_048_576).toFixed(1)} MB` : `${Math.max(1, Math.round(size / 1024))} KB`;
const errorMessage = (error: unknown) => error instanceof ApiError || error instanceof Error ? error.message : 'Permintaan belum dapat diproses.';
