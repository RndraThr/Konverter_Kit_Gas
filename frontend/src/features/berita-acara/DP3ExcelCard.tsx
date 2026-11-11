import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Download, FileSpreadsheet, RefreshCw } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { ApiError, apiRequest } from '@/lib/api';
import { useCan } from '@/lib/permissions';
import type { DP3ExcelStatus } from './types';

type Props = { scheduleID: string; regencyName: string };
type DataResponse<T> = { data: T };

const formatSyncedAt = (value: string) => new Intl.DateTimeFormat('id-ID', { day: 'numeric', month: 'long', year: 'numeric', hour: '2-digit', minute: '2-digit', timeZone: 'Asia/Jakarta' }).format(new Date(value));

// Excel DP3: semua penerima jadwal kabupaten dalam satu file, hanya data
// penerima tanpa status. Server membangunnya ulang berkala dan menyimpannya
// di folder DP3 kabupaten di Drive; kartu ini menampilkan file terakhir.
export function DP3ExcelCard({ scheduleID, regencyName }: Props) {
  const canManage = useCan('bast.manage');
  const queryClient = useQueryClient();
  const queryKey = ['bast', 'dp3', 'excel', scheduleID];
  const status = useQuery({
    queryKey,
    queryFn: () => apiRequest<DataResponse<DP3ExcelStatus>>(`/api/v1/bast/dp3/excel?schedule_id=${encodeURIComponent(scheduleID)}`),
  });
  const sync = useMutation({
    mutationFn: () => apiRequest<DataResponse<DP3ExcelStatus>>('/api/v1/bast/dp3/excel/sync', { method: 'POST', body: JSON.stringify({ schedule_id: scheduleID }) }),
    onSuccess: (result) => {
      queryClient.setQueryData(queryKey, result);
      toast.success('Excel DP3 sudah diperbarui.');
    },
    onError: (error) => toast.error(error instanceof ApiError || error instanceof Error ? error.message : 'Excel DP3 belum dapat diperbarui.'),
  });
  const file = status.data?.data;

  let detail = 'Dibuat otomatis setelah ada distribusi selesai.';
  if (status.isPending) detail = 'Memuat status file...';
  else if (status.isError) detail = 'Status file belum dapat dimuat.';
  else if (file?.available && file.synced_at) detail = `${file.recipient_count} penerima · diperbarui ${formatSyncedAt(file.synced_at)} WIB`;

  return <Card>
    <CardContent className="grid gap-3 sm:grid-cols-[1fr_auto] sm:items-center">
      <div className="flex gap-3">
        <FileSpreadsheet className="mt-0.5 size-5 shrink-0 text-emerald-600" aria-hidden="true" />
        <div className="grid gap-1">
          <strong className="text-sm">Excel data penerima</strong>
          <p className="text-xs text-muted-foreground">Semua penerima {regencyName} dalam satu file. Diperbarui otomatis tiap 10 menit dan disimpan di folder DP3 di Drive.</p>
          <p className="text-xs font-medium">{detail}</p>
          {file?.last_error && <p className="text-xs text-amber-700 dark:text-amber-300">Pembaruan terakhir gagal. Sistem mencoba lagi otomatis.</p>}
        </div>
      </div>
      <div className="flex flex-wrap gap-2">
        {canManage && <Button disabled={sync.isPending} onClick={() => sync.mutate()} variant="ghost"><RefreshCw className={sync.isPending ? 'animate-spin' : ''} />{sync.isPending ? 'Memperbarui...' : 'Perbarui sekarang'}</Button>}
        {file?.available && <Button nativeButton={false} render={<a href={`/api/v1/bast/dp3/excel/content?schedule_id=${encodeURIComponent(scheduleID)}`} />} variant="outline"><Download />Unduh Excel</Button>}
      </div>
    </CardContent>
  </Card>;
}
