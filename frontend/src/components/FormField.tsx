import { InputHTMLAttributes, ReactNode, useId } from 'react';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { cn } from '@/lib/utils';

type Props = InputHTMLAttributes<HTMLInputElement> & {
  label: string;
  error?: string;
  hint?: ReactNode;
  // Marks the label as "Wajib" or "Opsional" without turning on native browser validation.
  requirement?: 'required' | 'optional';
};

export function RequirementMark({ requirement }: { requirement: 'required' | 'optional' }) {
  return requirement === 'required'
    ? <span className="rounded-sm bg-destructive/10 px-1.5 py-0.5 text-[11px] font-semibold uppercase tracking-wide text-destructive">Wajib</span>
    : <span className="rounded-sm bg-muted px-1.5 py-0.5 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">Opsional</span>;
}

export function FormField({ label, error, hint, requirement, id, className, ...input }: Props) {
  const generatedID = useId();
  const fieldID = id ?? input.name ?? generatedID;
  const descriptionID = error ? `${fieldID}-error` : hint ? `${fieldID}-hint` : undefined;
  const describedBy = [input['aria-describedby'], descriptionID].filter(Boolean).join(' ') || undefined;

  return <div className={cn('grid min-w-0 gap-2', className)}>
    {requirement
      ? <div className="flex items-center gap-2"><Label htmlFor={fieldID}>{label}</Label><RequirementMark requirement={requirement} /></div>
      : <Label htmlFor={fieldID}>{label}</Label>}
    <Input
      {...input}
      id={fieldID}
      aria-describedby={describedBy}
      aria-invalid={error ? true : input['aria-invalid']}
      aria-required={requirement === 'required' ? true : input['aria-required']}
    />
    {error ? <p className="text-sm text-destructive" id={descriptionID} role="alert">{error}</p> : hint ? <p className="text-sm text-muted-foreground" id={descriptionID}>{hint}</p> : null}
  </div>;
}
