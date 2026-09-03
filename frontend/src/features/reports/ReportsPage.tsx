import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { apiRequest } from '../../lib/api';
import type { DataResponse, ReportRow, ReportSummary, ScheduleResponse } from './types';
import styles from './Reports.module.css';

const ALLOCATION_STATUSES = ['candidate', 'ready', 'needs_review', 'distributed', 'replaced', 'cancelled'];
const DISTRIBUTION_STATUSES = ['draft', 'completed', 'cancelled'];

export function ReportsPage() {
  const [scheduleID, setScheduleID] = useState('');
  const [allocationStatus, setAllocationStatus] = useState('');
  const [distributionStatus, setDistributionStatus] = useState('');
  const [documentationStatus, setDocumentationStatus] = useState('');

  const schedules = useQuery({ queryKey: ['program-setup', 'schedules'], queryFn: () => apiRequest<ScheduleResponse>('/api/v1/program-setup/schedules') });
  const params = new URLSearchParams({ allocation_status: allocationStatus, distribution_status: distributionStatus, documentation_status: documentationStatus });
  const queryString = params.toString();

  const summary = useQuery({
    queryKey: ['reports', 'summary', scheduleID, allocationStatus, distributionStatus, documentationStatus],
    queryFn: () => apiRequest<DataResponse<ReportSummary>>(`/api/v1/reports/schedule/${encodeURIComponent(scheduleID)}/summary?${queryString}`),
    enabled: Boolean(scheduleID),
  });
  const rows = useQuery({
    queryKey: ['reports', 'rows', scheduleID, allocationStatus, distributionStatus, documentationStatus],
    queryFn: () => apiRequest<DataResponse<ReportRow[]>>(`/api/v1/reports/schedule/${encodeURIComponent(scheduleID)}/rows?${queryString}`),
    enabled: Boolean(scheduleID),
  });

  return <div className="page">
    <header className="pageHeader"><div><h1>Laporan</h1><p>Pantau progres alokasi, distribusi, dan dokumentasi per jadwal.</p></div></header>
    <section className={styles.filters}>
      <label><span>Jadwal</span><select value={scheduleID} onChange={(event) => setScheduleID(event.target.value)}>
        <option value="">Pilih kabupaten dan jadwal</option>
        {schedules.data?.data.map((schedule) => <option value={schedule.id} key={schedule.id}>{schedule.regency?.name} / {schedule.name}</option>)}
      </select></label>
      <label><span>Status alokasi</span><select value={allocationStatus} onChange={(event) => setAllocationStatus(event.target.value)}>
        <option value="">Semua</option>
        {ALLOCATION_STATUSES.map((status) => <option value={status} key={status}>{status}</option>)}
      </select></label>
      <label><span>Status distribusi</span><select value={distributionStatus} onChange={(event) => setDistributionStatus(event.target.value)}>
        <option value="">Semua</option>
        {DISTRIBUTION_STATUSES.map((status) => <option value={status} key={status}>{status}</option>)}
      </select></label>
      <label><span>Dokumentasi</span><select value={documentationStatus} onChange={(event) => setDocumentationStatus(event.target.value)}>
        <option value="">Semua</option>
        <option value="complete">Lengkap</option>
        <option value="incomplete">Belum lengkap</option>
      </select></label>
    </section>

    {scheduleID && <>
      <section className={styles.summary}>
        <article><strong>{summary.data?.data.total_allocations ?? '-'}</strong><span>Total alokasi</span></article>
        {summary.data?.data.allocation_status_counts.map((item) => <article key={`allocation-${item.status}`}><strong>{item.count}</strong><span>{item.status}</span></article>)}
        {summary.data?.data.distribution_status_counts.map((item) => <article key={`distribution-${item.status}`}><strong>{item.count}</strong><span>Distribusi {item.status}</span></article>)}
        <article><strong>{summary.data?.data.documentation_incomplete ?? '-'}</strong><span>Dokumentasi belum lengkap</span></article>
      </section>

      <section className={styles.exportBar}>
        <a className={styles.exportButton} href={`/api/v1/reports/schedule/${encodeURIComponent(scheduleID)}/export.xlsx?${queryString}`}>Export Excel</a>
        <a className={styles.exportButton} href={`/api/v1/reports/schedule/${encodeURIComponent(scheduleID)}/export.pdf?${queryString}`}>Export PDF</a>
      </section>

      <table className={styles.table}>
        <thead><tr>
          <th>No. Pembagian</th><th>Nama</th><th>NIK</th><th>No. Kartu/KUSUKA</th><th>Desa/Kecamatan</th>
          <th>Status alokasi</th><th>Status distribusi</th><th>Dokumentasi</th><th>Tanggal selesai</th>
        </tr></thead>
        <tbody>{rows.data?.data.map((row) => <tr key={row.distribution_number}>
          <td>{row.distribution_number}</td>
          <td>{row.full_name}</td>
          <td>{row.nik}</td>
          <td>{row.sector_identifier}</td>
          <td>{[row.village, row.district].filter(Boolean).join(', ')}</td>
          <td>{row.allocation_status}</td>
          <td>{row.distribution_status}</td>
          <td>{row.documentation_complete ? 'Lengkap' : 'Belum lengkap'}</td>
          <td>{row.completed_at ? new Date(row.completed_at).toLocaleString('id-ID') : '-'}</td>
        </tr>)}</tbody>
      </table>
    </>}
  </div>;
}
