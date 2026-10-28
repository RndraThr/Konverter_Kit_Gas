import { useMutation, useQuery } from '@tanstack/react-query';
import { MapPin } from 'lucide-react';
import { useEffect, useId, useState } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { apiRequest } from '@/lib/api';
import type { DataResponse, DistributionSlot } from './types';

export type HandoverLocation = { id: string; name: string };

/** Comparison key of a place name: same as the server (textnorm.PlaceKey). */
export function placeKey(value: string | undefined) {
  return (value ?? '').toUpperCase().replace(/[^A-Z0-9]+/g, ' ').trim();
}

/** Similarity 0..1 of two place keys (edit distance), for "maybe you meant". */
export function placeSimilarity(a: string, b: string) {
  if (a === b) return 1;
  const rows = Array.from({ length: a.length + 1 }, (_, i) => [i, ...Array<number>(b.length).fill(0)]);
  for (let j = 1; j <= b.length; j++) rows[0][j] = j;
  for (let i = 1; i <= a.length; i++) {
    for (let j = 1; j <= b.length; j++) {
      rows[i][j] = Math.min(rows[i - 1][j] + 1, rows[i][j - 1] + 1, rows[i - 1][j - 1] + (a[i - 1] === b[j - 1] ? 0 : 1));
    }
  }
  return 1 - rows[a.length][b.length] / Math.max(a.length, b.length, 1);
}

/** The closest existing location to a typed new name, when close enough. */
export function similarLocation(name: string, locations: HandoverLocation[]) {
  const key = placeKey(name);
  if (key === '') return undefined;
  let best: HandoverLocation | undefined;
  let score = 0;
  for (const location of locations) {
    const value = placeSimilarity(key, placeKey(location.name));
    if (value > score) [best, score] = [location, value];
  }
  return score >= 0.75 && best && placeKey(best.name) !== key ? best : undefined;
}

const lastKey = (scheduleID: string) => `konkit.handover.last.${scheduleID}`;

function readLast(scheduleID: string) {
  try {
    return window.localStorage.getItem(lastKey(scheduleID)) ?? '';
  } catch {
    return '';
  }
}

function rememberLast(scheduleID: string, name: string) {
  try {
    window.localStorage.setItem(lastKey(scheduleID), name);
  } catch {
    // Only a convenience: the field still works without it.
  }
}

/**
 * Titik serah of a slot in POS Dokumen, beside the distribution date. The
 * list is shared by the whole regency; a new name creates a location, and a
 * name close to an existing one asks first. The last choice fills the next
 * slot of the same schedule.
 */
export function HandoverLocationField({ slot, editable, onSaved }: { slot: DistributionSlot; editable: boolean; onSaved: (slot: DistributionSlot) => void }) {
  const inputID = useId();
  const listID = useId();
  const locations = useQuery({
    queryKey: ['distribution', 'handover-locations', slot.schedule_id],
    queryFn: () => apiRequest<DataResponse<HandoverLocation[]>>(`/api/v1/distribution/handover-locations?schedule_id=${encodeURIComponent(slot.schedule_id)}`),
  });
  const items = (Array.isArray(locations.data?.data) ? locations.data.data : []).filter((item) => typeof item.name === 'string');
  const [name, setName] = useState(() => slot.handover_location_name || (editable ? readLast(slot.schedule_id) : ''));
  const [confirmedNew, setConfirmedNew] = useState(false);
  useEffect(() => {
    setName(slot.handover_location_name || (editable ? readLast(slot.schedule_id) : ''));
    setConfirmedNew(false);
  }, [slot.id, slot.handover_location_name, slot.schedule_id, editable]);

  const existing = items.find((item) => placeKey(item.name) === placeKey(name));
  const similar = existing ? undefined : similarLocation(name, items);
  const save = useMutation({
    mutationFn: (choice: { id?: string; name: string }) => apiRequest<DataResponse<DistributionSlot>>(
      `/api/v1/distribution/slots/${slot.slot_number}/handover-location?schedule_id=${encodeURIComponent(slot.schedule_id)}`,
      { method: 'PATCH', body: JSON.stringify({ handover_location_id: choice.id ?? '', handover_location_name: choice.name }) },
    ),
    onSuccess: ({ data }) => {
      rememberLast(slot.schedule_id, data.handover_location_name ?? '');
      void locations.refetch();
      onSaved(data);
    },
  });
  const saved = Boolean(slot.handover_location_id) && placeKey(slot.handover_location_name ?? '') === placeKey(name);
  const submit = () => save.mutate(existing ? { id: existing.id, name: existing.name } : { name });

  return <section aria-label="Titik serah" className="space-y-3 rounded-lg border p-4">
    <h4 className="flex items-center gap-2 font-medium"><MapPin className="size-4 text-primary" aria-hidden="true" />Titik serah</h4>
    <div className="flex flex-wrap items-end gap-3">
      <div className="grid min-w-52 flex-1 gap-2">
        <Label htmlFor={inputID}>Lokasi penyerahan</Label>
        <Input id={inputID} list={listID} value={name} disabled={!editable} placeholder="Cari atau ketik lokasi baru" onChange={(event) => { setName(event.target.value); setConfirmedNew(false); }} />
        <datalist id={listID}>{items.map((item) => <option key={item.id} value={item.name} />)}</datalist>
      </div>
      {editable && <Button type="button" variant="outline" disabled={placeKey(name) === '' || saved || save.isPending || (Boolean(similar) && !confirmedNew)} onClick={submit}>{existing || saved ? 'Simpan titik serah' : 'Tambah & simpan titik serah'}</Button>}
    </div>
    {editable && similar && !confirmedNew && <div className="flex flex-wrap items-center gap-2 rounded-md border border-amber-300/60 bg-amber-50 p-2 text-sm text-amber-950 dark:bg-amber-950/20 dark:text-amber-100">
      <span>Mungkin maksudnya <strong>{similar.name}</strong>?</span>
      <Button type="button" size="sm" onClick={() => { setName(similar.name); save.mutate({ id: similar.id, name: similar.name }); }}>Pakai {similar.name}</Button>
      <Button type="button" size="sm" variant="ghost" onClick={() => setConfirmedNew(true)}>Bukan, lokasi baru</Button>
    </div>}
    {!slot.handover_location_id && <p className="text-xs text-muted-foreground">Wajib diisi sebelum penyerahan diselesaikan. Daftar lokasi dipakai bersama semua petugas di kabupaten ini.</p>}
    {save.isError && <p className="text-sm text-destructive">{save.error instanceof Error ? save.error.message : 'Titik serah belum tersimpan.'}</p>}
  </section>;
}
