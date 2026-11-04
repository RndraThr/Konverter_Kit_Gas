import { useQueryClient } from '@tanstack/react-query';
import { RefreshCw, UploadCloud, X } from 'lucide-react';
import { useEffect, useLayoutEffect, useState } from 'react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet';
import { uploadQueue, useUploadCompletions, useUploadJobs, type UploadJob } from '@/lib/uploadQueue';
import { cn } from '@/lib/utils';

function statusText(job: UploadJob) {
  if (job.status === 'uploading') return `Mengunggah ${job.progress}%`;
  if (job.status === 'queued') return 'Menunggu antrian';
  return job.autoRetry ? 'Gagal, dicoba lagi otomatis' : 'Upload gagal';
}

/**
 * Indikator antrian unggah di header aplikasi. Selain menampilkan sisa
 * unggahan, komponen ini menyegarkan data halaman terkait saat unggahan selesai
 * (termasuk bila pengguna sudah pindah menu) dan mengingatkan sebelum tab ditutup.
 */
export function UploadQueueIndicator({ userId }: { userId: string }) {
  // Sebelum halaman mana pun sempat menambah antrian.
  useLayoutEffect(() => { uploadQueue.setOwner(userId); }, [userId]);
  const jobs = useUploadJobs();
  const client = useQueryClient();
  const [open, setOpen] = useState(false);
  const pending = jobs.filter((job) => job.status !== 'error').length;
  const failed = jobs.length - pending;

  useEffect(() => { uploadQueue.start(); }, []);
  useEffect(() => {
    if (!pending) return;
    const warn = (event: BeforeUnloadEvent) => { event.preventDefault(); };
    window.addEventListener('beforeunload', warn);
    return () => window.removeEventListener('beforeunload', warn);
  }, [pending]);
  useEffect(() => { if (!jobs.length) setOpen(false); }, [jobs.length]);

  useUploadCompletions(({ job }) => {
    if (job.target.kind === 'distribution-slot') {
      void client.invalidateQueries({ queryKey: ['distribution', 'slot-media'] });
      void client.invalidateQueries({ queryKey: ['distribution', 'slot-catalog'] });
    } else {
      void client.invalidateQueries({ queryKey: ['activities', job.target.activityType, job.target.programId, job.target.regencyId] });
      toast.success(`${job.label}: dokumentasi berhasil diunggah.`);
    }
  });

  if (!jobs.length) return null;
  const summary = pending ? `${pending} unggahan berjalan` : `${failed} unggahan gagal`;
  return <Sheet open={open} onOpenChange={setOpen}>
    <Button type="button" variant="outline" size="sm" aria-label={`Antrian unggah: ${summary}`} className={cn('gap-1.5', failed > 0 && 'border-destructive/50 text-destructive')} onClick={() => setOpen(true)}>
      <UploadCloud aria-hidden="true" className={cn(pending > 0 && 'animate-pulse')} />
      <span className="tabular-nums">{pending || failed}</span>
      <span className="hidden sm:inline">{pending ? 'unggahan' : 'gagal'}</span>
    </Button>
    <SheetContent className="w-full max-w-full gap-0 sm:max-w-md" aria-label="Antrian unggah">
      <SheetHeader className="border-b p-5 pr-14">
        <SheetTitle>Antrian unggah</SheetTitle>
        <SheetDescription>Unggahan tetap berjalan saat Anda pindah menu, dan dilanjutkan otomatis bila halaman dimuat ulang. Biarkan tab ini terbuka sampai selesai.</SheetDescription>
      </SheetHeader>
      <ul className="min-h-0 flex-1 divide-y overflow-y-auto">
        {jobs.map((job) => <li key={job.id} className="flex items-center gap-3 p-4">
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium" title={job.file.name}>{job.file.name}</p>
            <p className="truncate text-xs text-muted-foreground" title={job.label}>{job.label}</p>
            <p className={cn('text-xs', job.status === 'error' ? 'text-destructive' : 'text-muted-foreground')}>{statusText(job)}{job.status === 'error' && job.errorMessage ? ` — ${job.errorMessage}` : ''}</p>
            {job.status === 'uploading' && <div className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-muted" role="progressbar" aria-label={`Progres ${job.file.name}`} aria-valuemin={0} aria-valuemax={100} aria-valuenow={job.progress}><div className="h-full bg-primary transition-[width]" style={{ width: `${job.progress}%` }} /></div>}
          </div>
          {job.status === 'error' && <Button type="button" size="icon-sm" variant="ghost" aria-label={`Coba unggah lagi ${job.file.name}`} onClick={() => uploadQueue.retry(job.id)}><RefreshCw aria-hidden="true" /></Button>}
          <Button type="button" size="icon-sm" variant="ghost" aria-label={`Batalkan unggahan ${job.file.name}`} onClick={() => uploadQueue.cancel(job.id)}><X aria-hidden="true" /></Button>
        </li>)}
      </ul>
    </SheetContent>
  </Sheet>;
}
