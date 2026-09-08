import { useQuery } from '@tanstack/react-query';
import { CheckCircle2, CircleAlert, Download, FileSpreadsheet, FileText } from 'lucide-react';
import { useState } from 'react';
import { DataState } from '@/components/DataState';
import { DataTable } from '@/components/DataTable';
import { PageHeader } from '@/components/PageHeader';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { apiRequest } from '../../lib/api';
import type { DataResponse, ReportRow, ReportSummary, ScheduleResponse } from './types';
import styles from './Reports.module.css';

const ALLOCATION_STATUSES = ['candidate', 'ready', 'needs_review', 'distributed', 'replaced', 'cancelled'];
const DISTRIBUTION_STATUSES = ['draft', 'completed', 'cancelled'];

function statusText(status: string) { return status.replaceAll('_', ' '); }
function ReportStatus({ value, complete }: { value: string; complete?: boolean }) {
  const Icon = complete ? CheckCircle2 : CircleAlert;
  return <Badge variant={complete ? 'default' : 'outline'} className={styles.statusBadge}><Icon aria-hidden="true" />{statusText(value)}</Badge>;
}

export function ReportsPage() {
  const [scheduleID, setScheduleID] = useState('');
  const [allocationStatus, setAllocationStatus] = useState('');
  const [distributionStatus, setDistributionStatus] = useState('');
  const [documentationStatus, setDocumentationStatus] = useState('');
  const schedules = useQuery({ queryKey: ['program-setup', 'schedules'], queryFn: () => apiRequest<ScheduleResponse>('/api/v1/program-setup/schedules') });
  const params = new URLSearchParams({ allocation_status: allocationStatus, distribution_status: distributionStatus, documentation_status: documentationStatus });
  const queryString = params.toString();
  const summary = useQuery({ queryKey: ['reports', 'summary', scheduleID, allocationStatus, distributionStatus, documentationStatus], queryFn: () => apiRequest<DataResponse<ReportSummary>>(`/api/v1/reports/schedule/${encodeURIComponent(scheduleID)}/summary?${queryString}`), enabled: Boolean(scheduleID) });
  const rows = useQuery({ queryKey: ['reports', 'rows', scheduleID, allocationStatus, distributionStatus, documentationStatus], queryFn: () => apiRequest<DataResponse<ReportRow[]>>(`/api/v1/reports/schedule/${encodeURIComponent(scheduleID)}/rows?${queryString}`), enabled: Boolean(scheduleID) });
  const exportBase = `/api/v1/reports/schedule/${encodeURIComponent(scheduleID)}`;

  return <div className={styles.page}>
    <PageHeader title="Laporan" description="Pantau progres alokasi, distribusi, dan dokumentasi per jadwal." />
    <section className={styles.filters} aria-label="Filter laporan">
      <label><span>Jadwal</span><select value={scheduleID} onChange={(event) => setScheduleID(event.target.value)}><option value="">Pilih kabupaten dan jadwal</option>{schedules.data?.data.map((schedule) => <option value={schedule.id} key={schedule.id}>{schedule.regency?.name} / {schedule.name}</option>)}</select></label>
      <label><span>Status alokasi</span><select value={allocationStatus} onChange={(event) => setAllocationStatus(event.target.value)}><option value="">Semua</option>{ALLOCATION_STATUSES.map((status) => <option value={status} key={status}>{statusText(status)}</option>)}</select></label>
      <label><span>Status distribusi</span><select value={distributionStatus} onChange={(event) => setDistributionStatus(event.target.value)}><option value="">Semua</option>{DISTRIBUTION_STATUSES.map((status) => <option value={status} key={status}>{statusText(status)}</option>)}</select></label>
      <label><span>Dokumentasi</span><select value={documentationStatus} onChange={(event) => setDocumentationStatus(event.target.value)}><option value="">Semua</option><option value="complete">Lengkap</option><option value="incomplete">Belum lengkap</option></select></label>
    </section>
    {!scheduleID ? <DataState kind="empty" title="Pilih jadwal untuk melihat laporan" description="Filter dan tautan ekspor akan menyesuaikan jadwal yang dipilih." /> : rows.isError || summary.isError ? <DataState kind="error" title="Laporan belum dapat dimuat" description="Periksa koneksi lalu coba kembali." action={{ label: 'Coba lagi', onClick: () => { void rows.refetch(); void summary.refetch(); } }} /> : <>
      <section className={styles.summary} aria-label="Ringkasan laporan"><Card><CardContent><strong>{summary.data?.data.total_allocations ?? '-'}</strong><span>Total alokasi</span></CardContent></Card>{summary.data?.data.allocation_status_counts.map((item) => <Card key={`allocation-${item.status}`}><CardContent><strong>{item.count}</strong><span>Alokasi {statusText(item.status)}</span></CardContent></Card>)}{summary.data?.data.distribution_status_counts.map((item) => <Card key={`distribution-${item.status}`}><CardContent><strong>{item.count}</strong><span>Distribusi {statusText(item.status)}</span></CardContent></Card>)}<Card><CardContent><strong>{summary.data?.data.documentation_incomplete ?? '-'}</strong><span>Dokumentasi belum lengkap</span></CardContent></Card></section>
      <section className={styles.exportBar} aria-label="Ekspor laporan"><Button render={<a href={`${exportBase}/export.xlsx?${queryString}`} />} variant="outline"><FileSpreadsheet aria-hidden="true" />Export Excel<Download aria-hidden="true" /></Button><Button render={<a href={`${exportBase}/export.pdf?${queryString}`} />} variant="outline"><FileText aria-hidden="true" />Export PDF<Download aria-hidden="true" /></Button></section>
      {rows.data?.data.length ? <DataTable label="Baris laporan distribusi" minimumWidth={1080}><thead><tr><th>No. Pembagian</th><th>Nama</th><th>NIK</th><th>No. Kartu/KUSUKA</th><th>Desa/Kecamatan</th><th>Status alokasi</th><th>Status distribusi</th><th>Dokumentasi</th><th>Tanggal selesai</th></tr></thead><tbody>{rows.data.data.map((row) => <tr key={row.distribution_number}><td className="font-medium tabular-nums">{row.distribution_number}</td><td><strong>{row.full_name}</strong></td><td className="tabular-nums">{row.nik}</td><td>{row.sector_identifier}</td><td>{[row.village, row.district].filter(Boolean).join(', ')}</td><td><ReportStatus value={row.allocation_status} complete={row.allocation_status === 'distributed'} /></td><td><ReportStatus value={row.distribution_status} complete={row.distribution_status === 'completed'} /></td><td><ReportStatus value={row.documentation_complete ? 'Lengkap' : 'Belum lengkap'} complete={row.documentation_complete} /></td><td className="whitespace-nowrap">{row.completed_at ? new Date(row.completed_at).toLocaleString('id-ID') : '-'}</td></tr>)}</tbody></DataTable> : <DataState kind="empty" title="Tidak ada baris laporan" description="Ubah filter untuk menampilkan penerima lain." />}
    </>}
  </div>;
}
