import { InputHTMLAttributes, ReactNode } from 'react';

type Props = InputHTMLAttributes<HTMLInputElement> & {
  label: string;
  error?: string;
  hint?: ReactNode;
};

export function FormField({ label, error, hint, id, ...input }: Props) {
  const fieldID = id ?? input.name;
  return <label className="formField" htmlFor={fieldID}>
    <span>{label}</span>
    <input id={fieldID} aria-invalid={Boolean(error)} aria-describedby={error ? `${fieldID}-error` : undefined} {...input} />
    {error ? <small className="fieldError" id={`${fieldID}-error`}>{error}</small> : hint ? <small>{hint}</small> : null}
  </label>;
}
