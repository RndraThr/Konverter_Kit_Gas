import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Check, GitMerge, MapPin, Pencil, Trash2, X } from 'lucide-react';
import { useState } from 'react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { ApiError, apiRequest } from '@/lib/api';
import type { HandoverView } from './types';

type DataResponse<T> = { data: T };

export const handoverQueryKey = (scheduleID: string) => ['bast', 'closing-titik-serah', 'handover', scheduleID];

const selectClass = 'h-8 w-full rounded-md border border-input bg-transparent px-2 text-sm shadow-xs outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50';

/**
 * Merapikan titik serah yang dicatat petugas di POS Dokumen: ubah nama (salah
 * ketik), gabungkan varian dobel ke lokasi yang benar, hapus lokasi tanpa
 * penerima, dan isi titik serah penerima yang belum punya. Daftar lokasi
 * dipakai bersama satu kabupaten.
 */
export function HandoverLocationsEditor({ scheduleID, view, onClose }: { scheduleID: string; view: HandoverView; onClose: () => void }) {
  const queryClient = useQueryClient();
  const [renaming, setRenaming] = useState<{ id: string; name: string } | null>(null);
  const [merging, setMerging] = useState<{ id: string; target: string } | null>(null);
  const base = `/api/v1/bast/handover-locations`;
  const query = `?schedule_id=${encodeURIComponent(scheduleID)}`;

  const refresh = (data: HandoverView) => {
    queryClient.setQueryData(handoverQueryKey(scheduleID), { data });
    void queryClient.invalidateQueries({ queryKey: ['bast', 'closing-titik-serah'] });
    void queryClient.invalidateQueries({ queryKey: ['bast', 'closing-kabupaten'] });
    void queryClient.invalidateQueries({ queryKey: ['distribution', 'handover-locations'] });
  };
  const action = useMutation({
    mutationFn: ({ path, method, body }: { path: string; method: string; body?: unknown }) =>
      apiRequest<DataResponse<HandoverView>>(`${base}${path}${query}`, { method, body: body === undefined ? undefined : JSON.stringify(body) }),
    onSuccess: (response, variables) => {
      refresh(response.data);
      setRenaming(null);
      setMerging(null);
      if (variables.method !== 'PUT') toast.success('Titik serah diperbarui.');
    },
    onError: (error) => toast.error(errorMessage(error)),
  });
  const busy = action.isPending;

  return <div className="grid gap-5 rounded-xl border bg-card p-4 shadow-xs">
    <div className="flex flex-wrap items-center justify-between gap-2">
      <div><strong className="text-base">Rapikan titik serah</strong><p className="text-xs text-muted-foreground">Titik serah dipilih petugas di POS Dokumen dan dipakai bersama satu kabupaten. Perbaiki salah ketik dengan Ubah nama, satukan varian dobel dengan Gabungkan.</p></div>
      <Button variant="ghost" onClick={onClose}>Tutup</Button>
    </div>

    <div className="grid gap-2">
      {view.locations.length === 0 && <p className="text-sm text-muted-foreground">Belum ada titik serah. Petugas menambahkannya saat mengisi POS Dokumen.</p>}
      {view.locations.map((location) => <div key={location.id} className="grid gap-2 rounded-lg border p-3 text-sm">
        {renaming?.id === location.id
          ? <div className="flex items-center gap-2">
            <Input aria-label={`Nama baru ${location.name}`} value={renaming.name} onChange={(event) => setRenaming({ id: location.id, name: event.target.value })} />
            <Button size="icon" aria-label="Simpan nama" disabled={busy || renaming.name.trim() === ''} onClick={() => action.mutate({ path: `/${location.id}`, method: 'PATCH', body: { name: renaming.name } })}><Check /></Button>
            <Button size="icon" variant="ghost" aria-label="Batal ubah nama" onClick={() => setRenaming(null)}><X /></Button>
          </div>
          : <div className="flex flex-wrap items-center justify-between gap-2">
            <span className="flex items-center gap-2 font-medium"><MapPin className="size-4 text-primary" aria-hidden="true" />{location.name}<span className="font-normal text-muted-foreground">· {location.assigned} penerima jadwal ini</span></span>
            <span className="flex flex-wrap gap-1">
              <Button size="sm" variant="ghost" disabled={busy} onClick={() => setRenaming({ id: location.id, name: location.name })}><Pencil />Ubah nama</Button>
              {view.locations.length > 1 && <Button size="sm" variant="ghost" disabled={busy} onClick={() => setMerging({ id: location.id, target: '' })}><GitMerge />Gabungkan</Button>}
              {location.assigned === 0 && <Button size="sm" variant="ghost" disabled={busy} onClick={() => action.mutate({ path: `/${location.id}`, method: 'DELETE' })}><Trash2 />Hapus</Button>}
            </span>
          </div>}
        {merging?.id === location.id && <div className="flex flex-wrap items-center gap-2 rounded-md bg-muted/50 p-2">
          <span>Pindahkan semua penerima "{location.name}" ke</span>
          <select aria-label={`Gabungkan ${location.name} ke`} className={`${selectClass} w-auto min-w-48`} value={merging.target} onChange={(event) => setMerging({ id: location.id, target: event.target.value })}>
            <option value="">Pilih lokasi yang benar</option>
            {view.locations.filter((item) => item.id !== location.id).map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
          </select>
          <Button size="sm" disabled={busy || merging.target === ''} onClick={() => action.mutate({ path: `/${location.id}/merge`, method: 'POST', body: { target_id: merging.target } })}>Gabungkan sekarang</Button>
          <Button size="sm" variant="ghost" onClick={() => setMerging(null)}>Batal</Button>
        </div>}
      </div>)}
      <p className="text-xs text-muted-foreground">Mengubah titik serah menandai BA Serah Terima dan Closing Kabupaten yang sudah final perlu dibuat ulang.</p>
    </div>

    {view.unassigned.length > 0 && <div className="grid gap-2 border-t pt-4">
      <strong className="text-sm">Penerima tanpa titik serah ({view.unassigned.length})</strong>
      <div className="max-h-96 overflow-auto rounded-md border">
        <table className="w-full text-sm">
          <thead className="sticky top-0 bg-muted text-left text-xs"><tr><th className="px-2 py-1.5">No</th><th className="px-2 py-1.5">Penerima</th><th className="px-2 py-1.5">Desa / Kecamatan</th><th className="px-2 py-1.5">Titik serah</th></tr></thead>
          <tbody>{view.unassigned.map((recipient) => <tr key={recipient.slot_id} className="border-t">
            <td className="px-2 py-1 tabular-nums">{recipient.slot_number}</td>
            <td className="px-2 py-1">{recipient.name}</td>
            <td className="px-2 py-1 text-muted-foreground">{recipient.village || '-'} / {recipient.district || '-'}</td>
            <td className="px-2 py-1">
              <select aria-label={`Titik serah ${recipient.name}`} className={selectClass} disabled={busy || view.locations.length === 0} value=""
                onChange={(event) => action.mutate({ path: '/assign', method: 'PUT', body: { slot_id: recipient.slot_id, location_id: event.target.value } })}>
                <option value="">Pilih titik serah</option>
                {view.locations.map((location) => <option key={location.id} value={location.id}>{location.name}</option>)}
              </select>
            </td>
          </tr>)}</tbody>
        </table>
      </div>
    </div>}
  </div>;
}

function errorMessage(error: unknown) {
  return error instanceof ApiError || error instanceof Error ? error.message : 'Permintaan belum dapat diproses.';
}
