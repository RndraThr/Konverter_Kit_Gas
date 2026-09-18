import { Search, X } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import { Label } from '../../components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../components/ui/select';

type FilterOption = { value: string; label: string };

export function SetupToolbar({ entity, search, onSearchChange, status, onStatusChange, options, shown, total }: {
  entity: string;
  search: string;
  onSearchChange: (value: string) => void;
  status: string;
  onStatusChange: (value: string) => void;
  options: FilterOption[];
  shown: number;
  total: number;
}) {
  const hasFilters = Boolean(search || status !== 'all');
  return <div className="setupToolbar">
    <div className="setupToolbarControls">
      <div className="setupSearch"><Search aria-hidden="true" /><Input type="search" role="searchbox" aria-label={`Cari ${entity}`} placeholder={`Cari ${entity}...`} value={search} onChange={(event) => onSearchChange(event.target.value)} /></div>
      <div className="setupFilter"><Label className="sr-only" id={`${entity}-status-filter-label`}>Filter status {entity}</Label><Select value={status} onValueChange={(value) => onStatusChange(value ?? 'all')}><SelectTrigger aria-labelledby={`${entity}-status-filter-label`}><SelectValue /></SelectTrigger><SelectContent>{options.map((option) => <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>)}</SelectContent></Select></div>
      {hasFilters && <Button type="button" variant="ghost" className="setupReset" onClick={() => { onSearchChange(''); onStatusChange('all'); }}><X />Reset</Button>}
    </div>
    <p className="setupResult" aria-live="polite">{shown} dari {total} {entity}</p>
  </div>;
}
