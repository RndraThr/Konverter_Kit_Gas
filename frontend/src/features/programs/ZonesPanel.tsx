import { DndContext, DragOverlay, KeyboardSensor, PointerSensor, useDraggable, useDroppable, useSensor, useSensors, type DragEndEvent, type DragStartEvent } from '@dnd-kit/core';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { AlertTriangle, Check, GripVertical, MapPinned, Pencil, Plus, Search, Trash2, X } from 'lucide-react';
import { useEffect, useMemo, useState, type FormEvent } from 'react';
import { useSearchParams } from 'react-router-dom';
import { toast } from 'sonner';
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { DataState } from '@/components/DataState';
import { ApiError, apiRequest } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { uppercaseBusinessText } from '../../lib/text';
import type { DataResponse, Program, ProgramZone, Regency } from './types';
import { ZonePOPanel } from './ZonePOPanel';
import { programTypeLabel } from './types';

const unassignedColumn = '__unassigned__';
const columnDragPrefix = 'column:';
const naturalName = new Intl.Collator('id', { numeric: true, sensitivity: 'base' });
const orderKey = (programID: string) => `konkit.zone-order.${programID}`;

// Urutan kolom zona hanya disimpan di browser pengguna (localStorage).
function readZoneOrder(programID: string): string[] {
  try {
    const saved = JSON.parse(window.localStorage.getItem(orderKey(programID)) ?? '[]');
    return Array.isArray(saved) ? saved.filter((id): id is string => typeof id === 'string') : [];
  } catch {
    return [];
  }
}

function writeZoneOrder(programID: string, order: string[]) {
  try {
    window.localStorage.setItem(orderKey(programID), JSON.stringify(order));
  } catch {
    // Penyimpanan lokal tidak tersedia: urutan kembali ke bawaan saat dimuat ulang.
  }
}
const errorMessage = (error: unknown, fallback: string) => (error instanceof ApiError || error instanceof Error ? error.message : fallback);

/** Papan zona: kabupaten dipindah antar zona dengan drag & drop, atau dicentang lalu dipindah sekaligus. */
export function ZonesPanel() {
  const canManage = useCan('programs.manage');
  const canViewPO = useCan('bast.view');
  const canManagePO = useCan('bast.manage');
  const client = useQueryClient();
  // ?program= dipakai tautan "Isi No. PO per zona" dari Berita Acara.
  const [params] = useSearchParams();
  const [programID, setProgramID] = useState(params.get('program') ?? '');
  const [search, setSearch] = useState('');
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [pending, setPending] = useState<Record<string, string>>({});
  const [dragging, setDragging] = useState<string | null>(null);
  const [adding, setAdding] = useState(false);
  const [zoneOrder, setZoneOrder] = useState<string[]>([]);
  const programs = useQuery({ queryKey: ['program-setup', 'programs'], queryFn: () => apiRequest<DataResponse<Program[]>>('/api/v1/program-setup/programs') });
  const regencies = useQuery({ queryKey: ['program-setup', 'regencies'], queryFn: () => apiRequest<DataResponse<Regency[]>>('/api/v1/program-setup/regencies') });
  useEffect(() => { if (!programID && programs.data?.data[0]) setProgramID(programs.data.data[0].id); }, [programID, programs.data]);
  useEffect(() => { setSelected(new Set()); setPending({}); setZoneOrder(programID ? readZoneOrder(programID) : []); }, [programID]);
  const zonesKey = ['program-setup', 'programs', programID, 'zones'];
  const zones = useQuery({ queryKey: zonesKey, queryFn: () => apiRequest<DataResponse<ProgramZone[]>>(`/api/v1/program-setup/programs/${programID}/zones`), enabled: Boolean(programID) });
  const refresh = () => Promise.all([client.invalidateQueries({ queryKey: zonesKey }), client.invalidateQueries({ queryKey: ['bast', 'zone-po'] })]);

  const zoneItems = useMemo(() => (zones.data?.data ?? []).map((zone) => ({ ...zone, regencies: zone.regencies ?? [] })), [zones.data]);
  const placeholder = zoneItems.find((zone) => zone.is_placeholder);
  // Bawaan: ZONA 1, ZONA 2, ... (urutan nama alami); urutan yang digeser pengguna didahulukan.
  const realZones = useMemo(() => {
    const zonesOnly = zoneItems.filter((zone) => !zone.is_placeholder).sort((a, b) => naturalName.compare(a.name, b.name));
    const rank = (id: string) => { const index = zoneOrder.indexOf(id); return index < 0 ? Number.MAX_SAFE_INTEGER : index; };
    return zonesOnly.sort((a, b) => rank(a.id) - rank(b.id));
  }, [zoneItems, zoneOrder]);
  const allRegencies = useMemo(() => {
    const byID = new Map<string, Regency>();
    for (const regency of regencies.data?.data ?? []) byID.set(regency.id, regency);
    for (const zone of zoneItems) for (const regency of zone.regencies) if (!byID.has(regency.id)) byID.set(regency.id, regency);
    return [...byID.values()].sort((a, b) => a.name.localeCompare(b.name, 'id'));
  }, [regencies.data, zoneItems]);
  // Kabupaten di zona placeholder atau belum ditempatkan masuk kolom "Belum ada zona".
  const columnOf = useMemo(() => {
    const map = new Map<string, string>();
    for (const zone of realZones) for (const regency of zone.regencies) map.set(regency.id, zone.id);
    for (const [regencyID, columnID] of Object.entries(pending)) map.set(regencyID, columnID);
    return (regencyID: string) => map.get(regencyID) ?? unassignedColumn;
  }, [realZones, pending]);
  const query = search.trim().toLocaleLowerCase('id-ID');
  const matches = (regency: Regency) => !query || `${regency.document_code} ${regency.name}`.toLocaleLowerCase('id-ID').includes(query);
  const columns = [
    { id: unassignedColumn, name: 'Belum ada zona', code: '', zone: undefined as ProgramZone | undefined },
    ...realZones.map((zone) => ({ id: zone.id, name: zone.name, code: zone.code, zone: zone as ProgramZone | undefined })),
  ].map((column) => ({ ...column, regencies: allRegencies.filter((regency) => columnOf(regency.id) === column.id) }));
  const unassignedCount = columns[0].regencies.length;

  const move = useMutation({
    mutationFn: async ({ regencyIDs, columnID }: { regencyIDs: string[]; columnID: string }) => {
      const zoneID = columnID === unassignedColumn ? placeholder?.id : columnID;
      if (!zoneID) throw new Error('Zona placeholder program belum tersedia.');
      await Promise.all(regencyIDs.map((regencyID) => apiRequest(`/api/v1/program-setup/programs/${programID}/assignments/${regencyID}`, { method: 'PUT', body: JSON.stringify({ zone_id: zoneID }) })));
    },
    onMutate: ({ regencyIDs, columnID }) => setPending((current) => ({ ...current, ...Object.fromEntries(regencyIDs.map((id) => [id, columnID])) })),
    onSuccess: (_, { regencyIDs, columnID }) => { setSelected(new Set()); toast.success(`${regencyIDs.length} kabupaten dipindahkan ke ${columns.find((column) => column.id === columnID)?.name ?? 'zona'}.`); },
    onError: (error) => toast.error(errorMessage(error, 'Kabupaten belum dapat dipindahkan.')),
    onSettled: async () => { await refresh(); setPending({}); },
  });
  const moveTo = (regencyIDs: string[], columnID: string) => {
    const changed = regencyIDs.filter((id) => columnOf(id) !== columnID);
    if (changed.length) move.mutate({ regencyIDs: changed, columnID });
  };
  const toggle = (regencyID: string) => setSelected((current) => {
    const next = new Set(current);
    if (next.has(regencyID)) next.delete(regencyID);
    else next.add(regencyID);
    return next;
  });

  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }), useSensor(KeyboardSensor));
  const reorderColumn = (zoneID: string, overID: string) => {
    const ids = realZones.map((zone) => zone.id).filter((id) => id !== zoneID);
    const target = overID === unassignedColumn ? 0 : ids.indexOf(overID);
    if (target < 0) return;
    // Taruh di posisi kolom tujuan; geser ke kanan bila asalnya di kiri tujuan.
    const from = realZones.findIndex((zone) => zone.id === zoneID);
    const to = overID === unassignedColumn ? 0 : from <= target ? target + 1 : target;
    ids.splice(to, 0, zoneID);
    setZoneOrder(ids);
    writeZoneOrder(programID, ids);
  };
  const onDragStart = (event: DragStartEvent) => setDragging(String(event.active.id));
  const onDragEnd = (event: DragEndEvent) => {
    setDragging(null);
    if (!event.over) return;
    const activeID = String(event.active.id);
    if (activeID.startsWith(columnDragPrefix)) {
      reorderColumn(activeID.slice(columnDragPrefix.length), String(event.over.id));
      return;
    }
    const regencyID = activeID;
    // Menyeret kabupaten yang sedang dicentang memindahkan seluruh pilihan.
    moveTo(selected.has(regencyID) ? [...selected] : [regencyID], String(event.over.id));
  };
  const draggedCount = dragging && selected.has(dragging) ? selected.size : 1;
  const draggedRegency = allRegencies.find((regency) => regency.id === dragging);
  const draggedColumn = dragging?.startsWith(columnDragPrefix) ? realZones.find((zone) => columnDragPrefix + zone.id === dragging) : undefined;
  const selectedProgram = programs.data?.data.find((item) => item.id === programID);

  return <section className="setupPanel" aria-label="Zona program">
    <div className="panelHeading"><div><h2><MapPinned />Zona tender</h2><p>Kelompokkan kabupaten per zona untuk struktur folder Drive dan No. PO.</p></div></div>
    <div className="grid gap-4">
      <div className="overflow-hidden rounded-xl border bg-card">
        <div className="flex flex-wrap items-center gap-3 p-3">
          <div className="min-w-64 flex-1 sm:max-w-md">
            <Select value={programID} onValueChange={(value) => setProgramID(value ?? '')}><SelectTrigger className="w-full font-medium" aria-label="Program"><SelectValue placeholder="Pilih program" /></SelectTrigger><SelectContent>{programs.data?.data.map((program) => <SelectItem key={program.id} value={program.id}>{program.name}</SelectItem>)}</SelectContent></Select>
          </div>
          {selectedProgram && <div className="flex gap-1.5"><Badge>{programTypeLabel(selectedProgram.program_type)}</Badge><Badge variant="outline">{selectedProgram.code}</Badge></div>}
          <div className="ml-auto flex flex-wrap items-center gap-2">
            {zones.data && <>
              <span className="rounded-full bg-muted px-2.5 py-1 text-xs"><strong>{realZones.length}</strong> zona</span>
              <span className="rounded-full bg-muted px-2.5 py-1 text-xs"><strong>{allRegencies.length - unassignedCount}</strong>/{allRegencies.length} kabupaten berzona</span>
              {unassignedCount > 0 && <span className="inline-flex items-center gap-1 rounded-full bg-amber-100 px-2.5 py-1 text-xs font-medium text-amber-900 dark:bg-amber-950/40 dark:text-amber-100" title="Upload dokumen untuk kabupaten tersebut ditahan sampai zona dipilih."><AlertTriangle className="size-3.5" aria-hidden="true" />{unassignedCount} kabupaten belum memiliki zona</span>}
            </>}
            {canManage && <Button disabled={!programID} aria-expanded={adding} onClick={() => setAdding((value) => !value)}><Plus />Tambah zona</Button>}
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-3 border-t bg-muted/30 px-3 py-2">
          <div className="relative min-w-56 flex-1 sm:max-w-sm">
            <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
            <Input aria-label="Cari kabupaten" className="h-9 bg-card pl-9" placeholder="Cari kode atau nama kabupaten" value={search} onChange={(e) => setSearch(e.target.value)} />
          </div>
          {canManage && <p className="flex items-center gap-1 text-xs text-muted-foreground"><GripVertical className="size-3.5" aria-hidden="true" />Seret kabupaten antar zona, atau centang beberapa lalu pindahkan sekaligus. Seret judul zona untuk mengubah urutan kolom.</p>}
        </div>
        {canManage && adding && programID && <div className="border-t p-3"><AddZoneForm programID={programID} sortOrder={Math.max(0, ...zoneItems.map((zone) => zone.sort_order ?? 0)) + 1} onDone={() => { setAdding(false); refresh(); }} onCancel={() => setAdding(false)} /></div>}
      </div>

      {zones.isPending ? <DataState kind="loading" title="Memuat zona" description="Mengambil susunan zona dan kabupaten." /> : zones.isError ? <DataState kind="error" title="Zona belum dapat dimuat" description="Periksa koneksi lalu coba kembali." action={{ label: 'Coba lagi', onClick: () => zones.refetch() }} /> : <>
        <DndContext sensors={sensors} onDragStart={onDragStart} onDragEnd={onDragEnd} onDragCancel={() => setDragging(null)}>
          <div className="flex gap-3 overflow-x-auto pb-2" role="list" aria-label="Papan zona">
            {columns.map((column) => <ZoneColumn key={column.id} id={column.id} name={column.name} code={column.code} zone={column.zone} programID={programID}
              regencies={column.regencies} visible={column.regencies.filter(matches)} selected={selected} canManage={canManage} onToggle={toggle} onChanged={refresh} />)}
          </div>
          <DragOverlay>{draggedColumn && <div className="flex w-64 items-center gap-2 rounded-xl border bg-card px-3 py-3 text-sm font-semibold shadow-lg"><GripVertical className="size-4 text-muted-foreground" />{draggedColumn.name}<Badge variant="outline" className="ml-auto">{draggedColumn.regencies.length}</Badge></div>}{draggedRegency && <div className="flex items-center gap-2 rounded-md border bg-card px-2.5 py-1.5 text-sm shadow-lg"><GripVertical className="size-3.5 text-muted-foreground" /><span className="font-mono text-xs text-muted-foreground">{draggedRegency.document_code}</span>{draggedRegency.name}{draggedCount > 1 && <Badge>+{draggedCount - 1}</Badge>}</div>}</DragOverlay>
        </DndContext>
        {canManage && selected.size > 0 && <div className="sticky bottom-3 z-10 flex flex-wrap items-center gap-3 rounded-lg border bg-card p-3 shadow-lg">
          <strong className="text-sm">{selected.size} kabupaten dipilih</strong>
          <Select value="" onValueChange={(value) => value && moveTo([...selected], value)}>
            <SelectTrigger className="w-56" aria-label="Pindahkan ke zona"><SelectValue placeholder="Pindahkan ke..." /></SelectTrigger>
            <SelectContent>{columns.map((column) => <SelectItem key={column.id} value={column.id}>{column.name}</SelectItem>)}</SelectContent>
          </Select>
          <Button variant="ghost" onClick={() => setSelected(new Set())}><X />Batal</Button>
        </div>}
      </>}
      {canViewPO && <ZonePOPanel programID={programID} canManage={canManagePO} />}
    </div>
  </section>;
}

type ColumnProps = { id: string; name: string; code: string; zone?: ProgramZone; programID: string; regencies: Regency[]; visible: Regency[]; selected: Set<string>; canManage: boolean; onToggle: (id: string) => void; onChanged: () => void };

function ZoneColumn({ id, name, code, zone, programID, regencies, visible, selected, canManage, onToggle, onChanged }: ColumnProps) {
  const { setNodeRef, isOver } = useDroppable({ id, disabled: !canManage });
  const drag = useDraggable({ id: columnDragPrefix + id, disabled: !canManage || !zone });
  const [editing, setEditing] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [draftName, setDraftName] = useState(name);
  const rename = useMutation({
    mutationFn: () => apiRequest(`/api/v1/program-setup/programs/${programID}/zones/${id}`, { method: 'PATCH', body: JSON.stringify({ code: zone?.code, name: draftName, sort_order: zone?.sort_order ?? 0 }) }),
    onSuccess: () => { setEditing(false); onChanged(); toast.success('Nama zona diperbarui.'); },
    onError: (error) => toast.error(errorMessage(error, 'Nama zona belum dapat disimpan.')),
  });
  const remove = useMutation({
    mutationFn: () => apiRequest(`/api/v1/program-setup/programs/${programID}/zones/${id}`, { method: 'DELETE' }),
    onSuccess: () => { setConfirmDelete(false); onChanged(); toast.success(regencies.length ? `${name} dihapus; ${regencies.length} kabupaten kembali ke Belum ada zona.` : `${name} dihapus.`); },
    onError: (error) => toast.error(errorMessage(error, 'Zona belum dapat dihapus.')),
  });
  const unassigned = id === unassignedColumn;

  return <div ref={setNodeRef} role="listitem" aria-label={`${name}, ${regencies.length} kabupaten`}
    className={`flex max-h-128 min-w-72 flex-1 basis-72 flex-col rounded-xl border transition-colors ${isOver ? 'border-primary bg-primary/5' : unassigned ? 'border-dashed bg-muted/30' : 'bg-card'} ${drag.isDragging ? 'opacity-40' : ''}`}>
    <div className="flex min-h-12 items-center gap-1.5 border-b px-3 py-2">
      {editing
        ? <form className="flex min-w-0 flex-1 gap-1" onSubmit={(e) => { e.preventDefault(); rename.mutate(); }}>
          <Input autoFocus aria-label={`Nama ${name}`} className="h-8" value={draftName} onChange={(e) => setDraftName(uppercaseBusinessText(e.target.value))} />
          <Button type="submit" size="icon-sm" aria-label="Simpan nama zona" disabled={!draftName.trim() || rename.isPending}><Check /></Button>
          <Button type="button" variant="ghost" size="icon-sm" aria-label="Batal ubah nama" onClick={() => { setEditing(false); setDraftName(name); }}><X /></Button>
        </form>
        : <>
          {canManage && zone && <button type="button" ref={drag.setNodeRef} className="-ml-1 cursor-grab touch-none rounded p-0.5 text-muted-foreground hover:bg-muted active:cursor-grabbing" aria-label={`Geser urutan ${name}`} {...drag.attributes} {...drag.listeners}><GripVertical className="size-4" /></button>}
          <div className="min-w-0 flex-1"><strong className={`block truncate text-sm ${unassigned ? 'text-muted-foreground' : ''}`}>{name}</strong>{code && <span className="block truncate text-[11px] text-muted-foreground">{code}</span>}</div>
          <Badge variant={unassigned && regencies.length > 0 ? 'secondary' : 'outline'}>{regencies.length}</Badge>
          {canManage && zone && <>
            <Button variant="ghost" size="icon-sm" aria-label={`Ubah nama ${name}`} onClick={() => { setDraftName(name); setEditing(true); }}><Pencil /></Button>
            <Button variant="ghost" size="icon-sm" className="text-destructive" aria-label={`Hapus ${name}`} disabled={remove.isPending} onClick={() => setConfirmDelete(true)}><Trash2 /></Button>
          </>}
        </>}
    </div>
    <ul className="grid flex-1 content-start gap-1.5 overflow-x-hidden overflow-y-auto p-2" aria-label={`Kabupaten di ${name}`}>
      {visible.map((regency) => <RegencyChip key={regency.id} regency={regency} selected={selected.has(regency.id)} canManage={canManage} onToggle={onToggle} />)}
      {regencies.length === 0 && <li className="rounded-md border border-dashed p-3 text-center text-xs text-muted-foreground">{canManage ? 'Seret kabupaten ke sini' : 'Belum ada kabupaten.'}</li>}
      {regencies.length > 0 && visible.length === 0 && <li className="p-2 text-center text-xs text-muted-foreground">Tidak ada yang cocok.</li>}
    </ul>
    <AlertDialog open={confirmDelete} onOpenChange={setConfirmDelete}>
      <AlertDialogContent>
        <AlertDialogHeader><AlertDialogTitle>Hapus {name}?</AlertDialogTitle><AlertDialogDescription>{regencies.length ? `${regencies.length} kabupaten di zona ini akan dipindah ke Belum ada zona, dan No. PO zona ini ikut terhapus.` : 'Zona kosong ini akan dihapus beserta No. PO-nya.'}</AlertDialogDescription></AlertDialogHeader>
        <AlertDialogFooter><AlertDialogCancel disabled={remove.isPending}>Batal</AlertDialogCancel><AlertDialogAction disabled={remove.isPending} onClick={() => remove.mutate()}>{remove.isPending ? 'Menghapus...' : 'Hapus zona'}</AlertDialogAction></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>;
}

function RegencyChip({ regency, selected, canManage, onToggle }: { regency: Regency; selected: boolean; canManage: boolean; onToggle: (id: string) => void }) {
  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({ id: regency.id, disabled: !canManage });
  return <li ref={setNodeRef} className={`flex min-w-0 items-center gap-2 rounded-md border px-2 py-1.5 text-sm ${selected ? 'border-primary bg-primary/10' : 'bg-background'} ${isDragging ? 'opacity-40' : ''}`}>
    {canManage && <Checkbox checked={selected} aria-label={`Pilih ${regency.name}`} onCheckedChange={() => onToggle(regency.id)} />}
    <span className="font-mono text-[11px] text-muted-foreground">{regency.document_code}</span>
    <span className="min-w-0 flex-1 truncate" title={regency.name}>{regency.name}</span>
    {canManage && <button type="button" className="cursor-grab touch-none rounded p-0.5 text-muted-foreground hover:bg-muted active:cursor-grabbing" aria-label={`Seret ${regency.name}`} {...attributes} {...listeners}><GripVertical className="size-3.5" /></button>}
  </li>;
}

function AddZoneForm({ programID, sortOrder, onDone, onCancel }: { programID: string; sortOrder: number; onDone: () => void; onCancel: () => void }) {
  const [code, setCode] = useState('');
  const [name, setName] = useState('');
  const save = useMutation({
    mutationFn: () => apiRequest(`/api/v1/program-setup/programs/${programID}/zones`, { method: 'POST', body: JSON.stringify({ code, name, sort_order: sortOrder }) }),
    onSuccess: () => { toast.success('Zona berhasil ditambahkan.'); onDone(); },
    onError: (error) => toast.error(errorMessage(error, 'Zona belum dapat disimpan.')),
  });
  const submit = (event: FormEvent) => { event.preventDefault(); save.mutate(); };
  return <form className="flex flex-wrap items-end gap-3" onSubmit={submit}>
    <div className="grid w-40 gap-1.5"><Label htmlFor="zone-code">Kode zona</Label><Input id="zone-code" value={code} onChange={(event) => setCode(event.target.value.toUpperCase())} placeholder="ZONA-3" required /></div>
    <div className="grid min-w-48 flex-1 gap-1.5 sm:max-w-xs"><Label htmlFor="zone-name">Nama zona</Label><Input id="zone-name" value={name} onChange={(event) => setName(uppercaseBusinessText(event.target.value))} placeholder="ZONA 3" required /></div>
    <Button type="submit" disabled={save.isPending}><Check />{save.isPending ? 'Menyimpan...' : 'Simpan zona'}</Button>
    <Button type="button" variant="ghost" onClick={onCancel}>Batal</Button>
  </form>;
}
