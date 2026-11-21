import { useMutation, useQueryClient } from '@tanstack/react-query';
import { CircleHelp, ImageOff, Save, Trash2 } from 'lucide-react';
import { type FormEvent, useEffect, useState } from 'react';
import { toast } from 'sonner';
import { DataState } from '../../components/DataState';
import { Button } from '../../components/ui/button';
import { Label } from '../../components/ui/label';
import { apiRequest } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { slotExamplesQueryKey, type SlotExampleEntry, useSlotExamples } from '../distribution/slotExamples';

const stageLabel: Record<string, string> = { mesin: 'POS Mesin', dokumen: 'POS Dokumen', penyerahan: 'POS Penyerahan' };
const MAX_IMAGE_BYTES = 10 * 1024 * 1024;

/**
 * Contoh foto per slot dokumentasi pendistribusian. Petugas lapangan
 * membukanya dari tanda tanya di pojok kartu slot sebagai acuan sudut foto.
 * Hanya super admin yang mengelolanya.
 */
export function SlotExamplesPanel() {
  const canManage = useCan('documentation_examples.manage');
  const examples = useSlotExamples(canManage);
  if (!canManage) return null;

  return <section className="overflow-hidden rounded-lg border bg-card" aria-labelledby="slot-examples-title">
    <div className="flex items-center gap-3 border-b px-5 py-4">
      <span className="grid size-9 place-items-center rounded-lg bg-primary/10 text-primary"><CircleHelp className="size-4" aria-hidden="true" /></span>
      <div>
        <h2 id="slot-examples-title" className="font-semibold">Contoh foto dokumentasi</h2>
        <p className="mt-0.5 text-sm text-muted-foreground">Acuan sudut foto untuk petugas lapangan. Tampil lewat tanda tanya di pojok slot Pendistribusian, untuk semua jadwal.</p>
      </div>
    </div>
    {examples.isError
      ? <div className="p-5"><DataState kind="error" title="Daftar slot belum dapat dimuat" description="Periksa koneksi lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => examples.refetch() }} /></div>
      : examples.isPending
        ? <div className="p-5"><DataState kind="loading" title="Memuat slot dokumentasi" description="Mengambil daftar slot dan contohnya." /></div>
        : examples.data.length === 0
          ? <p className="p-5 text-sm text-muted-foreground">Belum ada slot dokumentasi. Buat template dokumentasi di Penyiapan Program terlebih dahulu.</p>
          : <ul className="divide-y">{examples.data.map((entry) => <SlotExampleRow key={entry.slot_code} entry={entry} />)}</ul>}
  </section>;
}

function SlotExampleRow({ entry }: { entry: SlotExampleEntry }) {
  const client = useQueryClient();
  const [caption, setCaption] = useState(entry.example?.caption ?? '');
  const [file, setFile] = useState<File | null>(null);
  const [fileKey, setFileKey] = useState(0);
  const [error, setError] = useState('');
  useEffect(() => setCaption(entry.example?.caption ?? ''), [entry.example?.caption]);
  const fieldId = `slot-example-${entry.slot_code}`;

  const save = useMutation({
    mutationFn: () => {
      const form = new FormData();
      form.append('caption', caption.trim());
      if (file) form.append('file', file);
      return apiRequest(`/api/v1/documentation-examples/${entry.slot_code}`, { method: 'PUT', body: form });
    },
    onSuccess: () => {
      toast.success(`Contoh foto ${entry.label} disimpan.`);
      setFile(null);
      setFileKey((key) => key + 1);
      void client.invalidateQueries({ queryKey: slotExamplesQueryKey });
    },
    onError: (cause: Error) => setError(cause.message || 'Contoh foto belum dapat disimpan.'),
  });
  const remove = useMutation({
    mutationFn: () => apiRequest(`/api/v1/documentation-examples/${entry.slot_code}`, { method: 'DELETE' }),
    onSuccess: () => {
      toast.success(`Contoh foto ${entry.label} dihapus.`);
      void client.invalidateQueries({ queryKey: slotExamplesQueryKey });
    },
    onError: (cause: Error) => setError(cause.message || 'Contoh foto belum dapat dihapus.'),
  });

  const chooseFile = (chosen: File | null) => {
    setError('');
    if (chosen && !['image/jpeg', 'image/png', 'image/webp'].includes(chosen.type)) { setError('Gunakan foto JPEG, PNG, atau WebP.'); return; }
    if (chosen && chosen.size > MAX_IMAGE_BYTES) { setError('Ukuran contoh foto maksimal 10 MiB.'); return; }
    setFile(chosen);
  };
  const changed = file !== null || (entry.example !== null && caption.trim() !== entry.example.caption);
  const busy = save.isPending || remove.isPending;

  return <li className="grid gap-4 px-5 py-4 sm:grid-cols-[8rem_minmax(0,1fr)]">
    {entry.example
      ? <img src={entry.example.image_url} alt={`Contoh foto ${entry.label}`} className="aspect-square w-32 rounded-lg border bg-muted object-cover" />
      : <div className="grid aspect-square w-32 place-items-center rounded-lg border border-dashed bg-muted/40 text-muted-foreground"><ImageOff className="size-6" aria-hidden="true" /><span className="sr-only">Belum ada contoh</span></div>}
    <form className="grid min-w-0 gap-3" aria-label={`Contoh foto ${entry.label}`} onSubmit={(event: FormEvent) => { event.preventDefault(); setError(''); if (changed) save.mutate(); }}>
      <div>
        <p className="font-medium">{entry.label}</p>
        <p className="text-xs text-muted-foreground">{[stageLabel[entry.stage], entry.slot_code].filter(Boolean).join(' · ')}</p>
      </div>
      <div className="grid gap-2">
        <Label htmlFor={`${fieldId}-file`}>{entry.example ? 'Ganti foto' : 'Foto contoh'}</Label>
        <input key={fileKey} id={`${fieldId}-file`} type="file" accept="image/jpeg,image/png,image/webp" className="text-sm" onChange={(event) => chooseFile(event.target.files?.[0] ?? null)} />
      </div>
      <div className="grid gap-2">
        <Label htmlFor={`${fieldId}-caption`}>Keterangan</Label>
        <textarea id={`${fieldId}-caption`} rows={2} maxLength={500} className="min-h-16 rounded-lg border border-input bg-transparent px-3 py-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50" value={caption} onChange={(event) => setCaption(event.target.value)} placeholder="Mis. ambil dari depan, KTP dan nomor urut terbaca jelas." />
      </div>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      <div className="flex flex-wrap justify-end gap-2">
        {entry.example && <Button type="button" variant="outline" disabled={busy} onClick={() => { if (window.confirm(`Hapus contoh foto ${entry.label}?`)) { setError(''); remove.mutate(); } }}><Trash2 aria-hidden="true" />Hapus</Button>}
        <Button type="submit" disabled={!changed || busy}><Save aria-hidden="true" />{save.isPending ? 'Menyimpan…' : 'Simpan'}</Button>
      </div>
    </form>
  </li>;
}
