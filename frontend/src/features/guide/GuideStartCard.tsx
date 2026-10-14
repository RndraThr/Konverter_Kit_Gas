import { ArrowRight, Sparkles, X } from 'lucide-react';
import { useState } from 'react';
import { Link } from 'react-router-dom';
import { Button, buttonVariants } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import { stepAccess } from './GuideStepCard';
import { guideSteps, hasAny } from './guideContent';

const dismissedKey = 'konkit.guide.start-dismissed';

type Props = {
  permissions: string[];
  /** Kartu di Dashboard/Profil bisa ditutup; di halaman Panduan selalu tampil. */
  dismissible?: boolean;
};

/**
 * "Mulai dari sini": tahap yang bisa dikerjakan pengguna sesuai permission,
 * berurutan, masing-masing dengan langkah pertamanya dan tautan ke halaman.
 */
export function GuideStartCard({ permissions, dismissible = false }: Props) {
  const [dismissed, setDismissed] = useState(() => {
    if (!dismissible) return false;
    try { return window.localStorage.getItem(dismissedKey) === 'true'; } catch { return false; }
  });
  if (dismissed) return null;

  const mine = guideSteps.filter((step) => stepAccess(step, permissions) === 'task');
  const viewOnly = guideSteps.filter((step) => stepAccess(step, permissions) === 'view');
  const dismiss = () => {
    setDismissed(true);
    try { window.localStorage.setItem(dismissedKey, 'true'); } catch { /* Penyimpanan preferensi mungkin tidak tersedia. */ }
  };

  return <Card className="border-primary/30 bg-primary/5">
    <CardHeader className="flex flex-row items-start justify-between gap-3">
      <div className="grid gap-1">
        <CardTitle className="flex items-center gap-2 text-base"><Sparkles className="size-4 text-primary" aria-hidden="true" />Mulai dari sini</CardTitle>
        <p className="text-sm text-muted-foreground">{mine.length > 0 ? 'Tugas Anda di Konkit, sesuai hak akses akun ini, berurutan:' : 'Akun ini untuk memantau. Halaman yang bisa Anda buka:'}</p>
      </div>
      {dismissible && <Button type="button" size="icon-sm" variant="ghost" aria-label="Tutup panduan awal" onClick={dismiss}><X aria-hidden="true" /></Button>}
    </CardHeader>
    <CardContent className="grid gap-3">
      <ol className="grid gap-2">
        {(mine.length > 0 ? mine : viewOnly).map((step, index) => {
          const firstTask = step.tasks.find((task) => !task.permissions || hasAny(permissions, task.permissions));
          return <li key={step.id} className="grid gap-2 rounded-lg border bg-background p-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
            <div className="grid gap-0.5">
              <strong className="text-sm">{index + 1}. {step.title}</strong>
              <span className="text-sm leading-7 text-muted-foreground">{firstTask?.text ?? step.summary}</span>
            </div>
            <Link to={step.route} className={cn(buttonVariants({ size: 'sm', variant: index === 0 ? 'default' : 'outline' }), 'justify-self-start')}>Buka<ArrowRight aria-hidden="true" /></Link>
          </li>;
        })}
      </ol>
      {(mine.length > 0 || viewOnly.length > 0)
        ? <Link to="/panduan" className="text-sm font-medium text-primary hover:underline">Lihat panduan lengkap dan alur kerja semua tim →</Link>
        : <p className="text-sm text-muted-foreground">Akun ini belum memiliki akses ke menu kerja. Hubungi admin untuk diberi role.</p>}
    </CardContent>
  </Card>;
}
