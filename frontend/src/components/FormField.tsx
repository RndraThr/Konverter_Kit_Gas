import { InputHTMLAttributes, ReactNode, useId } from 'react';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';

type Props = InputHTMLAttributes<HTMLInputElement> & {
  label: string;
  error?: string;
  hint?: ReactNode;
};

export function FormField({ label, error, hint, id, ...input }: Props) {
  const generatedID = useId();
  const fieldID = id ?? input.name ?? generatedID;
  const descriptionID = error ? `${fieldID}-error` : hint ? `${fieldID}-hint` : undefined;
  const describedBy = [input['aria-describedby'], descriptionID].filter(Boolean).join(' ') || undefined;

  return <div className="grid gap-2">
    <Label htmlFor={fieldID}>{label}</Label>
    <Input
      {...input}
      id={fieldID}
      aria-describedby={describedBy}
      aria-invalid={error ? true : input['aria-invalid']}
      className={input.className}
    />
    {error ? <p className="text-sm text-destructive" id={descriptionID} role="alert">{error}</p> : hint ? <p className="text-sm text-muted-foreground" id={descriptionID}>{hint}</p> : null}
  </div>;
}
