import { Button } from '../../../components/ui/button';

type Props<T extends string> = {
  label: string;
  value: T;
  options: Array<{ value: T; label: string }>;
  onChange: (value: T) => void;
};

/** Single-choice button group; the active option carries aria-pressed. */
export function Segmented<T extends string>({ label, value, options, onChange }: Props<T>) {
  return <div role="group" aria-label={label} className="inline-flex flex-wrap gap-1 rounded-lg border bg-muted/40 p-1">
    {options.map((option) => <Button
      key={option.value}
      type="button"
      size="sm"
      variant={option.value === value ? 'default' : 'ghost'}
      aria-pressed={option.value === value}
      onClick={() => onChange(option.value)}
    >{option.label}</Button>)}
  </div>;
}
