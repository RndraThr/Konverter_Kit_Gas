import { useMutation, useQueryClient } from '@tanstack/react-query';
import { format, parseISO } from 'date-fns';
import { id as localeId } from 'date-fns/locale';
import { CalendarDays, ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight, Eraser, Hand, Info, Save, Trash2, Undo2, UserRoundCog, X } from 'lucide-react';
import { Fragment, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Calendar } from '@/components/ui/calendar';
import { Checkbox } from '@/components/ui/checkbox';
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { cn } from '@/lib/utils';
import { monitoringKey, saveAssignments, saveCalendar, todayWIB } from './api';
import { addDays, blockAt, clearRange, dayNumbers, daysBetween, moveBlock, moveRange, resizeBlock, shiftAll, type Block, type Days } from './calendarBlocks';
import { phases, rakornasStyle, type Overview, type Phase, type PhaseDay, type ScheduleRow } from './types';

export { addDays, dayNumbers } from './calendarBlocks';

type Tool = 'geser' | Phase | 'hapus' | 'rakornas';
/** Selected dates: a rectangle of kabupaten rows by days. */
export type Selection = { rowIds: string[]; from: string; to: string };
type Drag =
  | { kind: 'paint'; rowId: string }
  | { kind: 'move' | 'row'; rowId: string; block: Block; origin: string; delta: number }
  | { kind: 'start' | 'end'; rowId: string; block: Block; day: string }
  | { kind: 'select'; anchor: number; anchorDay: string; index: number; day: string }
  | { kind: 'shiftSel'; origin: string; delta: number };

// View: a rolling month from a few days ago (default), the whole sheet range, or a number of weeks.
type View = 'bulan' | 'jadwal' | 21 | 35 | 49;
// Rolling month: the last days still matter (yesterday's photos), the weeks ahead matter most.
const daysBefore = 3;
const monthDays = 31;
const views: { view: View; label: string }[] = [{ view: 'bulan', label: 'Bulan' }, { view: 'jadwal', label: 'Jadwal' }, { view: 21, label: '3 mg' }, { view: 35, label: '5 mg' }, { view: 49, label: '7 mg' }];
// Like the sheet, only delivery days and distribution days are numbered.
const numbered: Phase[] = ['material', 'distribusi'];
const toMap = (days: PhaseDay[]) => Object.fromEntries(days.map((d) => [d.day, d.phase])) as Days;
const label = (day: string, pattern: string) => format(parseISO(day), pattern, { locale: localeId });
// The sheet is always drawn in its own light colors, also in dark mode, so it reads like the Excel file.
const cell = 'border border-neutral-300';
const zoneTones = [
  { rows: 'bg-[#FFF2CC]', total: 'bg-[#FFC000]' },
  { rows: 'bg-[#E2EFDA]', total: 'bg-[#92D050]' },
];

/** A row's days with a single-row drag applied (the live preview, then the result). */
function applyDrag(days: Days, d: Drag): Days {
  switch (d.kind) {
    case 'move': return moveBlock(days, d.block, d.delta);
    case 'row': return shiftAll(days, d.delta);
    case 'start': case 'end': return resizeBlock(days, d.block, d.kind, d.day);
    default: return days;
  }
}

/** The rectangle between two corners of a selection drag. */
function rectangle(order: ScheduleRow[], a: number, b: number, dayA: string, dayB: string): Selection {
  return {
    rowIds: order.slice(Math.min(a, b), Math.max(a, b) + 1).map((row) => row.schedule_id),
    from: dayA < dayB ? dayA : dayB,
    to: dayA < dayB ? dayB : dayA,
  };
}

/** The sheet's range: a day before the first phase day to three days after the last. */
function fitRange(overview: Overview) {
  const all = overview.rows.flatMap((row) => row.days.map((d) => d.day)).concat(overview.rakornas).sort();
  if (all.length === 0) return { from: addDays(overview.date, -3), days: 35 };
  const from = addDays(all[0], -1);
  return { from, days: daysBetween(from, all[all.length - 1]) + 4 };
}

function ToolButton({ active, onClick, children, label: text }: { active: boolean; onClick: () => void; children: ReactNode; label: string }) {
  return <button type="button" role="radio" aria-checked={active} onClick={onClick} title={text}
    className={cn('flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs font-medium transition', active ? 'bg-foreground text-background shadow-sm' : 'text-muted-foreground hover:bg-muted hover:text-foreground')}>
    {children}<span className="hidden sm:inline">{text}</span>
  </button>;
}

/**
 * Jadwal: the phase calendar laid out like the "Jadwal Pendistribusian" sheet.
 * Geser: drag over empty cells to select dates (several kabupaten at once),
 * then drag the selection or press ← → to move it; drag one block to move it,
 * its first or last day to stretch it, Shift + drag to move a whole kabupaten.
 * The phase tools paint days, Rakornas is set on the date header. Changes stay
 * a draft until saved.
 */
export function CalendarPanel({ overview, canManage }: { overview: Overview; canManage: boolean }) {
  const queryClient = useQueryClient();
  const fit = useMemo(() => fitRange(overview), [overview]);
  const today = todayWIB();
  const [view, setView] = useState<View>('bulan');
  // Month view: from 3 days ago to 27 days ahead, so it moves with the date
  // (offset: days moved away from that). Other views keep a fixed start.
  const [offset, setOffset] = useState(0);
  const [start, setStart] = useState(fit.from);
  const from = view === 'bulan' ? addDays(today, offset - daysBefore) : start;
  const span = view === 'bulan' ? monthDays : view === 'jadwal' ? fit.days : view;
  const choose = (next: View) => {
    if (next === 'bulan') setOffset(0);
    else if (next === 'jadwal') setStart(fit.from);
    else setStart(from);
    setView(next);
  };
  const step = (direction: 1 | -1) => (view === 'bulan' ? setOffset((o) => o + 7 * direction) : setStart((d) => addDays(d, 7 * direction)));
  /** Shows the picked day: where today sits in month view, first otherwise. */
  const goTo = (day: string) => (view === 'bulan' ? setOffset(daysBetween(today, day)) : setStart(day));
  const [tool, setTool] = useState<Tool>('geser');
  const [draft, setDraft] = useState<Record<string, Days>>({});
  const [rakornas, setRakornas] = useState<string[] | null>(null);
  const [drag, setDrag] = useState<Drag | null>(null);
  const [selection, setSelection] = useState<Selection | null>(null);
  const [assigning, setAssigning] = useState<ScheduleRow | null>(null);
  const [picking, setPicking] = useState(false);
  // Where the selection pop-up shows: next to the pointer that made the selection.
  const [menuAt, setMenuAt] = useState<{ x: number; y: number } | null>(null);

  const zones = useMemo(() => {
    const groups: { zone: string; rows: ScheduleRow[] }[] = [];
    overview.rows.forEach((row) => {
      const zone = row.zone_name || 'Tanpa zona';
      const group = groups.find((g) => g.zone === zone);
      if (group) group.rows.push(row); else groups.push({ zone, rows: [row] });
    });
    return groups;
  }, [overview.rows]);
  const order = useMemo(() => zones.flatMap((g) => g.rows), [zones]);

  const dates = useMemo(() => Array.from({ length: span }, (_, i) => addDays(from, i)), [from, span]);
  const rakornasDays = rakornas ?? overview.rakornas;
  const dirtyRows = Object.keys(draft);
  const dirty = dirtyRows.length > 0 || rakornas !== null;
  const baseOf = (rows: Record<string, Days>, row: ScheduleRow) => rows[row.schedule_id] ?? toMap(row.days);

  // The selection shown: the one being dragged, else the saved one.
  const shownSelection = drag?.kind === 'select' ? rectangle(order, drag.anchor, drag.index, drag.anchorDay, drag.day) : selection;
  const selected = (rowId: string, day: string) => !!shownSelection && shownSelection.rowIds.includes(rowId) && day >= shownSelection.from && day <= shownSelection.to;

  const daysOf = (row: ScheduleRow) => {
    const days = baseOf(draft, row);
    if (!drag) return days;
    if (drag.kind === 'shiftSel') return selection?.rowIds.includes(row.schedule_id) ? moveRange(days, selection.from, selection.to, drag.delta) : days;
    if (drag.kind === 'move' || drag.kind === 'row' || drag.kind === 'start' || drag.kind === 'end') return drag.rowId === row.schedule_id ? applyDrag(days, drag) : days;
    return days;
  };

  /** Moves (or clears) the selected dates of every selected kabupaten. */
  const changeSelection = (delta: number | 'clear') => {
    if (!selection) return;
    setDraft((current) => {
      const next = { ...current };
      order.filter((row) => selection.rowIds.includes(row.schedule_id)).forEach((row) => {
        const days = baseOf(current, row);
        next[row.schedule_id] = delta === 'clear' ? clearRange(days, selection.from, selection.to) : moveRange(days, selection.from, selection.to, delta);
      });
      return next;
    });
    if (delta !== 'clear') setSelection({ ...selection, from: addDays(selection.from, delta), to: addDays(selection.to, delta) });
  };

  // Latest state for the window listeners below.
  const live = useRef({ drag, selection, order, changeSelection });
  useEffect(() => { live.current = { drag, selection, order, changeSelection }; });

  useEffect(() => {
    const finish = (event: PointerEvent) => {
      const { drag: d, selection: sel, order: rows, changeSelection: change } = live.current;
      if (!d) return;
      setDrag(null);
      const point = { x: event.clientX, y: event.clientY };
      if (d.kind === 'select') { setSelection(rectangle(rows, d.anchor, d.index, d.anchorDay, d.day)); setMenuAt(point); return; }
      if (d.kind === 'shiftSel') { if (sel && d.delta !== 0) change(d.delta); setMenuAt(point); return; }
      if (d.kind === 'paint') return;
      const row = rows.find((r) => r.schedule_id === d.rowId);
      if (!row) return;
      // A click on a block (no movement) selects it, ready for ← →.
      if (d.kind === 'move' && d.delta === 0) { setSelection({ rowIds: [d.rowId], from: d.block.start, to: d.block.end }); setMenuAt(point); return; }
      if ((d.kind === 'move' || d.kind === 'row') && d.delta === 0) return;
      setDraft((current) => ({ ...current, [d.rowId]: applyDrag(current[d.rowId] ?? toMap(row.days), d) }));
      setSelection(null);
    };
    const onKey = (event: KeyboardEvent) => {
      const { selection: sel, changeSelection: change } = live.current;
      const target = event.target as HTMLElement;
      if (!sel || ['INPUT', 'TEXTAREA'].includes(target.tagName) || target.isContentEditable) return;
      const step = event.shiftKey ? 7 : 1;
      if (event.key === 'ArrowRight') change(step);
      else if (event.key === 'ArrowLeft') change(-step);
      else if (event.key === 'Delete' || event.key === 'Backspace') { change('clear'); setSelection(null); }
      else if (event.key === 'Escape') setSelection(null);
      else return;
      event.preventDefault();
    };
    // Scrolling moves the cells away from the pop-up: hide it, the selection and its keys stay.
    const onScroll = () => setMenuAt(null);
    window.addEventListener('pointerup', finish);
    window.addEventListener('keydown', onKey);
    window.addEventListener('scroll', onScroll, true);
    return () => { window.removeEventListener('pointerup', finish); window.removeEventListener('keydown', onKey); window.removeEventListener('scroll', onScroll, true); };
  }, []);

  const paint = (row: ScheduleRow, day: string) => {
    if (tool === 'geser' || tool === 'rakornas') return;
    setDraft((current) => {
      const days = { ...baseOf(current, row) };
      if (tool === 'hapus') delete days[day]; else days[day] = tool;
      return { ...current, [row.schedule_id]: days };
    });
  };
  const startDrag = (row: ScheduleRow, day: string, event: React.PointerEvent<HTMLTableCellElement>) => {
    if (!canManage || event.button !== 0) return;
    event.preventDefault();
    if (tool !== 'geser') {
      if (tool === 'rakornas') return;
      setSelection(null);
      setDrag({ kind: 'paint', rowId: row.schedule_id });
      paint(row, day);
      return;
    }
    if (selection && selected(row.schedule_id, day)) { setDrag({ kind: 'shiftSel', origin: day, delta: 0 }); return; }
    const index = order.findIndex((r) => r.schedule_id === row.schedule_id);
    const block = blockAt(baseOf(draft, row), day);
    if (event.shiftKey && block) { setSelection(null); setDrag({ kind: 'row', rowId: row.schedule_id, block, origin: day, delta: 0 }); return; }
    if (!block) { setSelection(null); setDrag({ kind: 'select', anchor: index, anchorDay: day, index, day }); return; }
    setSelection(null);
    // The outer edge of a block's first or last day stretches it.
    const rect = event.currentTarget.getBoundingClientRect();
    const x = event.clientX - rect.left;
    if (day === block.end && x > rect.width - 8) setDrag({ kind: 'end', rowId: row.schedule_id, block, day });
    else if (day === block.start && x < 8) setDrag({ kind: 'start', rowId: row.schedule_id, block, day });
    else setDrag({ kind: 'move', rowId: row.schedule_id, block, origin: day, delta: 0 });
  };
  const overDay = (row: ScheduleRow, day: string) => {
    if (!drag) return;
    switch (drag.kind) {
      case 'paint': if (drag.rowId === row.schedule_id) paint(row, day); return;
      case 'select': setDrag({ ...drag, index: order.findIndex((r) => r.schedule_id === row.schedule_id), day }); return;
      case 'shiftSel': case 'move': case 'row': setDrag({ ...drag, delta: daysBetween(drag.origin, day) }); return;
      case 'start': case 'end': setDrag({ ...drag, day }); return;
    }
  };
  const toggleRakornas = (day: string) => {
    if (!canManage || tool !== 'rakornas') return;
    setRakornas((current) => {
      const list = current ?? overview.rakornas;
      return list.includes(day) ? list.filter((d) => d !== day) : [...list, day].sort();
    });
  };

  const save = useMutation({
    mutationFn: () => saveCalendar({
      program_id: overview.program_id,
      rows: dirtyRows.map((rowId) => ({ schedule_id: rowId, days: Object.entries(draft[rowId]).map(([day, phase]) => ({ day, phase })) })),
      ...(rakornas !== null ? { rakornas } : {}),
    }),
    onSuccess: () => {
      toast.success('Jadwal tersimpan');
      setDraft({}); setRakornas(null); setSelection(null);
      void queryClient.invalidateQueries({ queryKey: monitoringKey });
    },
    onError: (e) => toast.error(e.message),
  });

  const months = useMemo(() => {
    const out: { label: string; count: number }[] = [];
    dates.forEach((day) => {
      const name = label(day, 'MMMM');
      const last = out[out.length - 1];
      if (last?.label === name) last.count += 1; else out.push({ label: name, count: 1 });
    });
    return out;
  }, [dates]);
  const weekend = (day: string) => [0, 6].includes(new Date(`${day}T00:00:00Z`).getUTCDay());
  const fixed = canManage ? 5 : 4;
  const cursor = !canManage ? '' : tool === 'geser' ? 'cursor-cell' : tool === 'rakornas' ? '' : 'cursor-crosshair';
  const selectedDays = selection ? daysBetween(selection.from, selection.to) + 1 : 0;

  return <section className="space-y-3" aria-label="Jadwal tahapan">
    <div className="flex flex-wrap items-center gap-2 rounded-xl border bg-card p-2 shadow-xs">
      {canManage ? <div className="flex flex-wrap items-center gap-0.5" role="radiogroup" aria-label="Alat jadwal">
        <ToolButton active={tool === 'geser'} onClick={() => setTool('geser')} label="Pilih & geser"><Hand className="size-3.5" /></ToolButton>
        <Popover>
          <PopoverTrigger render={<button type="button" aria-label="Cara memilih dan menggeser" className="flex size-7 items-center justify-center rounded-lg text-muted-foreground hover:bg-muted hover:text-foreground" />}>
            <Info className="size-3.5" />
          </PopoverTrigger>
          <PopoverContent align="start" className="w-80 text-xs leading-5">
            <p className="mb-1 text-sm font-semibold">Pilih & geser</p>
            <ul className="list-disc space-y-0.5 pl-4">
              <li>Seret di sel kosong untuk memilih beberapa tanggal, boleh banyak kabupaten.</li>
              <li>Seret pilihan, atau tekan ← → (Shift: 7 hari), untuk menggeser sekaligus.</li>
              <li>Klik satu blok untuk memilihnya.</li>
              <li>Tarik ujung blok untuk mengubah panjangnya.</li>
              <li>Shift + seret blok menggeser seluruh jadwal kabupaten.</li>
              <li>Delete mengosongkan pilihan, Esc membatalkan.</li>
            </ul>
          </PopoverContent>
        </Popover>
        <span className="mx-1 h-5 w-px bg-border" aria-hidden="true" />
        {phases.map((p) => <ToolButton key={p.phase} active={tool === p.phase} onClick={() => setTool(p.phase)} label={p.short}><span className={cn('size-3 rounded-sm ring-1 ring-black/10', p.dot)} /></ToolButton>)}
        <ToolButton active={tool === 'rakornas'} onClick={() => setTool('rakornas')} label="Rakornas"><span className={cn('size-3 rounded-sm', rakornasStyle.dot)} /></ToolButton>
        <span className="mx-1 h-5 w-px bg-border" aria-hidden="true" />
        <ToolButton active={tool === 'hapus'} onClick={() => setTool('hapus')} label="Hapus"><Eraser className="size-3.5" /></ToolButton>
      </div> : <p className="px-2 text-sm text-muted-foreground">Jadwal tahapan per kabupaten</p>}
      <div className="ml-auto flex flex-wrap items-center gap-2">
        <div className="flex items-center rounded-lg border bg-background p-0.5" role="radiogroup" aria-label="Rentang tampilan">
          {views.map((v) => <button key={v.view} type="button" role="radio" aria-checked={view === v.view} onClick={() => choose(v.view)}
            className={cn('rounded-md px-2 py-1 text-xs font-medium', view === v.view ? 'bg-muted text-foreground' : 'text-muted-foreground hover:text-foreground')}>{v.label}</button>)}
        </div>
        <div className="flex items-center overflow-hidden rounded-lg border bg-background shadow-xs">
          <button type="button" aria-label="Mundur seminggu" onClick={() => step(-1)} className="flex h-8 w-8 items-center justify-center text-muted-foreground hover:bg-muted hover:text-foreground"><ChevronLeft className="size-4" /></button>
          <Popover open={picking} onOpenChange={setPicking}>
            <PopoverTrigger render={<button type="button" aria-label="Pilih tanggal mulai" className="flex h-8 items-center gap-2 border-x px-3 text-sm font-medium hover:bg-muted" />}>
              <CalendarDays className="size-4 text-primary" />
              <span>{label(from, 'd MMM')} – {label(addDays(from, span - 1), 'd MMM yyyy')}</span>
            </PopoverTrigger>
            <PopoverContent align="end" className="w-auto p-0">
              <Calendar mode="single" locale={localeId} defaultMonth={parseISO(from)} selected={parseISO(from)}
                onSelect={(value) => { if (value) { goTo(format(value, 'yyyy-MM-dd')); setPicking(false); } }} />
            </PopoverContent>
          </Popover>
          <button type="button" aria-label="Maju seminggu" onClick={() => step(1)} className="flex h-8 w-8 items-center justify-center text-muted-foreground hover:bg-muted hover:text-foreground"><ChevronRight className="size-4" /></button>
        </div>
        <Button variant="outline" size="sm" onClick={() => (view === 'bulan' ? setOffset(0) : setStart(addDays(today, -3)))}>Hari ini</Button>
      </div>
    </div>
    {canManage && tool === 'geser' && selection && menuAt ? <div role="dialog" aria-label="Pilihan tanggal"
      className="fixed z-50 flex items-center gap-1 rounded-xl border bg-popover p-1.5 text-popover-foreground shadow-lg ring-1 ring-foreground/5 animate-in fade-in-0 zoom-in-95"
      style={{ left: Math.max(8, Math.min(menuAt.x - 150, window.innerWidth - 320)), top: menuAt.y + 70 > window.innerHeight ? menuAt.y - 56 : menuAt.y + 14 }}
      onPointerDown={(e) => e.stopPropagation()}>
      <span className="px-1.5 text-xs whitespace-nowrap text-muted-foreground">
        <b className="text-foreground">{selection.rowIds.length}</b> kab · <b className="text-foreground">{selectedDays}</b> hari · {label(selection.from, 'd MMM')}–{label(selection.to, 'd MMM')}
      </span>
      <span className="mx-0.5 h-5 w-px bg-border" aria-hidden="true" />
      <Button variant="ghost" size="icon-sm" title="Mundur 7 hari" aria-label="Geser pilihan 7 hari lebih awal" onClick={() => changeSelection(-7)}><ChevronsLeft /></Button>
      <Button variant="ghost" size="icon-sm" title="Mundur 1 hari" aria-label="Geser pilihan 1 hari lebih awal" onClick={() => changeSelection(-1)}><ChevronLeft /></Button>
      <Button variant="ghost" size="icon-sm" title="Maju 1 hari" aria-label="Geser pilihan 1 hari lebih lambat" onClick={() => changeSelection(1)}><ChevronRight /></Button>
      <Button variant="ghost" size="icon-sm" title="Maju 7 hari" aria-label="Geser pilihan 7 hari lebih lambat" onClick={() => changeSelection(7)}><ChevronsRight /></Button>
      <span className="mx-0.5 h-5 w-px bg-border" aria-hidden="true" />
      <Button variant="ghost" size="icon-sm" title="Kosongkan" aria-label="Kosongkan pilihan" className="text-destructive hover:text-destructive" onClick={() => { changeSelection('clear'); setSelection(null); }}><Trash2 /></Button>
      <Button variant="ghost" size="icon-sm" title="Batal (Esc)" aria-label="Batal pilih" onClick={() => setSelection(null)}><X /></Button>
    </div> : null}
    {/* Floats over the page, so unsaved changes never push the calendar down. */}
    {canManage && dirty ? <div role="status" className="fixed bottom-5 left-1/2 z-40 flex -translate-x-1/2 items-center gap-3 rounded-2xl border bg-foreground py-2 pr-2 pl-4 text-sm text-background shadow-2xl animate-in fade-in-0 slide-in-from-bottom-4">
      <span className="whitespace-nowrap"><b>{dirtyRows.length}</b> kabupaten{rakornas !== null ? ' + Rakornas' : ''} belum disimpan</span>
      <div className="flex gap-2">
        <Button variant="ghost" size="sm" className="text-background hover:bg-background/15 hover:text-background" onClick={() => { setDraft({}); setRakornas(null); setSelection(null); }} disabled={save.isPending}><Undo2 />Batalkan</Button>
        <Button size="sm" className="bg-background text-foreground hover:bg-background/90" onClick={() => save.mutate()} disabled={save.isPending}><Save />Simpan jadwal</Button>
      </div>
    </div> : null}
    <div className="overflow-x-auto rounded-xl border border-neutral-300 bg-white text-neutral-900 select-none [color-scheme:light]">
      <table className="border-collapse text-xs">
        {zones.map((group, zoneIndex) => {
          const tone = zoneTones[zoneIndex % zoneTones.length];
          return <Fragment key={group.zone}>
            <thead>
              <tr><th colSpan={fixed + dates.length} className={cn(cell, 'bg-[#A9D08E] py-2 text-center text-base font-bold tracking-wide')}>{group.zone.toUpperCase()}</th></tr>
              <tr className="bg-[#C6E0B4]">
                <th rowSpan={2} className={cn(cell, 'sticky left-0 z-20 w-9 bg-[#C6E0B4] px-1 text-sm')}>No.</th>
                <th rowSpan={2} className={cn(cell, 'px-2 text-left text-sm')}>Provinsi</th>
                <th rowSpan={2} className={cn(cell, 'sticky left-9 z-20 min-w-40 bg-[#C6E0B4] px-2 text-left text-sm')}>Kab/Kota</th>
                <th rowSpan={2} className={cn(cell, 'px-2 text-right text-sm')}>Target</th>
                {canManage ? <th rowSpan={2} className={cn(cell, 'px-2 text-left text-sm')}>Pemeriksa</th> : null}
                {months.map((m) => <th key={m.label} colSpan={m.count} className={cn(cell, 'px-2 py-0.5 text-center text-sm font-bold capitalize')}>{m.label}</th>)}
              </tr>
              <tr className="bg-[#E2EFDA]">
                {dates.map((day) => <th key={day} onClick={() => toggleRakornas(day)}
                  className={cn(cell, 'h-6 w-7 min-w-7 text-center font-semibold tabular-nums', weekend(day) && 'text-red-600', rakornasDays.includes(day) && rakornasStyle.cell,
                    day === today && 'shadow-[inset_0_-3px_0_#2563eb]', tool === 'rakornas' && canManage && 'cursor-pointer hover:bg-[#7030A0]/30')}
                  title={rakornasDays.includes(day) ? 'Rakornas' : `${label(day, 'EEEE, d MMMM yyyy')}${day === today ? ' (hari ini)' : ''}`}>
                  {Number(day.slice(8))}
                </th>)}
              </tr>
            </thead>
            <tbody>
              {group.rows.map((row, i) => {
                const days = daysOf(row);
                const numbers = dayNumbers(days);
                const changed = draft[row.schedule_id] !== undefined;
                return <tr key={row.schedule_id} className={tone.rows}>
                  <td className={cn(cell, 'sticky left-0 z-5 px-1 text-center tabular-nums', tone.rows)}>{row.sheet_no || i + 1}</td>
                  <td className={cn(cell, 'px-2 whitespace-nowrap')}>{row.province_name}</td>
                  <td className={cn(cell, 'sticky left-9 z-5 px-2 whitespace-nowrap font-medium', tone.rows)}>{row.regency_name}{changed ? <span className="ml-1 text-blue-600" title="Belum disimpan">●</span> : null}</td>
                  <td className={cn(cell, 'px-2 text-right tabular-nums')}>{row.quota.toLocaleString('id-ID')}</td>
                  {canManage ? <td className={cn(cell, 'px-1')}>
                    <button type="button" onClick={() => setAssigning(row)} className="flex max-w-44 items-center gap-1 rounded px-1 py-0.5 text-left hover:bg-white/70">
                      <UserRoundCog className="size-3.5 shrink-0 text-neutral-500" />
                      <span className={cn('truncate', row.reviewers.length === 0 && 'text-neutral-500 italic')}>{row.reviewers.length ? row.reviewers.map((r) => r.name).join(', ') : 'Atur'}</span>
                    </button>
                  </td> : null}
                  {dates.map((day) => {
                    const phase = days[day];
                    const style = phases.find((p) => p.phase === phase);
                    const prev = days[addDays(day, -1)] === phase, next = days[addDays(day, 1)] === phase;
                    const isSelected = selected(row.schedule_id, day);
                    return <td key={day}
                      onPointerDown={(e) => startDrag(row, day, e)}
                      onPointerEnter={() => overDay(row, day)}
                      className={cn(cell, 'relative h-6 w-7 min-w-7 bg-white text-center font-semibold tabular-nums', style?.cell, cursor,
                        phase && tool === 'geser' && canManage && 'cursor-grab active:cursor-grabbing',
                        isSelected && 'shadow-[inset_0_0_0_2px_#2563eb]', isSelected && !phase && 'bg-blue-100')}
                      title={style ? `${style.label}${numbered.includes(style.phase) ? ` hari ${numbers[day]}` : ''}` : undefined}>
                      {phase && numbered.includes(phase) ? numbers[day] : ''}
                      {phase && canManage && tool === 'geser' && !prev ? <span className="absolute inset-y-0 left-0 w-1.5 cursor-ew-resize hover:bg-black/25" aria-hidden="true" /> : null}
                      {phase && canManage && tool === 'geser' && !next ? <span className="absolute inset-y-0 right-0 w-1.5 cursor-ew-resize hover:bg-black/25" aria-hidden="true" /> : null}
                    </td>;
                  })}
                </tr>;
              })}
              <tr className={cn('font-bold', tone.total)}>
                <td colSpan={3} className={cn(cell, 'sticky left-0 z-5 px-2 text-center', tone.total)}>TOTAL</td>
                <td className={cn(cell, 'px-2 text-right tabular-nums')}>{group.rows.reduce((sum, row) => sum + row.quota, 0).toLocaleString('id-ID')}</td>
                <td colSpan={dates.length + (canManage ? 1 : 0)} className={cell} />
              </tr>
              <tr aria-hidden="true"><td colSpan={fixed + dates.length} className="h-5 bg-white" /></tr>
            </tbody>
          </Fragment>;
        })}
      </table>
    </div>
    <div className="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-xl border bg-card px-3 py-2 text-xs">
      <span className="font-semibold">Keterangan</span>
      {[...phases.slice(0, 1), { phase: 'rakornas', label: 'Rakornas', dot: rakornasStyle.dot }, ...phases.slice(1)].map((p) => <span key={p.phase} className="flex items-center gap-1.5">
        <span className={cn('h-3 w-6 rounded-sm ring-1 ring-black/10', p.dot)} />{p.label}
      </span>)}
      <span className="text-muted-foreground">Angka = hari ke-n pengiriman material atau distribusi.</span>
    </div>
    {assigning ? <AssignmentDialog overview={overview} row={assigning} onClose={() => setAssigning(null)} /> : null}
  </section>;
}

function PeoplePicker({ label: title, people, selected, onChange }: { label: string; people: { id: string; name: string }[]; selected: string[]; onChange: (ids: string[]) => void }) {
  const [search, setSearch] = useState('');
  const shown = people.filter((p) => p.name.toLowerCase().includes(search.trim().toLowerCase()));
  return <div className="space-y-2">
    <p className="text-sm font-medium">{title} <span className="font-normal text-muted-foreground">({selected.length})</span></p>
    {people.length > 8 ? <Input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Cari nama" aria-label={`Cari ${title}`} /> : null}
    <div className="max-h-48 space-y-1 overflow-y-auto rounded-lg border p-2">
      {shown.length === 0 ? <p className="text-xs text-muted-foreground">Tidak ada pengguna.</p> : shown.map((p) => <Label key={p.id} className="flex items-center gap-2 rounded px-1 py-0.5 font-normal hover:bg-muted">
        <Checkbox checked={selected.includes(p.id)} onCheckedChange={(checked) => onChange(checked === true ? [...selected, p.id] : selected.filter((x) => x !== p.id))} />{p.name}
      </Label>)}
    </div>
  </div>;
}

/** Who checks a kabupaten (central team) and who leads it in the field (gets revision notifications). */
function AssignmentDialog({ overview, row, onClose }: { overview: Overview; row: ScheduleRow; onClose: () => void }) {
  const queryClient = useQueryClient();
  const [reviewers, setReviewers] = useState(row.reviewers.map((r) => r.id));
  const [leaders, setLeaders] = useState(row.leaders.map((r) => r.id));
  const save = useMutation({
    mutationFn: () => saveAssignments({ schedule_id: row.schedule_id, reviewers, leaders }),
    onSuccess: () => { toast.success(`Penugasan ${row.regency_name} tersimpan`); void queryClient.invalidateQueries({ queryKey: monitoringKey }); onClose(); },
    onError: (e) => toast.error(e.message),
  });
  return <Dialog open onOpenChange={(open) => { if (!open) onClose(); }}>
    <DialogContent className="sm:max-w-lg">
      <DialogHeader><DialogTitle>Penugasan {row.regency_name}</DialogTitle></DialogHeader>
      <PeoplePicker label="Pemeriksa (tim pusat)" people={overview.reviewers} selected={reviewers} onChange={setReviewers} />
      <PeoplePicker label="Leader lapangan (menerima notifikasi revisi)" people={overview.users} selected={leaders} onChange={setLeaders} />
      <DialogFooter>
        <Button variant="outline" onClick={onClose}>Batal</Button>
        <Button onClick={() => save.mutate()} disabled={save.isPending}><Save />Simpan</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}
