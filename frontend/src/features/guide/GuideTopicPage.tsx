import { ArrowLeft, ArrowRight, CheckCircle2, CircleHelp, ListChecks } from 'lucide-react';
import { useEffect } from 'react';
import { Link, Navigate, useLocation, useParams } from 'react-router-dom';
import { PageHeader } from '@/components/PageHeader';
import { Badge } from '@/components/ui/badge';
import { buttonVariants } from '@/components/ui/button';
import { usePermissions } from '@/lib/permissions';
import { cn } from '@/lib/utils';
import { GuideShot } from './GuideShot';
import { hasAny } from './guideContent';
import { topicBySlug, type TopicSection } from './guideTopics';

/**
 * Tutorial rinci satu fitur: daftar isi, lalu setiap bagian berisi
 * screenshot bernomor (termasuk modal), langkah, penjelasan isian, dan
 * masalah umum. Langkah yang membutuhkan akses lain tetap tampil dengan
 * keterangan.
 */
export function GuideTopicPage() {
  const { slug } = useParams();
  const { hash } = useLocation();
  const permissions = usePermissions();
  const topic = topicBySlug(slug);

  // Lompat ke bagian dari tautan (mis. tombol "?" di tab Jadwal → #jadwal).
  useEffect(() => {
    if (!hash) return;
    const timer = window.setTimeout(() => document.getElementById(decodeURIComponent(hash.slice(1)))?.scrollIntoView({ block: 'start' }), 50);
    return () => window.clearTimeout(timer);
  }, [hash, slug]);

  if (!topic) return <Navigate to="/panduan" replace />;
  const canManage = hasAny(permissions, topic.manage);
  const canOpen = hasAny(permissions, topic.view);

  return <div className="grid gap-6">
    <Link to="/panduan" className="inline-flex w-fit items-center gap-1.5 text-sm font-medium text-primary hover:underline"><ArrowLeft className="size-4" aria-hidden="true" />Kembali ke Panduan</Link>
    <PageHeader
      eyebrow="Tutorial"
      title={topic.title}
      description={topic.intro}
      actions={canOpen ? <Link to={topic.route} className={buttonVariants()}>Buka {topic.title}<ArrowRight aria-hidden="true" /></Link> : undefined}
      context={<div className="pt-1">{canManage ? <Badge>Tugas Anda</Badge> : canOpen ? <Badge variant="secondary">Anda dapat melihat</Badge> : <Badge variant="outline">Dikerjakan tim lain</Badge>}</div>}
    />

    <div className="grid min-w-0 grid-cols-1 gap-6 lg:grid-cols-[14rem_minmax(0,1fr)]">
      <nav aria-label="Daftar isi" className="min-w-0 lg:sticky lg:top-20 lg:self-start">
        <p className="mb-2 flex items-center gap-1.5 text-xs font-semibold text-muted-foreground uppercase"><ListChecks className="size-3.5" aria-hidden="true" />Daftar isi</p>
        <ol className="flex gap-1.5 overflow-x-auto pb-1 lg:grid lg:overflow-visible">
          {topic.sections.map((section, index) => <li key={section.id} className="shrink-0">
            <a href={`#${section.id}`} className="block rounded-md px-2.5 py-1.5 text-sm whitespace-nowrap text-muted-foreground hover:bg-muted hover:text-foreground lg:whitespace-normal">{index + 1}. {section.title}</a>
          </li>)}
        </ol>
      </nav>

      <div className="grid min-w-0 gap-6">
        {topic.sections.map((section, index) => <TopicSectionCard key={section.id} section={section} number={index + 1} permissions={permissions} />)}
      </div>
    </div>
  </div>;
}

function TopicSectionCard({ section, number, permissions }: { section: TopicSection; number: number; permissions: string[] }) {
  return <section id={section.id} aria-labelledby={`guide-section-title-${section.id}`} className="grid min-w-0 scroll-mt-20 gap-5 rounded-xl border bg-card p-4 sm:p-6 [&>*]:min-w-0">
    <header className="grid gap-1">
      <h2 id={`guide-section-title-${section.id}`} className="flex items-center gap-2.5 text-lg font-semibold"><span className="grid size-7 shrink-0 place-items-center rounded-full bg-primary text-sm text-primary-foreground">{number}</span>{section.title}</h2>
      <p className="text-sm leading-6 text-muted-foreground">{section.summary}</p>
    </header>

    {section.shots?.map((shot) => <GuideShot key={shot.id} spec={shot} />)}

    {section.steps.length > 0 && <div className="grid gap-2">
      <h3 className="text-sm font-semibold">Langkah</h3>
      <ol className="grid gap-2.5">
        {section.steps.map((step, index) => {
          const allowed = !step.permissions || hasAny(permissions, step.permissions);
          return <li key={index} className={cn('flex gap-2.5 text-sm leading-7', !allowed && 'text-muted-foreground')}>
            <span className={cn('mt-1 grid size-5 shrink-0 place-items-center rounded-full text-[11px] leading-none font-bold', step.n !== undefined ? 'bg-primary/15 text-primary' : 'bg-muted text-muted-foreground')}>{step.n ?? <span className="size-1.5 rounded-full bg-current" aria-hidden="true" />}</span>
            <span className="min-w-0">{step.text}{!allowed && <span className="ml-1 text-xs italic">(dikerjakan pemegang akses lain)</span>}</span>
          </li>;
        })}
      </ol>
    </div>}

    {section.fields && section.fields.length > 0 && <div className="grid gap-2">
      <h3 className="text-sm font-semibold">Penjelasan isian</h3>
      <div className="overflow-x-auto rounded-lg border">
        <table className="w-full min-w-[36rem] text-left text-sm">
          <thead className="bg-muted/50 text-xs text-muted-foreground"><tr><th className="px-3 py-2 font-medium">Isian</th><th className="px-3 py-2 font-medium">Arti</th><th className="px-3 py-2 font-medium">Contoh</th><th className="px-3 py-2 font-medium">Wajib</th></tr></thead>
          <tbody>{section.fields.map((field) => <tr key={field.name} className="border-t align-top">
            <td className="px-3 py-2 font-medium whitespace-nowrap">{field.name}</td>
            <td className="px-3 py-2 text-muted-foreground">{field.meaning}</td>
            <td className="px-3 py-2">{field.example ?? '—'}</td>
            <td className="px-3 py-2">{field.required ? 'Ya' : 'Tidak'}</td>
          </tr>)}</tbody>
        </table>
      </div>
    </div>}

    {section.problems && section.problems.length > 0 && <div className="grid gap-2">
      <h3 className="flex items-center gap-1.5 text-sm font-semibold"><CircleHelp className="size-4 text-amber-600" aria-hidden="true" />Masalah umum</h3>
      <dl className="grid gap-2">{section.problems.map((problem) => <div key={problem.q} className="rounded-lg bg-amber-50 px-3 py-2 text-sm text-amber-950 dark:bg-amber-950/30 dark:text-amber-100"><dt className="font-medium">{problem.q}</dt><dd className="mt-0.5">{problem.a}</dd></div>)}</dl>
    </div>}

    {section.doneWhen && <p className="flex items-start gap-2 rounded-lg bg-emerald-50 px-3 py-2 text-sm text-emerald-900 dark:bg-emerald-950/30 dark:text-emerald-100"><CheckCircle2 className="mt-0.5 size-4 shrink-0" aria-hidden="true" /><span><b>Selesai bila:</b> {section.doneWhen}</span></p>}
  </section>;
}
