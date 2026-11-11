import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { CloudUpload, Download, FileSpreadsheet, FileText, RefreshCw } from 'lucide-react';
import { useState } from 'react';
import { toast } from 'sonner';
import { DataState } from '@/components/DataState';
import { DataTable } from '@/components/DataTable';
import { PageHeader } from '@/components/PageHeader';
import { Button, buttonVariants } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useCan } from '@/lib/permissions';
import { ApiError, apiRequest } from '../../lib/api';
import type { DataResponse, DriveStatus, Report, ScheduleResponse } from './types';
import styles from './Reports.module.css';

const formatDate = (value: string) => value ? new Intl.DateTimeFormat('id-ID', { day: '2-digit', month: 'short', year: 'numeric', timeZone: 'UTC' }).format(new Date(`${value}T00:00:00Z`)) : '-';
const formatSyncedAt = (value: string) => new Intl.DateTimeFormat('id-ID', { day: 'numeric', month: 'long', year: 'numeric', hour: '2-digit', minute: '2-digit', timeZone: 'Asia/Jakarta' }).format(new Date(value));
const distinct = (values: string[]) => new Set(values.map((value) => value.trim()).filter(Boolean)).size;

// Laporan DP3: penerima yang distribusinya sudah selesai pada satu jadwal
// kabupaten, hanya data penerima tanpa status. Excel-nya juga disimpan
// server secara berkala langsung di folder kabupaten di Drive.
export function ReportsPage() {
  const [scheduleID, setScheduleID] = useState('');
  const schedules = useQuery({ queryKey: ['program-setup', 'schedules'], queryFn: () => apiRequest<ScheduleResponse>('/api/v1/program-setup/schedules') });
  const base = `/api/v1/reports/schedule/${encodeURIComponent(scheduleID)}`;
  const report = useQuery({ queryKey: ['reports', 'dp3', scheduleID], queryFn: () => apiRequest<DataResponse<Report>>(`${base}/rows`), enabled: Boolean(scheduleID) });
  const data = report.data?.data;
  const rows = data?.rows ?? [];
  const cardTitle = data?.program_type === 'fisherman' ? 'No. KUSUKA' : 'No. Kartu Tani';

  return <div className={styles.page}>
    <PageHeader title="Laporan DP3" description="Daftar penerima yang distribusinya sudah selesai per kabupaten. Unduh sebagai Excel atau PDF; Excel juga tersimpan otomatis di folder kabupaten di Drive." />
    <section className={styles.filters} aria-label="Filter laporan">
      <div className="grid min-w-0 gap-2">
        <Label id="report-schedule-label">Jadwal</Label>
        <Select value={scheduleID} onValueChange={(value) => setScheduleID(value ?? '')}>
          <SelectTrigger className="w-full min-w-0" aria-labelledby="report-schedule-label"><SelectValue placeholder="Pilih kabupaten dan jadwal" /></SelectTrigger>
          <SelectContent>{schedules.data?.data.map((schedule) => <SelectItem key={schedule.id} value={schedule.id}>{schedule.regency?.name} / {schedule.name}</SelectItem>)}</SelectContent>
        </Select>
      </div>
    </section>
    {!scheduleID ? <DataState kind="empty" title="Pilih jadwal untuk melihat Laporan DP3" description="Laporan berisi penerima yang distribusinya sudah selesai di kabupaten tersebut." />
      : report.isPending ? <DataState kind="loading" title="Memuat Laporan DP3" description="Mengurutkan penerima sesuai nomor DP3." />
      : report.isError ? <DataState kind="error" title="Laporan belum dapat dimuat" description="Periksa koneksi lalu coba kembali." action={{ label: 'Coba lagi', onClick: () => void report.refetch() }} />
      : <>
        <section className={styles.summary} aria-label="Ringkasan laporan">
          <Card><CardContent><strong>{rows.length}</strong><span>Penerima selesai</span></CardContent></Card>
          <Card><CardContent><strong>{distinct(rows.map((row) => row.distribution_date))}</strong><span>Hari distribusi</span></CardContent></Card>
          <Card><CardContent><strong>{distinct(rows.map((row) => row.handover_location))}</strong><span>Titik serah</span></CardContent></Card>
          <Card><CardContent><strong>{distinct(rows.map((row) => `${row.village}|${row.district}`.replace(/^\|$/, '')))}</strong><span>Desa/kelurahan</span></CardContent></Card>
        </section>
        <div className={styles.toolbar}>
          <section className={styles.exportBar} aria-label="Ekspor laporan">
            <a className={buttonVariants({ variant: 'outline' })} href={`${base}/export.xlsx`}><FileSpreadsheet aria-hidden="true" />Export Excel<Download aria-hidden="true" /></a>
            <a className={buttonVariants({ variant: 'outline' })} href={`${base}/export.pdf`}><FileText aria-hidden="true" />Export PDF<Download aria-hidden="true" /></a>
          </section>
          <DriveCard scheduleID={scheduleID} regencyName={data?.regency_name ?? ''} />
        </div>
        {rows.length ? <DataTable label="Penerima Laporan DP3" minimumWidth={1280}>
          <thead><tr><th>No</th><th>No. Bagi</th><th>Nama Penerima</th><th>NIK</th><th>{cardTitle}</th><th>No. HP</th><th>Desa / Kecamatan</th><th>Tanggal</th><th>Titik Serah</th><th>Mesin</th><th>No. Seri Mesin</th></tr></thead>
          <tbody>{rows.map((row) => <tr key={`${row.row_number}-${row.distribution_number}`}>
            <td className="font-semibold tabular-nums">{row.row_number}</td>
            <td className="tabular-nums text-muted-foreground">{row.distribution_number || '-'}</td>
            <td><strong>{row.full_name}</strong>{row.address && <small className="subline">{row.address}</small>}</td>
            <td className="font-mono text-xs">{row.nik || '-'}</td>
            <td className="font-mono text-xs">{row.card_number || '-'}</td>
            <td className="tabular-nums">{row.phone_number || '-'}</td>
            <td>{[row.village, row.district].filter(Boolean).join(', ') || '-'}</td>
            <td className="whitespace-nowrap">{formatDate(row.distribution_date)}</td>
            <td>{row.handover_location || '-'}</td>
            <td>{[row.machine_brand, row.machine_type].filter(Boolean).join(' ') || '-'}</td>
            <td className="font-mono text-xs">{row.machine_serial || '-'}</td>
          </tr>)}</tbody>
        </DataTable> : <DataState kind="empty" title="Belum ada penerima yang selesai" description="Penerima muncul di Laporan DP3 setelah distribusinya diselesaikan di POS Penyerahan." />}
      </>}
  </div>;
}

function DriveCard({ scheduleID, regencyName }: { scheduleID: string; regencyName: string }) {
  const canManage = useCan('distribution.manage');
  const queryClient = useQueryClient();
  const queryKey = ['reports', 'dp3', 'drive', scheduleID];
  const status = useQuery({
    queryKey,
    queryFn: () => apiRequest<DataResponse<DriveStatus>>(`/api/v1/reports/schedule/${encodeURIComponent(scheduleID)}/drive`),
  });
  const sync = useMutation({
    mutationFn: () => apiRequest<DataResponse<DriveStatus>>(`/api/v1/reports/schedule/${encodeURIComponent(scheduleID)}/drive/sync`, { method: 'POST', body: '{}' }),
    onSuccess: (result) => {
      queryClient.setQueryData(queryKey, result);
      toast.success('Laporan DP3 di Drive sudah diperbarui.');
    },
    onError: (error) => toast.error(error instanceof ApiError || error instanceof Error ? error.message : 'Laporan DP3 di Drive belum dapat diperbarui.'),
  });
  const file = status.data?.data;

  let detail = 'Dibuat otomatis setelah ada distribusi selesai.';
  if (status.isPending) detail = 'Memuat status file...';
  else if (status.isError) detail = 'Status file belum dapat dimuat.';
  else if (file?.available && file.synced_at) detail = `${file.recipient_count} penerima · diperbarui ${formatSyncedAt(file.synced_at)} WIB`;

  return <Card className={styles.drive} aria-label="File Drive">
    <CardContent className="flex flex-wrap items-center justify-between gap-3">
      <div className="flex min-w-0 gap-3">
        <CloudUpload className="mt-0.5 size-5 shrink-0 text-primary" aria-hidden="true" />
        <div className="grid min-w-0 gap-0.5">
          <strong className="text-sm">Excel di Drive</strong>
          <p className="text-xs text-muted-foreground">Folder kabupaten {regencyName}, diperbarui otomatis tiap 10 menit.</p>
          <p className="text-xs font-medium">{detail}</p>
          {file?.last_error && <p className="text-xs text-amber-700 dark:text-amber-300">Pembaruan terakhir gagal. Sistem mencoba lagi otomatis.</p>}
        </div>
      </div>
      {canManage && <Button disabled={sync.isPending} onClick={() => sync.mutate()} variant="ghost"><RefreshCw className={sync.isPending ? 'animate-spin' : ''} />{sync.isPending ? 'Memperbarui...' : 'Perbarui sekarang'}</Button>}
    </CardContent>
  </Card>;
}
