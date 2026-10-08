import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import type { TemplateEntry } from './types';

const kindLabel: Record<string, string> = { machine: 'Opsi mesin', converter: 'Opsi konkit', hose_suction: 'Opsi selang hisap', hose_discharge: 'Opsi selang buang', component: 'Komponen' };

/** "Komponen · OLI (Pertamina Enduro)" untuk barang template. */
export function templateEntryLabel(entry: TemplateEntry | undefined, ref: string) {
  if (!entry) return `${ref} (tidak ada di template)`;
  const brands = entry.variants.map((variant) => variant.brand).filter(Boolean).join(' / ');
  const kind = kindLabel[entry.kind] ?? entry.kind;
  return entry.kind === 'component' ? `${kind} · ${entry.label}${brands ? ` (${brands})` : ''}` : `${kind}${brands ? ` · ${brands}` : ''}`;
}

/** Pilihan barang Template Paket jadwal yang dirujuk baris TKDN/Pemeriksaan. */
export function TemplateEntrySelect({ entries, value, disabled, label, onChange }: { entries: TemplateEntry[]; value: string; disabled?: boolean; label: string; onChange: (ref: string) => void }) {
  const known = entries.some((entry) => entry.ref === value);
  return <Select disabled={disabled} value={value} onValueChange={(next) => next && onChange(next)}>
    <SelectTrigger className="w-full" aria-label={label}><SelectValue>{(ref: string) => templateEntryLabel(entries.find((entry) => entry.ref === ref), ref)}</SelectValue></SelectTrigger>
    <SelectContent>
      {!known && value && <SelectItem value={value}>{templateEntryLabel(undefined, value)}</SelectItem>}
      {entries.map((entry) => <SelectItem key={entry.ref} value={entry.ref}>{templateEntryLabel(entry, entry.ref)}</SelectItem>)}
    </SelectContent>
  </Select>;
}
