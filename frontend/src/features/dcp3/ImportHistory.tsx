import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { AlertTriangle, FileSpreadsheet, Trash2 } from 'lucide-react';
import { useEffect, useState } from 'react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import { apiRequest } from '@/lib/api';
import { cn } from '@/lib/utils';

export type ImportSummary = {
  id: string; filename: string; total_rows: number; imported_at: string | null; imported_by: string;
  recipients: number; linked: number; distributed: number;
};
export type RemovalRecipient = {
  allocation_id: string; full_name: string; nik: string; slot_number?: number;
  state: 'free' | 'linked' | 'completed' | 'replaced'; warning?: string; selected: boolean;
};
export type Removal = { import: ImportSummary; recipients: RemovalRecipient[]; removed: number; unlinked: number; kept: number; import_deleted: boolean };

const importsKey = (scheduleId: string) => ['dcp3', 'imports', scheduleId];
const formatTime = (value: string | null) => value ? new Date(value).toLocaleString('id-ID', { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' }) : '-';
const stateLabels: Record<RemovalRecipient['state'], { label: string; tone: string }> = {
  free: { label: 'Belum tersambung', tone: 'bg-muted text-muted-foreground' },
  linked: { label: 'Tersambung', tone: 'bg-amber-100 text-amber-900 dark:bg-amber-950/40 dark:text-amber-200' },
  completed: { label: 'Sudah selesai', tone: 'bg-destructive/10 text-destructive' },
  replaced: { label: 'Ada penggantian', tone: 'bg-destructive/10 text-destructive' },
};

/**
 * The DCP3 files already imported into the schedule. A wrongly uploaded
 * file (e.g. another kabupaten's) can be removed recipient by recipient:
 * those without a warning are ticked, the ones on a nomor bagi, distributed
 * or replaced are not but can be ticked.
 */
export function ImportHistory({ scheduleId, canRemove }: { scheduleId: string; canRemove: boolean }) {
  const queryClient = useQueryClient();
  const [target, setTarget] = useState<ImportSummary | null>(null);
  const [chosen, setChosen] = useState<Set<string>>(new Set());
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
  useEffect(() => {
    if (preview.data) setChosen(new Set(preview.data.recipients.filter((r) => r.selected).map((r) => r.allocation_id)));
  }, [preview.data]);
  const remove = useMutation({
    mutationFn: (id: string) => apiRequest<{ data: Removal }>(`/api/v1/dcp3/imports/${id}`, { method: 'DELETE', body: JSON.stringify({ allocation_ids: [...chosen] }) }).then((r) => r.data),
    onSuccess: (result) => {
      toast.success(`${result.removed} penerima dari ${result.import.filename} dihapus${result.unlinked ? `, ${result.unlinked} nomor bagi dikosongkan` : ''}${result.import_deleted ? '. File bisa diimport ulang.' : ''}`);
      setTarget(null);
      void queryClient.invalidateQueries({ queryKey: ['dcp3'] });
      void queryClient.invalidateQueries({ queryKey: ['recipients'] });
    },
    onError: (e) => toast.error(e.message),
  });

  if (!scheduleId || imports.isPending || imports.isError || imports.data.length === 0) {
    return imports.isError ? <p className="text-sm text-destructive">Riwayat import belum dapat dimuat.</p> : null;
  }
  const recipients = preview.data?.recipients ?? [];
  const toggle = (id: string, on: boolean) => setChosen((current) => {
    const next = new Set(current);
    if (on) next.add(id); else next.delete(id);
    return next;
  });
  const chosenWarnings = recipients.filter((r) => chosen.has(r.allocation_id) && r.warning).length;
  const allChosen = recipients.length > 0 && chosen.size === recipients.length;

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
      <DialogContent className="flex max-h-[90dvh] flex-col sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Hapus penerima dari {target?.filename}</DialogTitle>
          <DialogDescription>Centang penerima yang dihapus permanen. Yang punya peringatan tidak dicentang otomatis. Penghapusan tercatat di riwayat audit dan tidak bisa dibatalkan.</DialogDescription>
        </DialogHeader>
        {preview.isPending ? <p className="text-sm text-muted-foreground">Memuat penerima…</p>
          : preview.isError ? <p className="text-sm text-destructive">{preview.error.message}</p>
          : <>
            <Label className="flex items-center gap-2 border-b pb-2 text-sm font-normal">
              <Checkbox checked={allChosen} onCheckedChange={(checked) => setChosen(checked === true ? new Set(recipients.map((r) => r.allocation_id)) : new Set())} />
              Pilih semua ({recipients.length})
            </Label>
            <ul className="-mx-1 min-h-0 flex-1 space-y-1 overflow-y-auto px-1" aria-label="Penerima import">
              {recipients.map((r) => {
                const state = stateLabels[r.state];
                return <li key={r.allocation_id}>
                  <Label className={cn('flex items-start gap-3 rounded-lg border p-2.5 font-normal', chosen.has(r.allocation_id) && r.warning && 'border-destructive/50 bg-destructive/5')}>
                    <Checkbox className="mt-0.5" checked={chosen.has(r.allocation_id)} onCheckedChange={(checked) => toggle(r.allocation_id, checked === true)} />
                    <span className="min-w-0 flex-1">
                      <span className="flex flex-wrap items-center gap-2">
                        <span className="font-medium">{r.slot_number ? `#${r.slot_number} ` : ''}{r.full_name || '(tanpa nama)'}</span>
                        <span className={cn('rounded px-1.5 py-0.5 text-[11px] font-medium', state.tone)}>{state.label}</span>
                      </span>
                      <span className="block text-xs text-muted-foreground tabular-nums">{r.nik || 'Tanpa NIK'}</span>
                      {r.warning ? <span className="mt-1 flex items-start gap-1 text-xs text-amber-800 dark:text-amber-200"><AlertTriangle className="mt-0.5 size-3 shrink-0" />{r.warning}</span> : null}
                    </span>
                  </Label>
                </li>;
              })}
            </ul>
            <p className="text-sm">
              <b className="tabular-nums">{chosen.size}</b> dari {recipients.length} penerima akan dihapus
              {chosenWarnings ? <span className="text-destructive"> · {chosenWarnings} dengan peringatan</span> : null}
              {chosen.size === recipients.length && recipients.length > 0 ? <span className="text-muted-foreground"> · file ikut terhapus dan bisa diimport ulang</span> : null}
            </p>
          </>}
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => setTarget(null)} disabled={remove.isPending}>Batal</Button>
          <Button type="button" variant="destructive" disabled={!preview.data || chosen.size === 0 || remove.isPending} onClick={() => target && remove.mutate(target.id)}>
            <Trash2 />Hapus {chosen.size} penerima
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </section>;
}
