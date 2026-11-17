import { Combobox, ComboboxContent, ComboboxEmpty, ComboboxInput, ComboboxItem, ComboboxList } from './ui/combobox';

export type SearchableOption = { value: string; label: string };

/**
 * A select whose options can be filtered by typing, for long lists such as
 * kabupaten or jadwal. Takes the same string value as `Select`; an option
 * whose value is '' (e.g. "Semua kabupaten") is selectable like any other.
 */
export function SearchableSelect({ options, value, onValueChange, placeholder, emptyText = 'Tidak ditemukan.', disabled, required, className = 'w-full', ...aria }: {
  options: SearchableOption[];
  value: string;
  onValueChange: (value: string) => void;
  placeholder?: string;
  emptyText?: string;
  disabled?: boolean;
  required?: boolean;
  className?: string;
  'aria-labelledby'?: string;
  'aria-label'?: string;
}) {
  const selected = options.find((option) => option.value === value) ?? null;
  return <Combobox
    items={options}
    value={selected}
    isItemEqualToValue={(item: SearchableOption, current: SearchableOption) => item.value === current.value}
    onValueChange={(option: SearchableOption | null) => { if (option) onValueChange(option.value); }}
    autoHighlight
    required={required}
  >
    {/* Selecting the shown label on focus lets typing replace it, so the
        filter starts fresh instead of appending to the current choice. */}
    <ComboboxInput className={className} placeholder={placeholder} disabled={disabled} onFocus={(event) => event.currentTarget.select()} {...aria} />
    <ComboboxContent>
      <ComboboxEmpty>{emptyText}</ComboboxEmpty>
      <ComboboxList>{(option: SearchableOption) => <ComboboxItem key={option.value} value={option}>{option.label}</ComboboxItem>}</ComboboxList>
    </ComboboxContent>
  </Combobox>;
}
