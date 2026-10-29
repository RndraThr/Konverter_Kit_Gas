import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '../../../components/ui/dialog';
import { MILESTONES } from './milestones';
import { useUnlocated } from './useUnlocated';

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Same filter query string as the points request. */
  filter: string;
  regencyNames: Map<string, string>;
};

/** Slots with no GPS photo yet: counted, not hidden (spec §4.1). */
export function UnlocatedDialog({ open, onOpenChange, filter, regencyNames }: Props) {
  const list = useUnlocated(filter, open);
  return <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-md">
      <DialogHeader>
        <DialogTitle>Nomor bagi belum ada lokasi</DialogTitle>
        <DialogDescription>Belum ada foto berkoordinat GPS yang diterima untuk nomor bagi ini, jadi tidak muncul di peta.</DialogDescription>
      </DialogHeader>
      {list.isPending && <p className="text-sm text-muted-foreground">Memuat daftar…</p>}
      {list.isError && <p role="alert" className="text-sm text-destructive">Daftar tidak dapat dimuat. Coba buka kembali.</p>}
      {list.data && <>
        {list.data.items.length === 0
          ? <p className="text-sm text-muted-foreground">Semua nomor bagi sudah punya lokasi.</p>
          : <ul aria-label="Nomor bagi tanpa lokasi" className="divide-y rounded-md border">
            {list.data.items.map((item) => <li key={item.slot_id} className="flex items-center gap-2 px-3 py-2 text-sm">
              <span className="font-medium">Nomor bagi {item.code}</span>
              <span className="text-muted-foreground">{regencyNames.get(item.regency_id) ?? '-'}</span>
              <span className="ml-auto text-xs text-muted-foreground">{MILESTONES[Math.min(5, Math.max(1, item.stage)) - 1].label}</span>
            </li>)}
          </ul>}
        {list.data.total > list.data.items.length && <p className="text-xs text-muted-foreground">Menampilkan {list.data.items.length} dari {list.data.total} nomor bagi. Persempit dengan filter untuk melihat sisanya.</p>}
      </>}
    </DialogContent>
  </Dialog>;
}
