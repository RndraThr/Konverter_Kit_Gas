import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ChevronDown, Eye, FilePlus2, FileUp, Landmark, Paperclip, Pencil, Wallet } from 'lucide-react';
import { useState } from 'react';
import { toast } from 'sonner';
import { DataState } from '@/components/DataState';
import { DateRangePicker } from '@/components/DateRangePicker';
import { PageHeader } from '@/components/PageHeader';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { cn } from '@/lib/utils';
import {
  blankSheet, composeVoucher, fetchRequest, fetchVoucher, fetchVoucherCandidates, fetchVouchers, fetchVoucherSettings, financeKey, formatDate, formatDateTime, formatRupiah, kindLabels,
  rebalance, saveVoucher, shiftDate, sheetLines, today, typeLabels, voucherFilePath, type FinanceRequest, type SheetForm,
} from './api';
import { FilePreviewButton } from './FilePreviewDialog';
import { errorMessage, ProofThumbs } from './shared';
import { VoucherPdfPreview } from './VoucherPdfPreview';
import { VoucherSheet } from './VoucherSheet';

const vouchersKey = [...financeKey, 'vouchers'];

type RangeKey = 'today' | 'yesterday' | 'week' | 'custom';
type FilterKey = 'all' | 'cashless' | 'kasbon' | 'fresh';

const ranges: { key: RangeKey; label: string }[] = [{ key: 'today', label: 'Hari ini' }, { key: 'yesterday', label: 'Kemarin' }, { key: 'week', label: '7 hari' }, { key: 'custom', label: 'Rentang' }];
const filters: { key: FilterKey; label: string; match: (request: FinanceRequest) => boolean }[] = [
  { key: 'all', label: 'Semua', match: () => true },
  { key: 'cashless', label: 'Cashless', match: (request) => request.type === 'cashless' },
  { key: 'kasbon', label: 'Kasbon', match: (request) => request.type === 'kasbon' },
  { key: 'fresh', label: 'Belum divoucher', match: (request) => (request.vouchers?.length ?? 0) === 0 },
];

function Pill({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return <button type="button" aria-pressed={active} onClick={onClick} className={cn('rounded-full px-3.5 py-1.5 text-sm font-medium transition-colors focus-visible:outline-2 focus-visible:outline-ring', active ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground hover:text-foreground')}>{children}</button>;
}

/** Finance menyusun voucher dari transaksi yang dipilih, lalu melengkapinya di lembar. */
export function VoucherPage() {
  const queryClient = useQueryClient();
  const [from, setFrom] = useState(today);
  const [to, setTo] = useState(today);
  const [range, setRange] = useState<RangeKey>('today');
  const [filter, setFilter] = useState<FilterKey>('all');
  const [selected, setSelected] = useState<string[]>([]);
  const [form, setForm] = useState<SheetForm>(blankSheet);
  const settings = useQuery({ queryKey: [...financeKey, 'voucher-settings'], queryFn: fetchVoucherSettings });
  const candidates = useQuery({ queryKey: [...financeKey, 'voucher-candidates', from, to], queryFn: () => fetchVoucherCandidates(from, to), enabled: Boolean(from && to) });
  const vouchers = useQuery({ queryKey: vouchersKey, queryFn: fetchVouchers });
  const rows = candidates.data?.data ?? [];
  const chosen = rows.filter((request) => selected.includes(request.id));
  const repeated = chosen.filter((request) => (request.vouchers?.length ?? 0) > 0);
  const visible = rows.filter(filters.find((item) => item.key === filter)?.match ?? (() => true));
  const allVisible = visible.length > 0 && visible.every((request) => selected.includes(request.id));
  const chosenTotal = chosen.reduce((sum, request) => sum + request.total_amount, 0);

  const pick = (key: RangeKey) => {
    setRange(key);
    setSelected([]);
    if (key === 'custom') return;
    const end = key === 'yesterday' ? shiftDate(today(), -1) : today();
    setFrom(key === 'week' ? shiftDate(today(), -6) : end);
    setTo(end);
  };
  const toggleVisible = (on: boolean) => {
    const ids = visible.map((request) => request.id);
    setSelected((current) => on ? [...new Set([...current, ...ids])] : current.filter((id) => !ids.includes(id)));
  };
  const toggle = (id: string, on: boolean) => setSelected((current) => on ? [...current, id] : current.filter((item) => item !== id));

  const compose = useMutation({
    mutationFn: () => composeVoucher(selected),
    onSuccess: ({ data }) => {
      setForm((current) => ({ ...current, id: '', pay_to: data.pay_to, description: data.description, lines: rebalance(sheetLines(data.lines)), request_ids: data.request_ids }));
      toast.success('Transaksi masuk ke lembar voucher.');
    },
    onError: (error) => toast.error(errorMessage(error)),
  });
  const save = useMutation({
    mutationFn: () => saveVoucher(form),
    onSuccess: ({ data }) => {
      setForm((current) => ({ ...current, id: data.id }));
      void queryClient.invalidateQueries({ queryKey: financeKey });
      toast.success(`Voucher ${data.number} terbit.`);
    },
    onError: (error) => toast.error(errorMessage(error)),
  });
  const open = useMutation({
    mutationFn: fetchVoucher,
    onSuccess: ({ data }) => {
      setForm({ id: data.id, number: data.number, date: data.date, account_id: data.account_id, pay_to: data.pay_to, description: data.description, lines: rebalance(sheetLines(data.lines ?? [])), request_ids: data.request_ids ?? [] });
      window.scrollTo({ top: 0, behavior: 'smooth' });
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  const debit = form.lines.reduce((sum, line) => sum + line.debit, 0);
  const kredit = form.lines.reduce((sum, line) => sum + line.kredit, 0);
  const blocked = !form.account_id ? 'Pilih rekening.' : form.lines.some((line) => !line.is_bank && !line.description.trim()) ? 'Lengkapi Description tiap baris.'
    : debit === 0 ? 'Isi nominal baris.' : debit !== kredit ? 'Debit dan kredit belum sama.' : '';
  const missing = !form.number ? 'Isi nomor voucher.' : !form.pay_to.trim() ? 'Isi Pay To.' : blocked;

  return <div className="space-y-6">
    <PageHeader
      title="Voucher"
      description="Pilih transaksi, lengkapi lembar voucher, lalu terbitkan PDF beserta buktinya."
      actions={<Button variant="outline" onClick={() => { setForm(blankSheet()); setSelected([]); }}><FilePlus2 />Voucher baru</Button>}
    />
    <Card>
      <CardHeader className="border-b"><CardTitle className="text-base">Pilih transaksi</CardTitle></CardHeader>
      <CardContent className="grid gap-4 pt-5">
        <div className="flex flex-wrap items-center gap-2" role="group" aria-label="Periode">
          {ranges.map((item) => <Pill key={item.key} active={range === item.key} onClick={() => pick(item.key)}>{item.label}</Pill>)}
          {range === 'custom' && <DateRangePicker aria-label="Rentang tanggal" from={from} to={to} max={new Date()} onChange={(start, end) => { setFrom(start); setTo(end); setSelected([]); }} />}
        </div>
        {candidates.isPending && <DataState kind="loading" title="Memuat transaksi" description="Mengambil pengajuan yang selesai." />}
        {candidates.isError && <DataState kind="error" title="Transaksi belum dapat dimuat" description="Periksa koneksi lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => candidates.refetch() }} />}
        {candidates.isSuccess && rows.length === 0 && <DataState kind="empty" title="Tidak ada transaksi" description="Belum ada pengajuan yang selesai pada periode ini. Voucher tetap bisa diisi manual di bawah." />}
        {rows.length > 0 && <>
          <div className="flex flex-wrap items-center gap-1.5" role="group" aria-label="Jenis transaksi">
            <span className="mr-1.5 ml-3.25 flex items-center" title="Pilih semua">
              <Checkbox aria-label="Pilih semua" checked={allVisible} disabled={visible.length === 0} onCheckedChange={(on) => toggleVisible(Boolean(on))} />
            </span>
            {filters.map((item) => <button key={item.key} type="button" aria-pressed={filter === item.key} onClick={() => setFilter(item.key)} className={cn('rounded-md border px-2.5 py-1 text-xs font-medium transition-colors focus-visible:outline-2 focus-visible:outline-ring', filter === item.key ? 'border-primary text-primary' : 'text-muted-foreground hover:text-foreground')}>{item.label} ({rows.filter(item.match).length})</button>)}
          </div>
          {visible.length === 0
            ? <p className="py-4 text-center text-sm text-muted-foreground">Tidak ada transaksi pada pilihan ini.</p>
            : <ul className="grid gap-2" aria-label="Transaksi selesai">{visible.map((request) => <CandidateRow key={request.id} request={request} checked={selected.includes(request.id)} onToggle={(on) => toggle(request.id, on)} />)}</ul>}
          {repeated.length > 0 && <p role="status" className="rounded-lg border border-amber-300/60 bg-amber-50 px-3 py-2 text-sm text-amber-950 dark:bg-amber-950/20 dark:text-amber-100">{repeated.length} transaksi pilihan sudah pernah masuk voucher. Boleh dibuat lagi, tapi pastikan tidak tercatat dobel.</p>}
          <div className="flex flex-wrap items-center justify-between gap-3 border-t pt-4">
            <p className="text-sm">{selected.length === 0 ? <span className="text-muted-foreground">Belum ada yang dipilih</span> : <><strong>{selected.length} transaksi</strong> · <strong className="text-primary">{formatRupiah(chosenTotal)}</strong></>}</p>
            <Button disabled={selected.length === 0 || compose.isPending} onClick={() => compose.mutate()}><FileUp />{compose.isPending ? 'Menyusun...' : 'Masukkan ke voucher'}</Button>
          </div>
        </>}
      </CardContent>
    </Card>
    <Card>
      <CardHeader className="border-b"><div className="flex flex-wrap items-center justify-between gap-2">
        <CardTitle className="text-base">{form.id ? `Ubah voucher ${form.number}` : 'Lembar voucher'}</CardTitle>
        <p className="text-xs text-muted-foreground">Baris 1 terisi otomatis, dan bisa diubah sendiri.</p>
      </div></CardHeader>
      <CardContent className="grid gap-4 pt-5">
        {settings.data ? <VoucherSheet form={form} settings={settings.data.data} onChange={setForm} /> : <DataState kind="loading" title="Memuat lembar" description="Mengambil kop dan rekening." />}
        <div className="flex flex-wrap items-center justify-end gap-3">
          {missing && <p className="text-sm text-muted-foreground">{missing}</p>}
          <VoucherPdfPreview form={form} blocked={blocked !== ''} />
          <Button disabled={missing !== '' || save.isPending} onClick={() => save.mutate()}>{save.isPending ? 'Menyusun PDF...' : form.id ? 'Simpan perubahan' : 'Terbitkan voucher'}</Button>
        </div>
      </CardContent>
    </Card>
    <Card>
      <CardHeader className="border-b"><CardTitle className="text-base">Voucher terbit</CardTitle></CardHeader>
      <CardContent className="grid gap-2 pt-5">
        {vouchers.isSuccess && vouchers.data.data.length === 0 && <p className="text-sm text-muted-foreground">Belum ada voucher.</p>}
        {vouchers.data?.data.map((voucher) => <div key={voucher.id} className="flex flex-wrap items-center justify-between gap-2 rounded-lg border p-3">
          <span className="min-w-0">
            <strong className="text-sm">{voucher.number}</strong>
            <span className="block truncate text-xs text-muted-foreground">{[formatDate(voucher.date), voucher.pay_to, formatRupiah(voucher.total_amount), `${voucher.page_count} halaman`, `dibuat ${formatDateTime(voucher.created_at)}`].join(' · ')}</span>
          </span>
          <span className="flex gap-1">
            <FilePreviewButton path={voucherFilePath(voucher.id)} filename={`Voucher_Bundle_${voucher.number}.pdf`} variant="ghost" size="sm"><Eye />Lihat</FilePreviewButton>
            <Button variant="ghost" size="sm" disabled={open.isPending} onClick={() => open.mutate(voucher.id)}><Pencil />Ubah</Button>
          </span>
        </div>)}
      </CardContent>
    </Card>
  </div>;
}

function CandidateRow({ request, checked, onToggle }: { request: FinanceRequest; checked: boolean; onToggle: (on: boolean) => void }) {
  const [open, setOpen] = useState(false);
  const refs = request.vouchers ?? [];
  const kasbon = request.type === 'kasbon';
  const Icon = kasbon ? Wallet : Landmark;
  return <li className={cn('rounded-xl border transition-colors', checked ? 'border-primary bg-primary/5' : 'hover:bg-muted/40')}>
    <div className="flex flex-wrap items-center gap-x-1 pr-2">
      <label className="flex min-w-0 flex-1 cursor-pointer items-center gap-3 p-3">
        <Checkbox checked={checked} onCheckedChange={(on) => onToggle(Boolean(on))} />
        <span aria-hidden="true" className={cn('grid size-9 shrink-0 place-items-center rounded-full', kasbon ? 'bg-amber-100 text-amber-800 dark:bg-amber-950/40 dark:text-amber-200' : 'bg-emerald-100 text-emerald-800 dark:bg-emerald-950/40 dark:text-emerald-200')}><Icon className="size-4" /></span>
        <span className="min-w-0 flex-1">
          <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <strong className="text-sm">{request.requester_name}</strong>
            <span className="text-xs text-muted-foreground">{request.number} · {typeLabels[request.type]}</span>
            {refs.length > 0 && <Badge className="border-transparent bg-amber-100 text-amber-900 dark:bg-amber-950/40 dark:text-amber-200">Pernah masuk {refs[0].number} · {formatDateTime(refs[0].created_at)}</Badge>}
          </span>
          <span className="block truncate text-xs text-muted-foreground">{[request.description, request.regency_name, `Selesai ${formatDateTime(request.done_at)}`].filter(Boolean).join(' · ')}</span>
        </span>
        <strong className="text-sm whitespace-nowrap">{formatRupiah(request.total_amount)}</strong>
      </label>
      <Button type="button" variant="ghost" size="sm" aria-expanded={open} onClick={() => setOpen(!open)}><Paperclip />Lampiran<ChevronDown className={cn('transition-transform', open && 'rotate-180')} /></Button>
    </div>
    {open && <RequestAttachments id={request.id} />}
  </li>;
}

/** Baru dimuat saat dibuka, supaya daftar tetap ringan. */
function RequestAttachments({ id }: { id: string }) {
  const request = useQuery({ queryKey: ['finance', 'request', id], queryFn: () => fetchRequest(id) });
  const files = request.data?.data.attachments ?? [];
  const groups = [...new Set(files.map((file) => kindLabels[file.kind]))];
  return <div className="border-t px-4 py-3">
    {request.isPending && <p className="text-sm text-muted-foreground">Memuat lampiran...</p>}
    {request.isError && <p className="text-sm text-destructive">Lampiran belum dapat dimuat.</p>}
    {request.isSuccess && files.length === 0 && <p className="text-sm text-muted-foreground">Tidak ada lampiran.</p>}
    {groups.length > 0 && <div className="flex flex-wrap gap-x-6 gap-y-3">{groups.map((label) => <div key={label} className="grid gap-1.5">
      <p className="text-xs font-medium text-muted-foreground">{label}</p>
      <ProofThumbs request={{ id }} files={files.filter((file) => kindLabels[file.kind] === label)} />
    </div>)}</div>}
  </div>;
}
