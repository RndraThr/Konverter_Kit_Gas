import { useQuery } from '@tanstack/react-query';
import { CheckCircle2, Database, Gauge, RefreshCw, TriangleAlert, XCircle } from 'lucide-react';
import { DataState } from '../../components/DataState';
import { PageHeader } from '../../components/PageHeader';
import { Alert, AlertDescription, AlertTitle } from '../../components/ui/alert';
import { Button } from '../../components/ui/button';
import { apiRequest } from '../../lib/api';
import styles from './HealthPage.module.css';

type Health = { status: 'healthy' | 'degraded' | 'unhealthy'; database: { status: string; code?: string; message: string; latency_ms: number }; migration_version: number; environment: string; version: string; uptime_seconds: number; checked_at: string };
const statusText = { healthy: 'Sehat', degraded: 'Perlu perhatian', unhealthy: 'Tidak tersedia' };
function StatusIcon({ status }: { status: Health['status'] }) { return status === 'healthy' ? <CheckCircle2 aria-hidden="true" /> : status === 'degraded' ? <TriangleAlert aria-hidden="true" /> : <XCircle aria-hidden="true" />; }

export function HealthPage() {
  const query = useQuery({ queryKey: ['health'], queryFn: () => apiRequest<{ data: Health }>('/api/v1/system/health', { acceptedStatuses: [503] }), refetchInterval: 60_000 });
  const health = query.data?.data;
  return <div className="space-y-5"><PageHeader title="Kesehatan sistem" description="Status dependency yang dibutuhkan aplikasi untuk beroperasi." actions={<Button variant="outline" onClick={() => query.refetch()} disabled={query.isFetching}><RefreshCw className={query.isFetching ? 'animate-spin' : undefined} />Periksa ulang</Button>} />
    {query.isError ? <DataState kind="error" title="Status sistem belum dapat diperiksa" description="Coba periksa ulang dalam beberapa saat." action={{ label: 'Periksa ulang', onClick: () => query.refetch() }} /> : query.isPending ? <DataState kind="loading" title="Memeriksa kesehatan sistem" description="Menghubungkan status dependency." /> : health ? <>
      <Alert className={`${styles.overall} ${styles[health.status]}`}><StatusIcon status={health.status} /><AlertTitle>{statusText[health.status]}</AlertTitle><AlertDescription>Diperiksa {new Date(health.checked_at).toLocaleString('id-ID')}</AlertDescription></Alert>
      <section className={styles.surfaces} aria-label="Ringkasan dependency"><article><Database aria-hidden="true" /><span>Database</span><strong>{health.database.status}</strong><small>Status koneksi PostgreSQL</small></article><article><Gauge aria-hidden="true" /><span>Latensi</span><strong>{health.database.latency_ms} ms</strong><small>Respons PostgreSQL</small></article><article><CheckCircle2 aria-hidden="true" /><span>Migration</span><strong>v{health.migration_version}</strong><small>{health.environment} · {health.version}</small></article></section>
      <dl className={styles.details}><div><dt>PostgreSQL</dt><dd>{health.database.message}</dd></div><div><dt>Latensi database</dt><dd>{health.database.latency_ms} ms</dd></div><div><dt>Versi migration</dt><dd>{health.migration_version}</dd></div><div><dt>Lingkungan</dt><dd>{health.environment}</dd></div><div><dt>Versi aplikasi</dt><dd>{health.version}</dd></div><div><dt>Uptime</dt><dd>{Math.floor(health.uptime_seconds / 60)} menit</dd></div></dl>
    </> : null}
  </div>;
}
