import { BellRing, CircleCheck, Copy } from 'lucide-react';
import { useMutation } from '@tanstack/react-query';
import { apiRequest } from '@/lib/api';
import { useCan } from '@/lib/permissions';
import { useState } from 'react';
import { toast } from 'sonner';
import { DataState } from '@/components/DataState';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { Label } from '@/components/ui/label';
import { serialFieldLabels, type Overview, type SerialDuplicate } from './types';
import type { UseQueryResult } from '@tanstack/react-query';

const formatTime = (value: string | null) => value ? new Date(value).toLocaleString('id-ID', { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' }) : '-';
const statusLabels: Record<string, string> = { open: 'Belum ada penerima', linked: 'Terhubung', completed: 'Selesai' };
const shortRegency = (name: string) => name.replace(/^KAB\.\s*/i, '').replace(/^KOTA\s+/i, 'Kota ');

/** The list as a message for the field group: per serial, where it was recorded and by whom. */
export function duplicatesMessage(list: SerialDuplicate[]): string {
  const lines = list.map((d, i) => `${i + 1}. ${serialFieldLabels[d.field]} ${d.serial}: ${d.entries.map((e) => `${shortRegency(e.regency_name)} #${e.slot_number}${e.entered_by ? ` (${e.entered_by})` : ''}`).join(' dan ')}`);
  return ['*Serial tercatat dobel*', 'Mohon dicocokkan dengan kartu barcode di box, lalu perbaiki serial yang tidak sesuai:', '', ...lines].join('\n');
}

/**
 * Equipment serials recorded on more than one slot. Saving is not blocked
 * (an earlier typo must not stop the team holding the real box), so the
 * central team follows the clashes here; a row leaves once one side is fixed.
 */
export function DuplicatesPanel({ overview, duplicates, userId, onOpen }: {
  overview: Overview; duplicates: UseQueryResult<SerialDuplicate[]>; userId: string; onOpen: (scheduleId: string, slot: number) => void;
}) {
  const mine = new Set(overview.rows.filter((row) => row.reviewers.some((r) => r.id === userId)).map((row) => row.schedule_id));
  const [onlyMine, setOnlyMine] = useState(false);
  const canReview = useCan('monitoring.review');
  const notify = useMutation({
    mutationFn: (d: SerialDuplicate) => apiRequest<{ data: { notified: number } }>('/api/v1/monitoring/serial-duplicates/notify', {
      method: 'POST', body: JSON.stringify({ program_id: overview.program_id, field: d.field, serial: d.serial }),
    }),
    onSuccess: (r, d) => toast.success(`${r.data.notified} orang dikabari tentang serial ${d.serial}.`),
    onError: (e) => toast.error(e.message),
  });
  if (duplicates.isPending) return <DataState kind="loading" title="Memeriksa serial" description="Mencari serial yang tercatat lebih dari sekali." />;
  if (duplicates.isError) return <DataState kind="error" title="Daftar serial dobel belum dapat dimuat" description={duplicates.error.message} action={{ label: 'Muat ulang', onClick: () => void duplicates.refetch() }} />;
  const all = duplicates.data;
  const list = onlyMine ? all.filter((d) => d.entries.some((e) => mine.has(e.schedule_id))) : all;
  if (all.length === 0) {
    return <Card className="flex flex-col items-center gap-2 p-10 text-center">
      <CircleCheck className="size-8 text-emerald-500" />
      <p className="font-medium">Tidak ada serial dobel</p>
      <p className="text-sm text-muted-foreground">Setiap serial mesin, konkit/reducer, dan selang hanya tercatat di satu nomor bagi.</p>
    </Card>;
  }
  const copy = () => navigator.clipboard.writeText(duplicatesMessage(list)).then(() => toast.success('Daftar disalin, siap ditempel di grup.'), () => toast.error('Gagal menyalin.'));
  return <div className="space-y-3">
    <div className="flex flex-wrap items-center justify-between gap-3">
      <p className="text-sm text-muted-foreground"><b className="text-destructive">{all.length} serial</b> tercatat di lebih dari satu nomor bagi. Cocokkan dengan kartu barcode di box, lalu perbaiki serial yang salah di nomor baginya.</p>
      <div className="flex items-center gap-3">
        {mine.size > 0 ? <Label className="flex items-center gap-2 font-normal"><Checkbox checked={onlyMine} onCheckedChange={(checked) => setOnlyMine(checked === true)} />Hanya kabupaten saya</Label> : null}
        <Button variant="outline" size="sm" onClick={() => void copy()}><Copy />Salin untuk grup</Button>
      </div>
    </div>
    <Card className="overflow-x-auto p-0">
      <table className="w-full text-sm">
        <thead><tr className="border-b bg-muted/40 text-left text-xs">
          <th className="px-3 py-2 font-medium">Serial</th><th className="px-3 py-2 font-medium">Jenis</th>
          <th className="px-3 py-2 font-medium">Kabupaten</th><th className="px-3 py-2 font-medium">Nomor bagi</th>
          <th className="px-3 py-2 font-medium">Status</th><th className="px-3 py-2 font-medium">Diinput</th>
        </tr></thead>
        <tbody>
          {list.map((d) => d.entries.map((e, i) => <tr key={`${d.field}|${d.serial}|${e.schedule_id}|${e.slot_number}`} className={i === d.entries.length - 1 ? 'border-b last:border-0' : ''}>
            {i === 0 ? <td rowSpan={d.entries.length} className="px-3 py-2 align-top">
              <p className="font-mono text-xs font-semibold">{d.serial}</p>
              {canReview ? <Button variant="outline" size="sm" className="mt-1.5 h-7 text-xs" disabled={notify.isPending} onClick={() => notify.mutate(d)}
                title="Kirim notifikasi ke penginput dan lead kabupaten terkait"><BellRing className="size-3.5" />Kabari PIC</Button> : null}
            </td> : null}
            {i === 0 ? <td rowSpan={d.entries.length} className="px-3 py-2 align-top">{serialFieldLabels[d.field]}</td> : null}
            <td className="px-3 py-1.5">{e.regency_name}</td>
            <td className="px-3 py-1.5"><button type="button" className="font-medium text-primary hover:underline" onClick={() => onOpen(e.schedule_id, e.slot_number)} aria-label={`Nomor bagi ${e.slot_number} ${e.regency_name}`}>#{e.slot_number}</button></td>
            <td className="px-3 py-1.5 text-muted-foreground">{statusLabels[e.status] ?? e.status}</td>
            <td className="px-3 py-1.5 text-muted-foreground">{e.entered_by || '-'} · {formatTime(e.entered_at)}</td>
          </tr>))}
        </tbody>
      </table>
    </Card>
  </div>;
}
