import { useId, useMemo, useState, type KeyboardEvent } from 'react';
import { Search } from 'lucide-react';
import { cn } from '../../../lib/utils';
import { searchMap, type SearchRegency, type SearchResult } from './search';
import type { PointFeature } from './types';

const PLACEHOLDER = 'Cari kabupaten di Indonesia atau nomor bagi…';

type Props = {
  regencies: SearchRegency[];
  points: PointFeature[];
  regencyNames: Map<string, string>;
  onPick: (result: SearchResult) => void;
};

export function MapSearch({ regencies, points, regencyNames, onPick }: Props) {
  const listId = useId();
  const [text, setText] = useState('');
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const results = useMemo(() => searchMap(text, regencies, points, regencyNames), [text, regencies, points, regencyNames]);
  const expanded = open && results.length > 0;

  const pick = (result: SearchResult) => {
    setText(result.label);
    setOpen(false);
    onPick(result);
  };

  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'ArrowDown') { event.preventDefault(); setOpen(true); setActive((i) => Math.min(results.length - 1, i + 1)); }
    else if (event.key === 'ArrowUp') { event.preventDefault(); setActive((i) => Math.max(0, i - 1)); }
    else if (event.key === 'Enter' && expanded) { event.preventDefault(); pick(results[Math.min(active, results.length - 1)]); }
    else if (event.key === 'Escape') setOpen(false);
  };

  return <div className="relative min-w-0 flex-1 sm:max-w-sm">
    <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
    <input
      type="search"
      role="combobox"
      aria-label={PLACEHOLDER}
      aria-expanded={expanded}
      aria-controls={listId}
      aria-autocomplete="list"
      aria-activedescendant={expanded ? `${listId}-${active}` : undefined}
      placeholder={PLACEHOLDER}
      className="h-11 w-full rounded-md border bg-background pr-3 pl-9 text-sm outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
      value={text}
      onChange={(event) => { setText(event.target.value); setOpen(true); setActive(0); }}
      onFocus={() => setOpen(true)}
      onBlur={() => setOpen(false)}
      onKeyDown={onKeyDown}
    />
    {expanded && <ul id={listId} role="listbox" aria-label="Hasil pencarian peta" className="absolute z-20 mt-1 max-h-72 w-full overflow-auto rounded-md border bg-popover p-1 shadow-md">
      {results.map((result, index) => <li
        key={`${result.kind}-${result.id}`}
        id={`${listId}-${index}`}
        role="option"
        aria-selected={index === active}
        className={cn('flex min-h-11 cursor-pointer flex-col justify-center rounded-sm px-3 py-1.5 text-sm', index === active && 'bg-muted')}
        // mousedown keeps focus in the input so blur does not close the list before the pick.
        onMouseDown={(event) => event.preventDefault()}
        onClick={() => pick(result)}
      >
        <span className="font-medium">{result.label}</span>
        <span className="text-xs text-muted-foreground">{result.detail}</span>
      </li>)}
    </ul>}
  </div>;
}
