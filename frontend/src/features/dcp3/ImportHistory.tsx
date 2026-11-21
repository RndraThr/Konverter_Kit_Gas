import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { FileSpreadsheet, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { apiRequest } from '@/lib/api';

export type ImportSummary = {
  id: string; filename: string; total_rows: number; imported_at: string | null; imported_by: string;
  recipients: number; linked: number; distributed: number;
};
type RemovalRecipient = { full_name: string; nik: string; slot_number?: number; reason?: string };
export type Removal = { import: ImportSummary; removed: number; unlinked: RemovalRecipient[]; kept: RemovalRecipient[]; import_deleted: boolean };

const importsKey = (scheduleId: string) => ['dcp3', 'imports', scheduleId];
const formatTime = (value: string | null) => value ? new Date(value).toLocaleString('id-ID', { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' }) : '-';

/**
 * The DCP3 files already imported into the schedule. A wrongly uploaded
 * file (e.g. another kabupaten's) can be removed: the dialog first shows
 * what happens to its recipients.
 */
export function ImportHistory({ scheduleId, canRemove }: { scheduleId: string; canRemove: boolean }) {
  const queryClient = useQueryClient();
  const [target, setTarget] = useState<ImportSummary | null>(null);
  const imports = useQuery({
    queryKey: importsKey(scheduleId),
    queryFn: () => apiRequest<{ data: ImportSummary[] }>(`/api/v1/dcp3/imports?schedule_id=${scheduleId}`).then((r) => r.data),
    enabled: scheduleId !== '',
  });
  const preview = useQuery({
    queryKey: ['dcp3', 'removal', target?.id],
    queryFn: () => apiRequest<{ data: Removal }>(`/api/v1/dcp3/imports/${target!.id}/removal`).then((r) => r.data),
    enabled: target !== null,
    gcTime: 0,
  });
  const remove = useMutation({
    mutationFn: (id: string) => apiRequest<{ data: Removal }>(`/api/v1/dcp3/imports/${id}`, { method: 'DELETE' }).then((r) => r.data),
    onSuccess: (result) => {
      toast.success(`Import ${result.import.filename} dihapus: ${result.removed} penerima${result.unlinked.length ? `, ${result.unlinked.length} nomor bagi dilepas` : ''}`);
      setTarget(null);
      void queryClient.invalidateQueries({ queryKey: ['dcp3'] });
      void queryClient.invalidateQueries({ queryKey: ['recipients'] });
    },
    onError: (e) => toast.error(e.message),
  });

  if (!scheduleId || imports.isPending || imports.isError || imports.data.length === 0) {
    return imports.isError ? <p className="text-sm text-destructive">Riwayat import belum dapat dimuat.</p> : null;
  }
  const result = preview.data;
  return <section className="space-y-2" aria-label="Riwayat import">
    <h3 className="text-sm font-semibold">Riwayat import jadwal ini</h3>
    <ul className="divide-y rounded-lg border">
      {imports.data.map((item) => <li key={item.id} className="flex flex-wrap items-center gap-3 px-3 py-2.5 text-sm">
        <FileSpreadsheet className="size-5 shrink-0 text-primary" aria-hidden="true" />
        <div className="min-w-0 flex-1">
          <p className="truncate font-medium">{item.filename}</p>
          <p className="text-xs text-muted-foreground">
            {formatTime(item.imported_at)}{item.imported_by ? ` · ${item.imported_by}` : ''} · {item.total_rows} baris · {item.recipients} penerima
            {item.linked ? ` · ${item.linked} tersambung nomor bagi` : ''}{item.distributed ? ` · ${item.distributed} selesai` : ''}
          </p>
        </div>
        {canRemove ? <Button type="button" variant="outline" size="sm" className="text-destructive hover:text-destructive" onClick={() => setTarget(item)}>
          <Trash2 />Hapus import
        </Button> : null}
      </li>)}
    </ul>
    <Dialog open={target !== null} onOpenChange={(open) => { if (!open && !remove.isPending) setTarget(null); }}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Hapus import {target?.filename}?</DialogTitle>
          <DialogDescription>Penerima dari file ini dihapus permanen supaya file yang benar bisa diimport ulang. Tindakan ini tercatat di riwayat audit dan tidak bisa dibatalkan.</DialogDescription>
        </DialogHeader>
        {preview.isPending ? <p className="text-sm text-muted-foreground">Menghitung dampak…</p>
          : preview.isError || !result ? <p className="text-sm text-destructive">{preview.error?.message ?? 'Dampak belum dapat dihitung.'}</p>
          : <div className="space-y-3 text-sm">
            <p><b className="tabular-nums">{result.removed}</b> penerima akan dihapus beserta baris DCP3-nya.</p>
            {result.unlinked.length > 0 ? <div className="rounded-lg bg-amber-50 p-3 text-amber-950 dark:bg-amber-950/40 dark:text-amber-100">
              <p className="font-medium">{result.unlinked.length} nomor bagi dilepas dan kembali kosong (foto tetap ada, tanda tangan penerima dihapus):</p>
              <p className="mt-1 text-xs">{result.unlinked.map((r) => `#${r.slot_number} ${r.full_name}`).join(', ')}</p>
            </div> : null}
            {result.kept.length > 0 ? <div className="rounded-lg border p-3">
              <p className="font-medium">{result.kept.length} penerima tetap disimpan:</p>
              <ul className="mt-1 max-h-32 space-y-0.5 overflow-y-auto text-xs text-muted-foreground">
                {result.kept.map((r) => <li key={`${r.nik}-${r.slot_number}`}>{r.slot_number ? `#${r.slot_number} ` : ''}{r.full_name} — {r.reason}</li>)}
              </ul>
              <p className="mt-1 text-xs text-muted-foreground">Karena itu, file ini tetap tercatat dengan sisa penerima tersebut.</p>
            </div> : <p className="text-xs text-muted-foreground">Tidak ada yang tersisa, jadi file ini ikut terhapus dan bisa diimport ulang.</p>}
          </div>}
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => setTarget(null)} disabled={remove.isPending}>Batal</Button>
          <Button type="button" variant="destructive" disabled={!result || remove.isPending} onClick={() => target && remove.mutate(target.id)}><Trash2 />Hapus import</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </section>;
}
