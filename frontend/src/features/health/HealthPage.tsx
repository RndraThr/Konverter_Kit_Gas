import { useQuery } from '@tanstack/react-query';
import {
  CheckCircle2,
  Clock3,
  Database,
  Gauge,
  HardDrive,
  RefreshCw,
  RotateCcw,
  TriangleAlert,
  UploadCloud,
  XCircle,
} from 'lucide-react';
import { DataState } from '../../components/DataState';
import { PageHeader } from '../../components/PageHeader';
import { Alert, AlertDescription, AlertTitle } from '../../components/ui/alert';
import { Button } from '../../components/ui/button';
import { apiRequest } from '../../lib/api';
import styles from './HealthPage.module.css';

type ComponentHealth = { status: string; code?: string; message: string; latency_ms: number };
type Health = {
  status: 'healthy' | 'degraded' | 'unhealthy';
  database: ComponentHealth;
  operations: ComponentHealth;
  migration_version: number;
  environment: string;
  version: string;
  storage_backend: 'local' | 'gdrive' | string;
  media_worker_status: 'active' | 'not_applicable' | string;
  media_moves: { queued: number; processing: number; retry: number; failed: number };
  uptime_seconds: number;
  checked_at: string;
};

const statusText = {
  healthy: { title: 'Sistem sehat', description: 'Seluruh layanan utama siap digunakan.' },
  degraded: { title: 'Perlu perhatian', description: 'Aplikasi dapat digunakan, tetapi ada komponen yang perlu diperiksa.' },
  unhealthy: { title: 'Sistem tidak tersedia', description: 'Layanan utama belum dapat digunakan dengan normal.' },
};

function StatusIcon({ status }: { status: Health['status'] }) {
  return status === 'healthy'
    ? <CheckCircle2 aria-hidden="true" />
    : status === 'degraded'
      ? <TriangleAlert aria-hidden="true" />
      : <XCircle aria-hidden="true" />;
}

function formatUptime(seconds: number) {
  const days = Math.floor(seconds / 86_400);
  const hours = Math.floor((seconds % 86_400) / 3_600);
  const minutes = Math.floor((seconds % 3_600) / 60);
  if (days > 0) return `${days} hari ${hours} jam`;
  if (hours > 0) return `${hours} jam ${minutes} menit`;
  return `${minutes} menit`;
}

function storageLabel(storage: string) {
  return storage === 'gdrive' ? 'Google Drive' : storage === 'local' ? 'Penyimpanan lokal' : storage;
}

export function HealthPage() {
  const query = useQuery({
    queryKey: ['health'],
    queryFn: () => apiRequest<{ data: Health }>('/api/v1/system/health', { acceptedStatuses: [503] }),
    refetchInterval: 60_000,
  });
  const health = query.data?.data;

  return <div className="space-y-6">
    <PageHeader
      title="Kesehatan sistem"
      description="Pantau kesiapan aplikasi, database, penyimpanan, dan antrean pemindahan media."
      actions={<Button className={styles.refreshButton} variant="outline" onClick={() => query.refetch()} disabled={query.isFetching}>
        <RefreshCw className={query.isFetching ? styles.spinning : undefined} aria-hidden="true" />
        {query.isFetching ? 'Memeriksa…' : 'Periksa ulang'}
      </Button>}
    />

    {query.isError
      ? <DataState kind="error" title="Status sistem belum dapat diperiksa" description="Coba periksa ulang dalam beberapa saat." action={{ label: 'Periksa ulang', onClick: () => query.refetch() }} />
      : query.isPending
        ? <DataState kind="loading" title="Memeriksa kesehatan sistem" description="Menghubungkan status layanan utama." />
        : health
          ? <>
            <Alert className={`${styles.overall} ${styles[health.status]}`}>
              <StatusIcon status={health.status} />
              <AlertTitle>{statusText[health.status].title}</AlertTitle>
              <AlertDescription>
                <span>{statusText[health.status].description}</span>
                <span className={styles.checkedAt}>Terakhir diperiksa {new Date(health.checked_at).toLocaleString('id-ID')}</span>
              </AlertDescription>
            </Alert>

            <section className={styles.metricsSection} aria-label="Ringkasan kesehatan">
              <div className={styles.sectionHeading}>
                <div>
                  <h2>Ringkasan layanan</h2>
                  <p>Kondisi komponen yang paling berpengaruh pada operasional.</p>
                </div>
                <span className={styles.environment}>{health.environment} · {health.version}</span>
              </div>
              <div className={styles.metrics}>
                <article><Database aria-hidden="true" /><div><span>Database</span><strong>{health.database.status === 'healthy' ? 'Terhubung' : 'Bermasalah'}</strong><small>{health.database.message}</small></div></article>
                <article><Gauge aria-hidden="true" /><div><span>Latensi</span><strong>{health.database.latency_ms} ms</strong><small>Respons PostgreSQL</small></div></article>
                <article><CheckCircle2 aria-hidden="true" /><div><span>Migration</span><strong>Versi {health.migration_version}</strong><small>Skema database aktif</small></div></article>
                <article><Clock3 aria-hidden="true" /><div><span>Uptime</span><strong>{formatUptime(health.uptime_seconds)}</strong><small>Sejak aplikasi dijalankan</small></div></article>
                <article><HardDrive aria-hidden="true" /><div><span>Penyimpanan</span><strong>{storageLabel(health.storage_backend)}</strong><small>{health.media_worker_status === 'active' ? 'Worker pemindahan aktif' : 'Pemindahan tidak diperlukan'}</small></div></article>
              </div>
            </section>

            <section className={styles.queueSection} aria-label="Antrean pemindahan media">
              <div className={styles.queueIntro}>
                <div className={styles.queueIcon}><UploadCloud aria-hidden="true" /></div>
                <div><h2>Antrean pemindahan media</h2><p>{health.operations.message}</p></div>
                <span className={`${styles.workerStatus} ${health.media_worker_status === 'active' ? styles.workerActive : ''}`}>
                  {health.media_worker_status === 'active' ? 'Worker aktif' : 'Tidak diperlukan'}
                </span>
              </div>
              <dl className={styles.queueStats}>
                <div><dt>Menunggu</dt><dd>{health.media_moves.queued} menunggu</dd></div>
                <div><dt>Diproses</dt><dd>{health.media_moves.processing} diproses</dd></div>
                <div><dt><RotateCcw aria-hidden="true" /> Retry</dt><dd>{health.media_moves.retry} dijadwalkan ulang</dd></div>
                <div className={health.media_moves.failed > 0 ? styles.failed : undefined}><dt>Gagal</dt><dd>{health.media_moves.failed} gagal</dd></div>
              </dl>
            </section>
          </>
          : null}
  </div>;
}
