import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Pencil, Save, TriangleAlert } from 'lucide-react';
import { useEffect, useState } from 'react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { apiRequest } from '@/lib/api';
import { cn } from '@/lib/utils';
import type { PemeriksaanFormSummary } from './types';

/**
 * No. PO filled per kabupaten (the LPG tank: each kabupaten has its own
 * loading order). Open and amber while empty; one line once filled, opened
 * again with Ubah. The other forms keep the zone's No. PO.
 */
export function KabupatenPOCard({ scheduleID, form, canManage }: { scheduleID: string; form: PemeriksaanFormSummary; canManage: boolean }) {
  const po = form.kabupaten_po!;
  const queryClient = useQueryClient();
  const [value, setValue] = useState(po.po_number);
  const [editing, setEditing] = useState(po.po_number === '');
  useEffect(() => { setValue(po.po_number); setEditing(po.po_number === ''); }, [po.po_number]);
  const save = useMutation({
    mutationFn: () => apiRequest('/api/v1/bast/pemeriksaan/kabupaten-po', {
      method: 'PUT', body: JSON.stringify({ schedule_id: scheduleID, item_kind: po.item_kind, item_code: po.item_code, po_number: value.trim() }),
    }),
    onSuccess: () => { void queryClient.invalidateQueries({ queryKey: ['bast', 'pemeriksaan'] }); toast.success(`No. PO ${form.title} tersimpan.`); },
    onError: (error) => toast.error(error.message),
  });
  const empty = po.po_number === '';
  if (!editing) {
    return <div className="flex items-center justify-between gap-2 rounded-lg border px-3 py-2 text-sm">
      <span className="min-w-0"><span className="text-muted-foreground">No. PO {form.title}: </span><b className="break-all">{po.po_number}</b></span>
      {canManage && <Button variant="ghost" size="sm" onClick={() => setEditing(true)}><Pencil />Ubah</Button>}
    </div>;
  }
  return <div className={cn('grid gap-2 rounded-lg border p-3', empty && 'border-amber-300 bg-amber-50 dark:border-amber-800 dark:bg-amber-950/30')}>
    <p className="flex items-center gap-2 text-sm font-medium">
      {empty && <TriangleAlert className="size-4 text-amber-600" />}No. PO {form.title}{empty ? ' belum diisi' : ''}
    </p>
    <p className="text-xs text-muted-foreground">Berbeda tiap kabupaten (mis. nomor LO dari Pertamina). Dicetak di BA Pemeriksaan {form.title}.</p>
    {canManage ? <form className="flex gap-2" onSubmit={(event) => { event.preventDefault(); save.mutate(); }}>
      <Input value={value} maxLength={120} placeholder="Nomor PO / LO" aria-label={`No. PO ${form.title}`} onChange={(event) => setValue(event.target.value)} />
      <Button type="submit" disabled={save.isPending || value.trim() === po.po_number}><Save />Simpan</Button>
      {!empty && <Button type="button" variant="ghost" onClick={() => { setValue(po.po_number); setEditing(false); }}>Batal</Button>}
    </form> : <p className="text-xs text-muted-foreground">Diisi oleh pengelola BA kabupaten.</p>}
  </div>;
}
