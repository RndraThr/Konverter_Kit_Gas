import { AlertCircle, Inbox, LoaderCircle } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';

type DataStateKind = 'loading' | 'empty' | 'error';

type DataStateProps = {
  kind: DataStateKind;
  title: string;
  description: string;
  action?: {
    label: string;
    onClick: () => void;
  };
};

const stateIcon = {
  loading: LoaderCircle,
  empty: Inbox,
  error: AlertCircle,
};

export function DataState({ kind, title, description, action }: DataStateProps) {
  const Icon = stateIcon[kind];
  const liveRole = kind === 'error' ? 'alert' : kind === 'loading' ? 'status' : undefined;

  return <section className="rounded-lg border border-dashed bg-muted/20 px-5 py-8 text-center" role={liveRole} aria-busy={kind === 'loading' || undefined}>
    <div className={cn('mx-auto flex size-11 items-center justify-center rounded-full bg-secondary text-secondary-foreground', kind === 'error' && 'bg-destructive/10 text-destructive')}>
      <Icon className={cn('size-5', kind === 'loading' && 'animate-spin')} aria-hidden="true" />
    </div>
    <h2 className="mt-4 text-base font-semibold">{title}</h2>
    <p className="mx-auto mt-1 max-w-md text-sm leading-6 text-muted-foreground">{description}</p>
    {kind === 'loading' ? <Skeleton className="mx-auto mt-5 h-11 w-32" aria-hidden="true" /> : null}
    {action ? <Button className="mt-5" type="button" variant={kind === 'error' ? 'outline' : 'default'} onClick={action.onClick}>{action.label}</Button> : null}
  </section>;
}
