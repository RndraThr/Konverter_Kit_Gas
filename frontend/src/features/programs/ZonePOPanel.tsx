import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ReceiptText, RotateCcw, Save } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { toast } from 'sonner';
import { DataState } from '@/components/DataState';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ApiError, apiRequest } from '@/lib/api';
import type { ZonePO } from '../berita-acara/types';
import type { DataResponse } from './types';

const rowKey = (kind: string, code: string) => `${kind}|${code}`;
const cellKey = (zoneID: string, key: string) => `${zoneID}::${key}`;
const kindSuffix: Record<string, string> = { hose_suction: ' (hisap)', hose_discharge: ' (buang)' };

/** No. PO dalam satu tabel: baris = barang·merk, kolom = zona. Satu tombol simpan untuk semua zona. */
export function ZonePOPanel({ programID, canManage }: { programID: string; canManage: boolean }) {
  const client = useQueryClient();
  const query = useQuery({
    queryKey: ['bast', 'zone-po', programID],
    queryFn: () => apiRequest<DataResponse<ZonePO[]>>(`/api/v1/bast/items/zone-po?program_id=${encodeURIComponent(programID)}`),
    enabled: Boolean(programID),
  });
  const zones = useMemo(() => query.data?.data ?? [], [query.data]);
  const saved = useMemo(() => Object.fromEntries(zones.flatMap((zone) => zone.rows.map((row) => [cellKey(zone.zone_id, rowKey(row.kind, row.code)), row.po_number]))), [zones]);
  const [values, setValues] = useState<Record<string, string>>({});
  useEffect(() => setValues(saved), [saved]);
  // Baris gabungan semua zona, urut sesuai kemunculan (urutan form BA Pemeriksaan).
  const rows = useMemo(() => {
    const seen = new Map<string, { key: string; label: string; brand: string }>();
    for (const zone of zones) for (const row of zone.rows) {
      const key = rowKey(row.kind, row.code);
      if (!seen.has(key)) seen.set(key, { key, label: row.name + (kindSuffix[row.kind] ?? ''), brand: row.brand });
    }
    return [...seen.values()];
  }, [zones]);
  const zoneRows = useMemo(() => new Map(zones.map((zone) => [zone.zone_id, new Set(zone.rows.map((row) => rowKey(row.kind, row.code)))])), [zones]);
  const dirtyZones = zones.filter((zone) => zone.rows.some((row) => { const key = cellKey(zone.zone_id, rowKey(row.kind, row.code)); return (values[key] ?? '') !== (saved[key] ?? ''); }));

  const save = useMutation({
    mutationFn: () => Promise.all(dirtyZones.map((zone) => apiRequest('/api/v1/bast/items/zone-po', { method: 'PUT', body: JSON.stringify({
      program_id: programID, zone_id: zone.zone_id,
      po_numbers: zone.rows.map((row) => ({ kind: row.kind, code: row.code, po_number: values[cellKey(zone.zone_id, rowKey(row.kind, row.code))] ?? '' })),
    }) }))),
    onSuccess: () => { client.invalidateQueries({ queryKey: ['bast'] }); toast.success(`No. PO ${dirtyZones.map((zone) => zone.zone_name).join(', ')} tersimpan.`); },
    onError: (error) => toast.error(error instanceof ApiError || error instanceof Error ? error.message : 'No. PO belum dapat disimpan.'),
  });

  if (!programID) return null;
  if (query.isPending) return <DataState kind="loading" title="Memuat No. PO" description="Mengambil barang dan merk per zona." />;
  if (query.isError) return <DataState kind="error" title="No. PO belum dapat dimuat" description="Periksa koneksi lalu coba kembali." action={{ label: 'Coba lagi', onClick: () => query.refetch() }} />;

  return <section className="grid gap-3 rounded-xl border bg-card p-4" aria-label="No. PO per zona">
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div><h3 className="flex items-center gap-2 text-sm font-semibold"><ReceiptText className="size-4 text-primary" aria-hidden="true" />No. PO per zona</h3><p className="mt-0.5 text-xs text-muted-foreground">Dipakai BA Pemeriksaan sesuai merk barang di kabupaten. Barang mengikuti form BA Pemeriksaan; merk dari Template Paket jadwal di zona.</p></div>
      {canManage && rows.length > 0 && <div className="flex gap-2">
        <Button variant="ghost" size="sm" disabled={dirtyZones.length === 0 || save.isPending} onClick={() => setValues(saved)}><RotateCcw />Batal</Button>
        <Button size="sm" disabled={dirtyZones.length === 0 || save.isPending} onClick={() => save.mutate()}><Save />{save.isPending ? 'Menyimpan...' : dirtyZones.length ? `Simpan (${dirtyZones.length} zona)` : 'Simpan No. PO'}</Button>
      </div>}
    </div>
    {zones.length === 0 ? <p className="text-sm text-muted-foreground">Tambahkan zona terlebih dahulu.</p>
      : rows.length === 0 ? <p className="text-sm text-muted-foreground">Belum ada jadwal dengan template paket.</p>
      : <div className="overflow-x-auto rounded-lg border">
        <table className="w-full border-collapse text-sm">
          <thead className="bg-muted/50 text-xs">
            <tr>
              <th scope="col" className="sticky left-0 z-10 min-w-56 bg-muted px-3 py-2 text-left font-semibold">Barang · merk</th>
              {zones.map((zone) => {
                const filled = zone.rows.filter((row) => (values[cellKey(zone.zone_id, rowKey(row.kind, row.code))] ?? '').trim() !== '').length;
                return <th scope="col" key={zone.zone_id} className="min-w-44 px-2 py-2 text-left font-semibold"><div className="flex items-center justify-between gap-2"><span>{zone.zone_name}</span><Badge variant={filled === zone.rows.length ? 'default' : 'outline'}>{filled}/{zone.rows.length}</Badge></div></th>;
              })}
            </tr>
          </thead>
          <tbody>{rows.map((row) => <tr key={row.key} className="border-t">
            <th scope="row" className="sticky left-0 z-10 bg-card px-3 py-1.5 text-left font-normal"><span className="font-medium">{row.label}</span><span className="text-muted-foreground"> · {row.brand}</span></th>
            {zones.map((zone) => {
              const key = cellKey(zone.zone_id, row.key);
              if (!zoneRows.get(zone.zone_id)?.has(row.key)) return <td key={zone.zone_id} className="px-2 py-1.5 text-center text-xs text-muted-foreground">—</td>;
              const dirty = (values[key] ?? '') !== (saved[key] ?? '');
              return <td key={zone.zone_id} className="px-2 py-1.5">
                <input aria-label={`No. PO ${row.label} ${row.brand} ${zone.zone_name}`} className={`h-8 w-full min-w-36 rounded-md border bg-transparent px-2 text-sm outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/40 disabled:opacity-60 ${dirty ? 'border-primary bg-primary/5' : 'border-input'}`}
                  value={values[key] ?? ''} disabled={!canManage} placeholder="No. PO" onChange={(e) => setValues((current) => ({ ...current, [key]: e.target.value }))} />
              </td>;
            })}
          </tr>)}</tbody>
        </table>
      </div>}
  </section>;
}
