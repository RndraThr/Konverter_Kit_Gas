import { CircleHelp } from 'lucide-react';
import { useState } from 'react';
import { Link, useLocation, useSearchParams } from 'react-router-dom';
import { Button } from '@/components/ui/button';
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle, SheetTrigger } from '@/components/ui/sheet';
import { GuideStepCard } from './GuideStepCard';
import { guideSteps, stepsForPath } from './guideContent';
import { sectionForTab, topicForStep } from './guideTopics';

/**
 * Tombol "?" di header: panduan khusus halaman yang sedang dibuka, sesuai
 * hak akses pengguna, dengan tautan ke halaman Panduan lengkap.
 */
export function PageHelp({ permissions }: { permissions: string[] }) {
  const { pathname } = useLocation();
  const [open, setOpen] = useState(false);
  const [params] = useSearchParams();
  const steps = stepsForPath(pathname);

  return <Sheet open={open} onOpenChange={setOpen}>
    <SheetTrigger render={<Button type="button" variant="ghost" size="icon" aria-label="Bantuan halaman ini" title="Bantuan halaman ini" />}>
      <CircleHelp aria-hidden="true" />
    </SheetTrigger>
    <SheetContent side="right" className="w-full overflow-y-auto data-[side=right]:w-full data-[side=right]:sm:max-w-2xl">
      <SheetHeader>
        <SheetTitle>Bantuan halaman ini</SheetTitle>
        <SheetDescription>Langkah kerja di halaman ini sesuai akses akun Anda.</SheetDescription>
      </SheetHeader>
      <div className="grid gap-4 px-4 pb-6">
        {steps.length > 0
          ? steps.map((step) => <GuideStepCard key={step.id} step={step} number={guideSteps.indexOf(step) + 1} permissions={permissions} currentPath={pathname} detailedSection={sectionForTab(topicForStep(step.id)?.slug ?? '', params.get('tab'))} onNavigate={() => setOpen(false)} />)
          : <p className="text-sm text-muted-foreground">Belum ada panduan khusus untuk halaman ini.</p>}
        <Link to="/panduan" onClick={() => setOpen(false)} className="text-sm font-medium text-primary hover:underline">Buka Panduan lengkap: alur kerja semua tim →</Link>
      </div>
    </SheetContent>
  </Sheet>;
}
