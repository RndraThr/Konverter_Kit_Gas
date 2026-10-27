import { useMutation, useQueryClient } from '@tanstack/react-query';
import { MapPin, Plus, Save, Trash2 } from 'lucide-react';
import { useMemo, useState } from 'react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import { ApiError, apiRequest } from '@/lib/api';
import type { HandoverArea, HandoverView } from './types';

type DataResponse<T> = { data: T };
type Draft = { key: string; id?: string; name: string; areas: HandoverArea[] };

export const handoverQueryKey = (scheduleID: string) => ['bast', 'closing-titik-serah', 'handover', scheduleID];

const selectClass = 'h-8 w-full rounded-md border border-input bg-transparent px-2 text-sm shadow-xs outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50';

const sourceLabel: Record<string, string> = { override: 'manual', village: 'desa', district: 'kecamatan', single: 'satu lokasi' };

/**
 * Pengaturan titik serah BA Serah Terima: daftar lokasi beserta cakupan
 * kecamatan/desa, dan pemindahan manual penerima tertentu.
 */
export function HandoverLocationsEditor({ scheduleID, view, onClose }: { scheduleID: string; view: HandoverView; onClose: () => void }) {
  const queryClient = useQueryClient();
  const [drafts, setDrafts] = useState<Draft[]>(() => view.locations.length > 0
    ? view.locations.map((location) => ({ key: location.id, id: location.id, name: location.name, areas: location.areas }))
    : [{ key: 'new-0', name: '', areas: [] }]);
  const [search, setSearch] = useState('');
  const [onlyUnassigned, setOnlyUnassigned] = useState(false);

  const districts = useMemo(() => {
    const groups = new Map<string, { village: string; recipients: number }[]>();
    for (const area of view.areas) groups.set(area.district, [...(groups.get(area.district) ?? []), { village: area.village, recipients: area.recipients }]);
    return [...groups.entries()];
  }, [view.areas]);

  const refresh = (data: HandoverView) => {
    queryClient.setQueryData(handoverQueryKey(scheduleID), { data });
    void queryClient.invalidateQueries({ queryKey: ['bast', 'closing-titik-serah'] });
    void queryClient.invalidateQueries({ queryKey: ['bast', 'closing-kabupaten'] });
  };
  const save = useMutation({
    mutationFn: () => apiRequest<DataResponse<HandoverView>>('/api/v1/bast/handover-locations', { method: 'PUT', body: JSON.stringify({ schedule_id: scheduleID, locations: drafts.map(({ id, name, areas }) => ({ id: id ?? '', name, areas })) }) }),
    onSuccess: (response) => {
      refresh(response.data);
      setDrafts(response.data.locations.map((location) => ({ key: location.id, id: location.id, name: location.name, areas: location.areas })));
      toast.success('Titik serah tersimpan.');
    },
    onError: (error) => toast.error(errorMessage(error)),
  });
  const move = useMutation({
    mutationFn: ({ slotID, locationID }: { slotID: string; locationID: string }) => apiRequest<DataResponse<HandoverView>>('/api/v1/bast/handover-locations/override', { method: 'PUT', body: JSON.stringify({ schedule_id: scheduleID, slot_id: slotID, location_id: locationID }) }),
    onSuccess: (response) => refresh(response.data),
    onError: (error) => toast.error(errorMessage(error)),
  });

  const update = (key: string, change: Partial<Draft>) => setDrafts((items) => items.map((item) => item.key === key ? { ...item, ...change } : item));
  const hasArea = (draft: Draft, district: string, village = '') => draft.areas.some((area) => area.district === district && (area.village ?? '') === village);
  const toggleArea = (draft: Draft, district: string, village = '') => {
    const exists = hasArea(draft, district, village);
    update(draft.key, { areas: exists ? draft.areas.filter((area) => !(area.district === district && (area.village ?? '') === village)) : [...draft.areas, village ? { district, village } : { district }] });
  };
  const owner = (district: string, village = '') => drafts.find((draft) => hasArea(draft, district, village));

  const needle = search.trim().toLowerCase();
  const recipients = view.recipients.filter((recipient) => (!onlyUnassigned || !recipient.location_id)
    && (needle === '' || `${recipient.name} ${recipient.village} ${recipient.district} ${recipient.slot_number}`.toLowerCase().includes(needle)));
  const saved = drafts.every((draft) => draft.id) && view.locations.length === drafts.length;

  return <div className="grid gap-5 rounded-xl border bg-card p-4 shadow-xs">
    <div className="flex flex-wrap items-center justify-between gap-2">
      <div><strong className="text-base">Atur titik serah</strong><p className="text-xs text-muted-foreground">Penerima masuk ke titik serah sesuai desa, lalu kecamatan. Bila hanya ada satu titik serah, semua penerima masuk ke sana.</p></div>
      <Button variant="ghost" onClick={onClose}>Tutup</Button>
    </div>

    <div className="grid gap-3">
      {drafts.map((draft, index) => <div key={draft.key} className="grid gap-3 rounded-lg border p-3">
        <div className="flex items-center gap-2">
          <MapPin className="size-4 shrink-0 text-primary" aria-hidden="true" />
          <Input aria-label={`Nama titik serah ${index + 1}`} placeholder="Contoh: Balai Desa Tempe" value={draft.name} onChange={(event) => update(draft.key, { name: event.target.value })} />
          <Button variant="ghost" size="icon" aria-label={`Hapus titik serah ${index + 1}`} disabled={drafts.length === 1} onClick={() => setDrafts((items) => items.filter((item) => item.key !== draft.key))}><Trash2 /></Button>
        </div>
        {districts.length > 0 && drafts.length > 1 && <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
          {districts.map(([district, villages]) => {
            const whole = owner(district);
            return <div key={district} className="grid gap-1 rounded-md border p-2 text-sm">
              <label className="flex items-center gap-2 font-medium"><Checkbox checked={hasArea(draft, district)} disabled={Boolean(whole && whole.key !== draft.key)} onCheckedChange={() => toggleArea(draft, district)} />Kec. {district}{whole && whole.key !== draft.key && <span className="text-xs font-normal text-muted-foreground">({whole.name || 'lokasi lain'})</span>}</label>
              {villages.filter((item) => item.village).map((item) => {
                const taken = owner(district, item.village);
                return <label key={item.village} className="ml-5 flex items-center gap-2 text-xs"><Checkbox checked={hasArea(draft, district, item.village)} disabled={Boolean(taken && taken.key !== draft.key)} onCheckedChange={() => toggleArea(draft, district, item.village)} />{item.village}<span className="text-muted-foreground">· {item.recipients}</span>{taken && taken.key !== draft.key && <span className="text-muted-foreground">({taken.name || 'lokasi lain'})</span>}</label>;
              })}
            </div>;
          })}
        </div>}
      </div>)}
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Button variant="outline" onClick={() => setDrafts((items) => [...items, { key: `new-${Date.now()}`, name: '', areas: [] }])}><Plus />Tambah titik serah</Button>
        <Button disabled={save.isPending || drafts.some((draft) => draft.name.trim() === '')} onClick={() => save.mutate()}><Save />{save.isPending ? 'Menyimpan...' : 'Simpan titik serah'}</Button>
      </div>
      <p className="text-xs text-muted-foreground">Mengubah titik serah menandai BA Serah Terima dan Closing Kabupaten yang sudah final perlu dibuat ulang.</p>
    </div>

    {view.recipients.length > 0 && <div className="grid gap-2 border-t pt-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <strong className="text-sm">Pindahkan penerima</strong>
        <div className="flex flex-wrap items-center gap-3">
          <label className="flex items-center gap-2 text-sm"><Checkbox checked={onlyUnassigned} onCheckedChange={(value) => setOnlyUnassigned(Boolean(value))} />Hanya yang belum ada titik serah</label>
          <Input className="h-8 w-56" aria-label="Cari penerima" placeholder="Cari nama, desa, nomor" value={search} onChange={(event) => setSearch(event.target.value)} />
        </div>
      </div>
      {!saved && <p className="text-xs text-amber-700 dark:text-amber-300">Simpan titik serah dulu sebelum memindahkan penerima.</p>}
      <div className="max-h-96 overflow-auto rounded-md border">
        <table className="w-full text-sm">
          <thead className="sticky top-0 bg-muted text-left text-xs"><tr><th className="px-2 py-1.5">No</th><th className="px-2 py-1.5">Penerima</th><th className="px-2 py-1.5">Desa / Kecamatan</th><th className="px-2 py-1.5">Titik serah</th></tr></thead>
          <tbody>{recipients.slice(0, 300).map((recipient) => <tr key={recipient.slot_id} className="border-t">
            <td className="px-2 py-1 tabular-nums">{recipient.slot_number}</td>
            <td className="px-2 py-1">{recipient.name}{!recipient.completed && <Badge variant="outline" className="ml-2">belum distribusi</Badge>}</td>
            <td className="px-2 py-1 text-muted-foreground">{recipient.village || '-'} / {recipient.district || '-'}</td>
            <td className="px-2 py-1">
              <select aria-label={`Titik serah ${recipient.name}`} className={selectClass} disabled={!saved || move.isPending}
                value={recipient.source === 'override' ? recipient.location_id ?? '' : ''}
                onChange={(event) => move.mutate({ slotID: recipient.slot_id, locationID: event.target.value })}>
                <option value="">{recipient.location_id && recipient.source !== 'override' ? `Otomatis: ${view.locations.find((location) => location.id === recipient.location_id)?.name ?? ''} (${sourceLabel[recipient.source ?? ''] ?? ''})` : 'Otomatis: belum ada titik serah'}</option>
                {view.locations.map((location) => <option key={location.id} value={location.id}>{location.name}</option>)}
              </select>
            </td>
          </tr>)}</tbody>
        </table>
      </div>
      {recipients.length > 300 && <p className="text-xs text-muted-foreground">Menampilkan 300 dari {recipients.length} penerima; persempit dengan pencarian.</p>}
    </div>}
  </div>;
}

function errorMessage(error: unknown) {
  return error instanceof ApiError || error instanceof Error ? error.message : 'Permintaan belum dapat diproses.';
}
