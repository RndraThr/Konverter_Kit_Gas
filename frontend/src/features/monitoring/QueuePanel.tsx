import { Search } from 'lucide-react';
import { useMemo, useState } from 'react';
import { DataState } from '@/components/DataState';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { cn } from '@/lib/utils';
import { useQueue } from './api';
import { ReviewDialog } from './ReviewDialog';
import { stateLabels, type Overview, type QueueItem, type ReviewState } from './types';

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
 * Antrean cek: one tab per kabupaten assigned to me (all, for managers), its
 * completed recipients by review state, and the review dialog.
 */
export function QueuePanel({ overview, userId, canSeeAll, scheduleId, slot, onSchedule, onSlot }: {
  overview: Overview; userId: string; canSeeAll: boolean; scheduleId: string; slot: number | null;
  onSchedule: (scheduleId: string) => void; onSlot: (slot: number | null) => void;
}) {
  const mine = overview.rows.filter((row) => row.reviewers.some((r) => r.id === userId));
  const [showAll, setShowAll] = useState(mine.length === 0 && canSeeAll);
  const tabs = showAll
    ? overview.rows.filter((row) => row.completed > 0 || row.pending > 0 || row.revision > 0 || row.today_phase === 'distribusi' || row.schedule_id === scheduleId)
    : mine.length > 0 || !canSeeAll ? mine : [];
  const selected = tabs.find((row) => row.schedule_id === scheduleId) ?? overview.rows.find((row) => row.schedule_id === scheduleId) ?? tabs[0];
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

  return <section className="space-y-4" aria-label="Antrean cek">
    <div className="flex flex-wrap items-center justify-between gap-3">
      <p className="text-sm text-muted-foreground">{showAll ? 'Semua kabupaten yang sudah ada pelanggan selesai.' : mine.length ? `${mine.length} kabupaten ditugaskan ke Anda.` : 'Belum ada kabupaten yang ditugaskan ke Anda. Admin membaginya di tab Jadwal.'}</p>
      {canSeeAll ? <Label className="flex items-center gap-2 text-sm font-normal">
        <Checkbox checked={showAll} onCheckedChange={(checked) => setShowAll(checked === true)} />Semua kabupaten
      </Label> : null}
    </div>
    {tabs.length > 0 ? <div role="tablist" aria-label="Kabupaten" className="-mx-1 flex gap-1.5 overflow-x-auto px-1 pb-1">
      {tabs.map((row) => {
        const active = row.schedule_id === selected?.schedule_id;
        return <button key={row.schedule_id} type="button" role="tab" aria-selected={active} onClick={() => onSchedule(row.schedule_id)}
          className={cn('flex shrink-0 items-center gap-2 rounded-lg border px-3 py-1.5 text-sm transition', active ? 'border-primary bg-primary text-primary-foreground' : 'bg-card hover:bg-muted')}>
          {row.regency_name}
          {row.pending > 0 ? <span className={cn('rounded-full px-1.5 text-xs tabular-nums', active ? 'bg-primary-foreground/20' : 'bg-amber-100 text-amber-900 dark:bg-amber-950/40 dark:text-amber-200')}>{row.pending}</span> : null}
        </button>;
      })}
    </div> : null}
    {!selected ? <DataState kind="empty" title="Tidak ada antrean" description="Pilih kabupaten atau minta admin menugaskan kabupaten ke Anda." />
      : <>
        <div className="flex flex-wrap items-center gap-2">
          {filters.map((f) => <button key={f.value} type="button" onClick={() => setFilter(f.value)} aria-pressed={filter === f.value}
            className={cn('rounded-full border px-3 py-1 text-sm', filter === f.value ? 'border-foreground bg-foreground text-background' : 'hover:bg-muted')}>
            {f.label} <span className="tabular-nums opacity-70">{counts(f.states)}</span>
          </button>)}
          <div className="relative ml-auto w-full sm:w-64">
            <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Nama, NIK, nomor bagi, no. mesin" className="pl-8" aria-label="Cari pelanggan" />
          </div>
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
  </section>;
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
