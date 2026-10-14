import { BookOpenText, CircleHelp, Compass, Route } from 'lucide-react';
import { useState } from 'react';
import { PageHeader } from '@/components/PageHeader';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { usePermissions } from '@/lib/permissions';
import { GuideStartCard } from './GuideStartCard';
import { GuideStepCard, stepAccess } from './GuideStepCard';
import { glossary, guideIntro, guideSteps } from './guideContent';

/**
 * Halaman Panduan: untuk apa sistem ini, mulai dari mana sesuai role, alur
 * kerja lengkap (tahap milik pengguna ditandai), dan daftar istilah.
 */
export function GuidePage() {
  const permissions = usePermissions();
  const [onlyMine, setOnlyMine] = useState(false);
  const mine = guideSteps.filter((step) => stepAccess(step, permissions) === 'task');
  const shown = onlyMine ? mine : guideSteps;

  return <div className="grid gap-6">
    <PageHeader eyebrow="Bantuan" title="Panduan Konkit" description="Penjelasan singkat sistem, langkah awal sesuai akses Anda, dan alur kerja dari persiapan program sampai laporan." />

    <Card>
      <CardHeader><CardTitle className="flex items-center gap-2 text-base"><Compass className="size-4 text-primary" aria-hidden="true" />{guideIntro.title}</CardTitle></CardHeader>
      <CardContent className="grid gap-4 text-sm leading-6">
        <p>{guideIntro.body}</p>
        <ol aria-label="Alur besar" className="flex flex-wrap items-center gap-1.5 text-xs">
          {guideSteps.map((step, index) => <li key={step.id} className="flex items-center gap-1.5">
            <a href={`#guide-step-${step.id}`} className={`rounded-full border px-2.5 py-1 hover:bg-muted ${stepAccess(step, permissions) === 'task' ? 'border-primary bg-primary/10 font-medium text-foreground' : 'text-muted-foreground'}`}>{index + 1}. {step.title}</a>
            {index < guideSteps.length - 1 && <span aria-hidden="true" className="text-muted-foreground">→</span>}
          </li>)}
        </ol>
        <p className="flex items-center gap-2 text-xs text-muted-foreground"><CircleHelp className="size-4" aria-hidden="true" />Di setiap halaman, tombol <b>?</b> di kanan atas membuka panduan khusus halaman itu.</p>
      </CardContent>
    </Card>

    <GuideStartCard permissions={permissions} />

    <section aria-labelledby="guide-workflow" className="grid gap-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 id="guide-workflow" className="flex items-center gap-2 text-lg font-semibold"><Route className="size-5 text-primary" aria-hidden="true" />Alur kerja</h2>
        {mine.length > 0 && <Button type="button" size="sm" variant={onlyMine ? 'default' : 'outline'} aria-pressed={onlyMine} onClick={() => setOnlyMine((value) => !value)}>{onlyMine ? 'Tampilkan semua tahap' : 'Hanya tugas saya'}</Button>}
      </div>
      {shown.map((step) => <GuideStepCard key={step.id} step={step} number={guideSteps.indexOf(step) + 1} permissions={permissions} />)}
    </section>

    <Card>
      <CardHeader><CardTitle className="flex items-center gap-2 text-base"><BookOpenText className="size-4 text-primary" aria-hidden="true" />Istilah</CardTitle></CardHeader>
      <CardContent>
        <dl className="grid gap-3 text-sm sm:grid-cols-2">
          {glossary.map((item) => <div key={item.term} className="rounded-lg border p-3"><dt className="font-semibold">{item.term}</dt><dd className="mt-0.5 text-muted-foreground">{item.meaning}</dd></div>)}
        </dl>
      </CardContent>
    </Card>
  </div>;
}
