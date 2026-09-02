import { useQuery } from '@tanstack/react-query';
import { CheckCircle2, RefreshCw, TriangleAlert, XCircle } from 'lucide-react';
import { apiRequest } from '../../lib/api';
import styles from './HealthPage.module.css';

type Health = { status: 'healthy' | 'degraded' | 'unhealthy'; database: { status: string; code?: string; message: string; latency_ms: number }; migration_version: number; environment: string; version: string; uptime_seconds: number; checked_at: string };
const statusText = { healthy: 'Sehat', degraded: 'Perlu perhatian', unhealthy: 'Tidak tersedia' };
function StatusIcon({ status }: { status: Health['status'] }) { return status === 'healthy' ? <CheckCircle2 /> : status === 'degraded' ? <TriangleAlert /> : <XCircle />; }

export function HealthPage() {
  const query = useQuery({ queryKey: ['health'], queryFn: () => apiRequest<{ data: Health }>('/api/v1/system/health'), refetchInterval: 60_000 });
  const health = query.data?.data;
  return <div className="page"><header className="pageHeader"><div><h1>Kesehatan sistem</h1><p>Status dependency yang dibutuhkan aplikasi untuk beroperasi.</p></div><button className="secondaryButton" onClick={() => query.refetch()} disabled={query.isFetching}><RefreshCw /> Periksa ulang</button></header>
    {query.isError ? <div className="errorState">Status sistem belum dapat diperiksa.</div> : health && <>
      <section className={`${styles.overall} ${styles[health.status]}`}><StatusIcon status={health.status} /><div><strong>{statusText[health.status]}</strong><span>Diperiksa {new Date(health.checked_at).toLocaleString('id-ID')}</span></div></section>
      <dl className={styles.details}><div><dt>PostgreSQL</dt><dd>{health.database.message}</dd></div><div><dt>Latensi database</dt><dd>{health.database.latency_ms} ms</dd></div><div><dt>Versi migration</dt><dd>{health.migration_version}</dd></div><div><dt>Lingkungan</dt><dd>{health.environment}</dd></div><div><dt>Versi aplikasi</dt><dd>{health.version}</dd></div><div><dt>Uptime</dt><dd>{Math.floor(health.uptime_seconds / 60)} menit</dd></div></dl>
    </>}
  </div>;
}
