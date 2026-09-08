import { ReactNode } from 'react';

type PageHeaderProps = {
  title: string;
  description: string;
  eyebrow?: ReactNode;
  actions?: ReactNode;
  context?: ReactNode;
};

export function PageHeader({ title, description, eyebrow, actions, context }: PageHeaderProps) {
  return <header className="flex flex-col gap-4 border-b pb-5 sm:flex-row sm:items-end sm:justify-between">
    <div className="min-w-0 space-y-1.5">
      {eyebrow ? <p className="text-xs font-semibold text-primary">{eyebrow}</p> : null}
      <h1 className="text-balance text-2xl font-semibold tracking-tight sm:text-[1.75rem]">{title}</h1>
      <p className="max-w-3xl text-sm leading-6 text-muted-foreground">{description}</p>
      {context}
    </div>
    {actions ? <div className="flex shrink-0 flex-wrap gap-2">{actions}</div> : null}
  </header>;
}
