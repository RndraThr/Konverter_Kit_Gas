import { CircleCheck, Search } from 'lucide-react';
import { useMemo, useState } from 'react';
import { DataState } from '@/components/DataState';
import { SearchableSelect } from '@/components/SearchableSelect';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { cn } from '@/lib/utils';
import { useQueue } from './api';
import { ReviewDialog } from './ReviewDialog';
import { stateLabels, type Overview, type QueueItem, type ReviewState, type ScheduleRow } from './types';

type Filter = 'belum' | 'revisi' | 'berjalan' | 'disetujui' | 'semua';
const filters: { value: Filter; label: string; states: ReviewState[] }[] = [
  { value: 'belum', label: 'Ada foto baru', states: ['pending'] },
  { value: 'revisi', label: 'Perlu revisi', states: ['revision'] },
  { value: 'berjalan', label: 'Menunggu foto', states: ['partial'] },
  { value: 'disetujui', label: 'Disetujui', states: ['approved'] },
  { value: 'semua', label: 'Semua', states: ['pending', 'revision', 'partial', 'approved'] },
];

const formatTime = (value: string | null) => value ? new Date(value).toLocaleString('id-ID', { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' }) : '-';

/**
 * Antrean cek: a searchable list of kabupaten on the left (mine, or all for
 * managers), the selected kabupaten's recipients by review state on the
 * right, and the review dialog. On small screens the list becomes a
 * searchable dropdown.
 */
export function QueuePanel({ overview, userId, canSeeAll, scheduleId, slot, onSchedule, onSlot }: {
  overview: Overview; userId: string; canSeeAll: boolean; scheduleId: string; slot: number | null;
  onSchedule: (scheduleId: string) => void; onSlot: (slot: number | null) => void;
}) {
  const mine = overview.rows.filter((row) => row.reviewers.some((r) => r.id === userId));
  const [showAll, setShowAll] = useState(mine.length === 0 && canSeeAll);
  const regencies = showAll
    ? overview.rows.filter((row) => row.completed > 0 || row.pending > 0 || row.revision > 0 || row.today_phase === 'distribusi' || row.schedule_id === scheduleId)
    : mine.length > 0 || !canSeeAll ? mine : [];
  const selected = regencies.find((row) => row.schedule_id === scheduleId) ?? overview.rows.find((row) => row.schedule_id === scheduleId) ?? regencies[0];
  const queue = useQueue(selected?.schedule_id ?? '');
  const [filter, setFilter] = useState<Filter>('belum');
  const [search, setSearch] = useState('');

  const items = useMemo(() => {
    const states = filters.find((f) => f.value === filter)!.states;
    const term = search.trim().toLowerCase();
    return (queue.data ?? [])
      .filter((item) => states.includes(item.state))
      .filter((item) => !term || item.full_name.toLowerCase().includes(term) || item.nik.includes(term) || String(item.slot_number) === term || item.machine_serial.toLowerCase().includes(term))
      // Oldest upload first: photos are checked in the order the field sent them.
      .sort((a, b) => (a.last_upload_at ?? '').localeCompare(b.last_upload_at ?? '') || a.slot_number - b.slot_number);
  }, [queue.data, filter, search]);
  const counts = (states: ReviewState[]) => (queue.data ?? []).filter((item) => states.includes(item.state)).length;

  // Next recipient to check after the one just handled: the next one still unchecked in this list.
  const nextAfter = (current: number): number | null => {
    const unchecked = items.filter((item) => item.state === 'pending' && item.slot_number !== current);
    const position = items.findIndex((item) => item.slot_number === current);
    return (unchecked.find((item) => items.indexOf(item) > position) ?? unchecked[0])?.slot_number ?? null;
  };

  return <section className="grid gap-4 lg:grid-cols-[17.5rem_minmax(0,1fr)] lg:items-start" aria-label="Antrean cek">
    <RegencyPicker rows={regencies} selectedId={selected?.schedule_id ?? ''} onSelect={onSchedule}
      scopeToggle={canSeeAll ? { all: showAll, mine: mine.length, onChange: setShowAll } : null} />
    <div className="min-w-0 space-y-4">
      {!selected ? <DataState kind="empty" title="Tidak ada antrean" description={mine.length || showAll ? 'Pilih kabupaten di daftar.' : 'Belum ada kabupaten yang ditugaskan ke Anda. Admin membaginya di tab Jadwal.'} />
        : <>
          <div className="flex flex-wrap items-end justify-between gap-3">
            <div className="min-w-0">
              <h2 className="truncate text-lg font-semibold">{selected.regency_name}</h2>
              <p className="text-xs text-muted-foreground">{selected.province_name} · {selected.completed} selesai · {selected.approved} disetujui{selected.reviewers.length ? ` · pemeriksa ${selected.reviewers.map((r) => r.name).join(', ')}` : ''}</p>
            </div>
            <div className="relative w-full sm:w-64">
              <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Nama, NIK, nomor bagi, no. mesin" className="pl-8" aria-label="Cari pelanggan" />
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {filters.map((f) => <button key={f.value} type="button" onClick={() => setFilter(f.value)} aria-pressed={filter === f.value}
              className={cn('rounded-full border px-3 py-1 text-sm', filter === f.value ? 'border-foreground bg-foreground text-background' : 'hover:bg-muted')}>
              {f.label} <span className="tabular-nums opacity-70">{counts(f.states)}</span>
            </button>)}
          </div>
          {queue.isPending ? <DataState kind="loading" title="Memuat antrean" description={selected.regency_name} />
            : queue.isError ? <DataState kind="error" title="Antrean belum dapat dimuat" description={queue.error.message} action={{ label: 'Muat ulang', onClick: () => void queue.refetch() }} />
            : items.length === 0 ? <DataState kind="empty" title={filter === 'belum' ? 'Semua foto sudah dicek' : 'Tidak ada pelanggan'} description={filter === 'belum' ? 'Foto baru dari lapangan, termasuk POS Mesin yang diisi lebih awal, muncul di sini secara langsung.' : 'Tidak ada pelanggan pada filter ini.'} />
            : <div className="overflow-x-auto rounded-lg border">
              <table className="w-full text-sm">
                <thead className="bg-muted/50 text-left text-xs text-muted-foreground">
                  <tr><th className="px-3 py-2">No.</th><th className="px-3 py-2">Pelanggan</th><th className="px-3 py-2">Desa</th><th className="px-3 py-2">No. mesin</th><th className="px-3 py-2">Foto</th><th className="px-3 py-2">Selesai</th><th className="px-3 py-2">Status</th></tr>
                </thead>
                <tbody>{items.map((item) => <QueueRow key={item.slot_number} item={item} onOpen={() => onSlot(item.slot_number)} />)}</tbody>
              </table>
            </div>}
          <ReviewDialog scheduleId={selected.schedule_id} slot={slot} onClose={() => onSlot(null)} onNext={(current) => onSlot(nextAfter(current))}
            onStep={(current, step) => {
              const position = items.findIndex((item) => item.slot_number === current);
              const target = items[position + step];
              if (target) onSlot(target.slot_number);
            }} />
        </>}
    </div>
  </section>;
}

/**
 * The kabupaten to check: a list with search, grouped by zone, each with
 * its photos still to check. A dropdown with search on small screens.
 */
function RegencyPicker({ rows, selectedId, onSelect, scopeToggle }: {
  rows: ScheduleRow[]; selectedId: string; onSelect: (scheduleId: string) => void;
  scopeToggle: { all: boolean; mine: number; onChange: (all: boolean) => void } | null;
}) {
  const [term, setTerm] = useState('');
  const [onlyNew, setOnlyNew] = useState(false);
  const shown = rows.filter((row) => (!onlyNew || row.pending_photos > 0 || row.schedule_id === selectedId)
    && (!term.trim() || `${row.regency_name} ${row.province_name}`.toLowerCase().includes(term.trim().toLowerCase())));
  const zones = shown.reduce<{ zone: string; rows: ScheduleRow[] }[]>((groups, row) => {
    const zone = row.zone_name || 'Tanpa zona';
    const group = groups.find((g) => g.zone === zone);
    if (group) group.rows.push(row); else groups.push({ zone, rows: [row] });
    return groups;
  }, []);
  const totalNew = rows.reduce((sum, row) => sum + row.pending_photos, 0);

  return <aside className="space-y-2 lg:sticky lg:top-4">
    {scopeToggle ? <div className="flex rounded-lg border bg-card p-0.5 text-sm" role="radiogroup" aria-label="Cakupan kabupaten">
      {[{ all: false, label: `Tugas saya (${scopeToggle.mine})` }, { all: true, label: 'Semua kabupaten' }].map((option) => <button key={String(option.all)} type="button" role="radio" aria-checked={scopeToggle.all === option.all}
        onClick={() => scopeToggle.onChange(option.all)}
        className={cn('flex-1 rounded-md px-2 py-1.5 font-medium', scopeToggle.all === option.all ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:text-foreground')}>{option.label}</button>)}
    </div> : null}
    <div className="lg:hidden">
      <SearchableSelect aria-label="Kabupaten" options={rows.map((row) => ({ value: row.schedule_id, label: `${row.regency_name}${row.pending_photos ? ` · ${row.pending_photos} foto baru` : ''}` }))}
        value={selectedId} onValueChange={onSelect} placeholder="Pilih kabupaten" />
    </div>
    <div className="hidden overflow-hidden rounded-xl border bg-card lg:flex lg:max-h-[calc(100dvh-9rem)] lg:flex-col">
      <div className="space-y-2 border-b p-2.5">
        <div className="relative">
          <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input value={term} onChange={(e) => setTerm(e.target.value)} placeholder="Cari kabupaten / provinsi" className="h-8 pl-8" aria-label="Cari kabupaten" />
        </div>
        <Label className="flex items-center justify-between gap-2 text-xs font-normal text-muted-foreground">
          <span className="flex items-center gap-2"><Checkbox checked={onlyNew} onCheckedChange={(checked) => setOnlyNew(checked === true)} />Hanya yang ada foto baru</span>
          <span className="tabular-nums">{totalNew} foto</span>
        </Label>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto p-1.5" role="listbox" aria-label="Kabupaten">
        {zones.length === 0 ? <p className="px-2 py-6 text-center text-xs text-muted-foreground">Tidak ada kabupaten.</p> : zones.map((group) => <div key={group.zone} className="mb-1.5">
          <p className="px-2 pt-1.5 pb-1 text-[11px] font-semibold tracking-wide text-muted-foreground uppercase">{group.zone}</p>
          {group.rows.map((row) => {
            const active = row.schedule_id === selectedId;
            return <button key={row.schedule_id} type="button" role="option" aria-selected={active} onClick={() => onSelect(row.schedule_id)}
              className={cn('flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left transition', active ? 'bg-primary/10 ring-1 ring-primary/40' : 'hover:bg-muted')}>
              <span className="min-w-0 flex-1">
                <span className={cn('block truncate text-sm', active && 'font-semibold text-primary')}>{row.regency_name}</span>
                <span className="block truncate text-[11px] text-muted-foreground">{row.province_name} · {row.completed} selesai</span>
              </span>
              {row.pending_photos > 0
                ? <span className="shrink-0 rounded-full bg-amber-400 px-2 py-0.5 text-xs font-semibold text-neutral-900 tabular-nums" title={`${row.pending_photos} foto belum dicek`}>{row.pending_photos}</span>
                : row.revision > 0 ? <span className="shrink-0 rounded-full bg-destructive/15 px-2 py-0.5 text-xs font-semibold text-destructive tabular-nums" title={`${row.revision} pelanggan perlu revisi`}>{row.revision}</span>
                : row.completed > 0 ? <CircleCheck className="size-4 shrink-0 text-emerald-600" aria-label="Semua foto sudah dicek" /> : null}
            </button>;
          })}
        </div>)}
      </div>
    </div>
  </aside>;
}

function QueueRow({ item, onOpen }: { item: QueueItem; onOpen: () => void }) {
  const state = stateLabels[item.state];
  return <tr className="cursor-pointer border-t hover:bg-muted/40" onClick={onOpen}>
    <td className="px-3 py-2 font-medium tabular-nums">#{item.slot_number}</td>
    <td className="px-3 py-2"><button type="button" className="text-left font-medium hover:underline" onClick={(e) => { e.stopPropagation(); onOpen(); }}>{item.full_name || '-'}</button><p className="text-xs text-muted-foreground tabular-nums">{item.nik}</p></td>
    <td className="px-3 py-2">{item.village || '-'}</td>
    <td className="px-3 py-2 font-mono text-xs">{item.machine_serial || '-'}</td>
    <td className="px-3 py-2 text-xs">
      <div className="flex h-1.5 w-24 overflow-hidden rounded-full bg-muted" aria-hidden="true">
        <span className="bg-emerald-500" style={{ width: `${(item.approved_photos / item.photos) * 100}%` }} />
        <span className="bg-destructive" style={{ width: `${(item.rejected_photos / item.photos) * 100}%` }} />
      </div>
      <p className="mt-1 tabular-nums text-muted-foreground">
        {item.approved_photos}/{item.photos} disetujui
        {item.pending_photos > 0 ? <b className="ml-1 text-amber-700 dark:text-amber-300">+{item.pending_photos} baru</b> : null}
        {item.rejected_photos > 0 ? <b className="ml-1 text-destructive">{item.rejected_photos} ditolak</b> : null}
      </p>
    </td>
    <td className="px-3 py-2 text-xs">{item.status === 'completed' ? <>{formatTime(item.completed_at)}<p className="text-muted-foreground">{item.completed_by}</p></> : <span className="text-muted-foreground">Belum selesai</span>}</td>
    <td className="px-3 py-2">
      <span className={cn('rounded-md px-2 py-0.5 text-xs font-medium', state.tone)}>{state.label}</span>
      {item.reviewed_by ? <p className="mt-0.5 text-xs text-muted-foreground">{item.reviewed_by}</p> : null}
    </td>
  </tr>;
}
