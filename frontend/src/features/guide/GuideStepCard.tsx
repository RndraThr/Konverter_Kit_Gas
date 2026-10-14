import { ArrowRight, CheckCircle2, Eye, Lightbulb, UsersRound } from 'lucide-react';
import { Link } from 'react-router-dom';
import { Badge } from '@/components/ui/badge';
import { buttonVariants } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { GuideShot } from './GuideShot';
import { hasAny, type GuideStep } from './guideContent';
import { topicForStep } from './guideTopics';

export type StepAccess = 'task' | 'view' | 'other';

export function stepAccess(step: GuideStep, permissions: string[]): StepAccess {
  if (hasAny(permissions, step.manage)) return 'task';
  if (hasAny(permissions, step.view)) return 'view';
  return 'other';
}

type Props = {
  step: GuideStep;
  number: number;
  permissions: string[];
  /** Tampilkan contoh tampilan (screenshot); false untuk ringkasan. */
  detailed?: boolean;
  /** Halaman yang sedang dibuka: tombol "Buka halaman" disembunyikan bila sama. */
  currentPath?: string;
  /** Bagian tutorial yang dituju tombol "Tutorial lengkap" (mis. tab yang sedang dibuka). */
  detailedSection?: string;
  onNavigate?: () => void;
};

/**
 * Satu tahap alur kerja. Langkah yang membutuhkan permission yang tidak
 * dimiliki pengguna tetap ditampilkan (redup) dengan keterangan siapa yang
 * mengerjakannya, agar pengguna memahami alur secara utuh.
 */
export function GuideStepCard({ step, number, permissions, detailed = true, currentPath, detailedSection, onNavigate }: Props) {
  const access = stepAccess(step, permissions);
  const canOpen = hasAny(permissions, step.view) && currentPath !== step.route;
  const topic = topicForStep(step.id);
  return <section aria-labelledby={`guide-step-${step.id}`} className={cn('grid min-w-0 gap-4 rounded-xl border bg-card p-4 sm:p-5 [&>*]:min-w-0', access === 'task' ? 'border-primary/40 shadow-sm' : access === 'other' && 'opacity-80')}>
    <header className="flex flex-wrap items-start gap-3">
      <span className={cn('grid size-8 shrink-0 place-items-center rounded-full text-sm font-bold', access === 'task' ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground')}>{number}</span>
      <div className="grid min-w-0 flex-1 basis-56 gap-1">
        <div className="flex flex-wrap items-center gap-2">
          <h3 id={`guide-step-${step.id}`} className="text-base font-semibold">{step.title}</h3>
          {access === 'task' && <Badge>Tugas Anda</Badge>}
          {access === 'view' && <Badge variant="secondary"><Eye aria-hidden="true" />Anda dapat melihat</Badge>}
          {access === 'other' && <Badge variant="outline">Dikerjakan tim lain</Badge>}
        </div>
        <p className="text-sm text-muted-foreground">{step.summary}</p>
        <p className="flex items-center gap-1.5 text-xs text-muted-foreground"><UsersRound className="size-3.5" aria-hidden="true" />Biasanya oleh: {step.doers}</p>
      </div>
      <div className="flex flex-wrap gap-2">
        {topic && <Link to={`/panduan/${topic.slug}${detailedSection ? `#${detailedSection}` : ''}`} onClick={onNavigate} className={cn(buttonVariants({ variant: 'outline', size: 'sm' }))}>Tutorial lengkap<ArrowRight aria-hidden="true" /></Link>}
        {canOpen && <Link to={step.route} onClick={onNavigate} className={cn(buttonVariants({ variant: access === 'task' ? 'default' : 'outline', size: 'sm' }))}>Buka halaman<ArrowRight aria-hidden="true" /></Link>}
      </div>
    </header>

    {detailed && step.shots?.map((shot) => <GuideShot key={shot.id} spec={shot} />)}

    <ol className="grid gap-2.5">
      {step.tasks.map((task, index) => {
        const allowed = !task.permissions || hasAny(permissions, task.permissions);
        return <li key={index} className={cn('flex gap-2.5 text-sm leading-7', !allowed && 'text-muted-foreground')}>
          <span className={cn('mt-1 grid size-5 shrink-0 place-items-center rounded-full text-[11px] leading-none font-bold', task.n !== undefined ? 'bg-primary/15 text-primary' : 'bg-muted text-muted-foreground')}>{task.n ?? <span className="size-1.5 rounded-full bg-current" aria-hidden="true" />}</span>
          <span className="min-w-0">
            {task.text}
            {!allowed && <span className="ml-1 text-xs italic">(dikerjakan pemegang akses lain)</span>}
          </span>
        </li>;
      })}
    </ol>

    <p className="flex items-start gap-2 rounded-lg bg-emerald-50 px-3 py-2 text-sm text-emerald-900 dark:bg-emerald-950/30 dark:text-emerald-100"><CheckCircle2 className="mt-0.5 size-4 shrink-0" aria-hidden="true" /><span><b>Selesai bila:</b> {step.doneWhen}</span></p>
    {step.tips && step.tips.length > 0 && <ul className="grid gap-1.5 rounded-lg bg-amber-50 px-3 py-2 text-sm text-amber-950 dark:bg-amber-950/30 dark:text-amber-100">
      {step.tips.map((tip) => <li key={tip} className="flex items-start gap-2"><Lightbulb className="mt-0.5 size-4 shrink-0" aria-hidden="true" />{tip}</li>)}
    </ul>}
  </section>;
}
