import { useMutation, useQueryClient } from '@tanstack/react-query';
import { AlertTriangle, ChevronDown, Eye, FileText, FileUp, Landmark, Plus, Trash2, Wallet } from 'lucide-react';
import { useState } from 'react';
import { Link } from 'react-router-dom';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { ApiError } from '@/lib/api';
import { cn } from '@/lib/utils';
import { attachmentPath, blankItem, deleteAttachment, formatDate, formatRupiah, isOverdue, itemsTotal, kindLabels, requestLine, statusLabels, statusTone, timeAgo, typeLabels, uploadAttachment, type Attachment, type FinanceRequest, type Item } from './api';
import { FilePreviewButton } from './FilePreviewDialog';
import { MoneyInput } from './MoneyInput';

export function errorMessage(error: unknown) {
  return error instanceof ApiError || error instanceof Error ? error.message : 'Permintaan belum dapat diproses.';
}

export function FinanceStatus({ status }: { status: string }) {
  return <Badge className={cn('border-transparent', statusTone(status))}>{statusLabels[status] ?? status}</Badge>;
}

/** Baris ringkas satu pengajuan di semua daftar. */
export function RequestRow({ request, queue = false }: { request: FinanceRequest; queue?: boolean }) {
  const overdue = isOverdue(request);
  const waiting = timeAgo(request.submitted_at);
  const title = queue ? request.requester_name : request.number || 'Draf';
  const meta = queue
    ? [request.number, request.team_name, waiting && `Diajukan ${waiting}`]
    : [request.team_name, request.needed_date && `Butuh ${formatDate(request.needed_date)}`];
  const kasbon = request.type === 'kasbon';
  const Icon = kasbon ? Wallet : Landmark;
  return <li>
    <Link to={`/keuangan/${request.id}`} className="grid grid-cols-[auto_minmax(0,1fr)] items-center gap-x-3 gap-y-1 rounded-xl border bg-card p-3 transition-colors hover:border-primary/50 focus-visible:outline-2 focus-visible:outline-ring sm:grid-cols-[auto_minmax(0,1fr)_auto]">
      <span aria-hidden="true" className={cn('grid size-10 place-items-center rounded-full', kasbon ? 'bg-amber-100 text-amber-800 dark:bg-amber-950/40 dark:text-amber-200' : 'bg-emerald-100 text-emerald-800 dark:bg-emerald-950/40 dark:text-emerald-200')}><Icon className="size-5" /></span>
      <span className="min-w-0">
        <span className="flex flex-wrap items-center gap-2">
          <strong className="text-sm">{title}</strong>
          <Badge variant="outline">{typeLabels[request.type]}</Badge>
          <FinanceStatus status={request.status} />
          {overdue && <Badge className="border-transparent bg-destructive/10 text-destructive"><AlertTriangle aria-hidden="true" />Lewat batas lapor</Badge>}
        </span>
        <span className="mt-0.5 block truncate text-sm text-muted-foreground">{request.description || 'Belum ada keterangan'}</span>
        <span className="block truncate text-xs text-muted-foreground">{meta.filter(Boolean).join(' · ')}</span>
      </span>
      <strong className="col-start-2 text-base sm:col-start-3 sm:text-right">{formatRupiah(request.total_amount)}</strong>
    </Link>
  </li>;
}

/** Bukti langsung terlihat; klik membuka preview di halaman ini. */
export function ProofThumbs({ request, files }: { request: Pick<FinanceRequest, 'id'>; files: Attachment[] }) {
  if (files.length === 0) return null;
  return <div className="flex flex-wrap gap-2">{files.map((file) => <FilePreviewButton key={file.id} path={attachmentPath(request.id, file.id)} filename={file.filename} variant="outline" className="h-16 w-16 overflow-hidden p-0" aria-label={`Lihat ${kindLabels[file.kind]} ${file.filename}`}>
    {file.content_type === 'application/pdf'
      ? <FileText aria-hidden="true" className="size-6 text-muted-foreground" />
      : <img src={attachmentPath(request.id, file.id)} alt="" loading="lazy" className="size-full object-cover" />}
  </FilePreviewButton>)}</div>;
}

export type RowProofs = { request: FinanceRequest; beforeUpload: () => Promise<unknown> };
type ItemsEditorProps = { items: Item[]; disabled?: boolean; proofs?: RowProofs; onChange: (items: Item[]) => void };

/** Bukti satu biaya; tercetak di belakang voucher sesuai nomor barisnya. */
function ProofList({ item, proofs }: { item: Item; proofs: RowProofs }) {
  const queryClient = useQueryClient();
  const id = proofs.request.id;
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['finance', 'request', id] });
  const upload = useMutation({
    mutationFn: async ({ kind, file }: { kind: string; file: File }) => {
      await proofs.beforeUpload();
      return uploadAttachment(id, kind, file, item.key);
    },
    onSuccess: () => void refresh(),
    onError: (error) => toast.error(errorMessage(error)),
  });
  const remove = useMutation({
    mutationFn: (attachmentID: string) => deleteAttachment(id, attachmentID),
    onSuccess: () => void refresh(),
    onError: (error) => toast.error(errorMessage(error)),
  });
  const files = (proofs.request.attachments ?? []).filter((file) => file.item_key === item.key);
  return <div className="grid gap-2">
    {files.map((file) => <div key={file.id} className="flex flex-wrap items-center justify-between gap-2 rounded-md bg-muted/50 px-2 py-1 text-xs">
      <span className="min-w-0 truncate">{kindLabels[file.kind]}: {file.filename}</span>
      <span className="flex gap-1">
        <FilePreviewButton path={attachmentPath(id, file.id)} filename={file.filename} variant="ghost" size="sm"><Eye />Lihat</FilePreviewButton>
        <Button type="button" variant="ghost" size="icon-sm" aria-label={`Hapus ${file.filename}`} disabled={remove.isPending} onClick={() => remove.mutate(file.id)}><Trash2 /></Button>
      </span>
    </div>)}
    <label className="inline-flex w-fit min-h-9 cursor-pointer items-center gap-2 rounded-md border px-3 text-xs font-medium hover:bg-muted">
      <input type="file" accept="image/jpeg,image/png,application/pdf" className="sr-only" disabled={upload.isPending} onChange={(event) => { const file = event.target.files?.[0]; event.target.value = ''; if (file) upload.mutate({ kind: 'item_transaction', file }); }} />
      <FileUp aria-hidden="true" className="size-4" />{upload.isPending ? 'Mengunggah...' : 'Unggah nota / bukti'}
    </label>
  </div>;
}

/** Daftar biaya; total dihitung ulang di server. */
export function ItemsEditor({ items, disabled, proofs, onChange }: ItemsEditorProps) {
  const incomplete = (item: Item) => item.description.trim() === '' || requestLine(item) === 0;
  const [open, setOpen] = useState(() => items.length === 1 ? items[0].key : items.find(incomplete)?.key ?? '');
  const update = (index: number, patch: Partial<Item>) => onChange(items.map((item, position) => position === index ? { ...item, ...patch } : item));
  const add = () => {
    const item = blankItem();
    onChange([...items, item]);
    setOpen(item.key);
  };
  return <div className="grid gap-2">
    {items.map((item, index) => {
      const expanded = open === item.key;
      const proofCount = (proofs?.request.attachments ?? []).filter((file) => file.item_key === item.key).length;
      const facts = [`${item.quantity} × ${formatRupiah(item.unit_price)}`, proofCount > 0 && `${proofCount} bukti`].filter(Boolean).join(' · ');
      return <div key={item.key} className={cn('rounded-lg border transition-colors', expanded && 'border-primary/50')}>
        <button type="button" aria-expanded={expanded} className="flex w-full items-center gap-3 rounded-lg p-3 text-left hover:bg-muted/40 focus-visible:outline-2 focus-visible:outline-ring" onClick={() => setOpen(expanded ? '' : item.key)}>
          <span aria-hidden="true" className="grid size-6 shrink-0 place-items-center rounded-full bg-muted text-xs font-semibold text-muted-foreground">{index + 1}</span>
          <span className="min-w-0 flex-1">
            <strong className="block truncate text-sm">{item.description.trim() || `Biaya ${index + 1}`}</strong>
            {!expanded && <span className={cn('block truncate text-xs', incomplete(item) ? 'font-medium text-amber-700 dark:text-amber-300' : 'text-muted-foreground')}>{incomplete(item) ? 'Belum lengkap' : facts}</span>}
          </span>
          <strong className={cn('text-sm whitespace-nowrap', item.entry === 'kredit' && 'text-destructive')}>{item.entry === 'kredit' ? '-' : ''}{formatRupiah(requestLine(item))}</strong>
          <ChevronDown aria-hidden="true" className={cn('size-4 shrink-0 text-muted-foreground transition-transform', expanded && 'rotate-180')} />
        </button>
        {expanded && <div className="grid gap-3 border-t p-3">
          <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_5rem_9rem_auto] sm:items-end">
            <label className="grid gap-1 text-xs font-medium">Nama barang / biaya<Input value={item.description} disabled={disabled} maxLength={200} onChange={(event) => update(index, { description: event.target.value })} /></label>
            <label className="grid gap-1 text-xs font-medium">Banyaknya<Input type="number" inputMode="numeric" min={0} value={item.quantity || ''} disabled={disabled} onChange={(event) => update(index, { quantity: Math.max(0, Math.floor(Number(event.target.value) || 0)) })} /></label>
            <label className="grid gap-1 text-xs font-medium">Harga per barang (Rp)<MoneyInput value={item.unit_price} disabled={disabled} onChange={(unit_price) => update(index, { unit_price })} /></label>
            <Button type="button" variant="ghost" size="icon" aria-label={`Hapus biaya ${index + 1}`} disabled={disabled || items.length === 1} onClick={() => onChange(items.filter((_, position) => position !== index))}><Trash2 /></Button>
          </div>
          {proofs && <ProofList item={item} proofs={proofs} />}
        </div>}
      </div>;
    })}
    <Button type="button" variant="outline" className="justify-self-start" disabled={disabled || items.length >= 100} onClick={add}><Plus />Tambah biaya</Button>
    <div className="flex items-center justify-between gap-2 rounded-lg bg-primary/10 px-4 py-3">
      <span className="text-sm font-medium">Total pengajuan</span>
      <strong className="text-xl text-primary">{formatRupiah(itemsTotal(items))}</strong>
    </div>
  </div>;
}
