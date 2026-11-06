import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowLeft, Check, Eye, FileUp, Save, Trash2 } from 'lucide-react';
import { useEffect, useId, useRef, useState } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { toast } from 'sonner';
import { DataState } from '@/components/DataState';
import { FormField } from '@/components/FormField';
import { PageHeader } from '@/components/PageHeader';
import { AlertDialog, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Combobox, ComboboxContent, ComboboxEmpty, ComboboxInput, ComboboxItem, ComboboxList } from '@/components/ui/combobox';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { ApiError, apiRequest } from '@/lib/api';
import { useCan } from '@/lib/permissions';
import { cn } from '@/lib/utils';
import {
  actionLabels, attachmentPath, blankItem, deleteAttachment, earliestSettlement, fetchProfile, fetchRequest, formatDate, formatDateTime, formatRupiah, kindLabels, noteActions,
  runAction, saveDraft, saveSettlement, stageIndex, stageLabels, statusLabels, today, typeLabels, uploadAttachment, type DraftInput, type FinanceRequest, type Item, type Option, type RequestType, type Team,
} from './api';
import { PayeePicker } from './PayeePicker';
import { FilePreviewButton } from './FilePreviewDialog';
import { errorMessage, FinanceStatus, ItemsEditor, ProofThumbs } from './shared';
import { VoucherCard } from './VoucherCards';

/** Halaman daftar yang bisa dibuka akun ini. */
function useListPath() {
  const canRequest = useCan('finance.request');
  const canReview = useCan('finance.review');
  if (canRequest) return '/keuangan';
  return canReview ? '/keuangan-review' : '/keuangan-monitoring';
}

const textarea = 'min-h-24 w-full rounded-lg border border-input bg-transparent px-3 py-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:opacity-60';

/** Tim dan kabupaten terisi sendiri bila pilihannya hanya satu. */
function blankDraft(type: RequestType, team?: Team): DraftInput {
  return { type, team_id: team?.id ?? '', regency_id: team?.regencies.length === 1 ? team.regencies[0].id : '', needed_date: today(), category: '', description: '', payee_name: '', bank_name: '', account_number: '', account_name: '', invoice_number: '', settlement_due: type === 'kasbon' ? earliestSettlement(today()) : '', items: [blankItem()] };
}

function draftFrom(request: FinanceRequest): DraftInput {
  const { type, team_id, regency_id, needed_date, category, description, payee_name, bank_name, account_number, account_name, invoice_number, settlement_due } = request;
  return { type, team_id, regency_id, needed_date, category, description, payee_name, bank_name, account_number, account_name, invoice_number, settlement_due, items: request.items?.length ? request.items : [blankItem()] };
}

export function RequestDetailPage() {
  const { id = '' } = useParams();
  const [search] = useSearchParams();
  const newType = search.get('baru') as RequestType | null;
  const request = useQuery({
    queryKey: ['finance', 'request', id],
    queryFn: () => fetchRequest(id),
    retry: (count, error) => !(error instanceof ApiError && error.status === 404) && count < 2,
  });
  const profile = useQuery({ queryKey: ['finance', 'profile'], queryFn: fetchProfile });

  if (request.isPending || profile.isPending) return <DataState kind="loading" title="Memuat pengajuan" description="Mengambil data pengajuan." />;
  const notFound = request.error instanceof ApiError && request.error.status === 404;
  if (notFound && newType && profile.data) {
    return <DraftForm key={id} id={id} initial={blankDraft(newType, profile.data.data.leader_teams[0])} />;
  }
  if (!request.data) return <DataState kind="error" title="Pengajuan tidak ditemukan" description="Pengajuan tidak ada, atau Anda tidak punya akses." />;
  const data = request.data.data;
  return data.editable ? <DraftForm key={id} id={id} initial={draftFrom(data)} request={data} /> : <RequestView request={data} />;
}

type DraftFormProps = { id: string; initial: DraftInput; request?: FinanceRequest };

/** Formulir draf: tersimpan otomatis setelah jeda mengetik. */
function DraftForm({ id, initial, request }: DraftFormProps) {
  const listPath = useListPath();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [form, setForm] = useState(initial);
  const [savedAt, setSavedAt] = useState<string | null>(null);
  const [confirmRemove, setConfirmRemove] = useState(false);
  const edited = useRef(false);
  const profile = useQuery({ queryKey: ['finance', 'profile'], queryFn: fetchProfile });
  const teams = profile.data?.data.leader_teams ?? [];
  const team = teams.find((item) => item.id === form.team_id);

  const save = useMutation({
    mutationFn: (input: DraftInput) => saveDraft(id, input),
    onSuccess: (result) => {
      queryClient.setQueryData(['finance', 'request', id], result);
      setSavedAt(new Date().toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit' }));
    },
    onError: (error) => toast.error(`Draf belum tersimpan: ${errorMessage(error)}`),
  });
  const saveLatest = save.mutate;
  useEffect(() => {
    if (!edited.current) return;
    const timer = window.setTimeout(() => saveLatest(form), 800);
    return () => window.clearTimeout(timer);
  }, [form, saveLatest]);

  const submit = useMutation({
    mutationFn: async () => {
      await saveDraft(id, form);
      return runAction(id, 'submit');
    },
    onSuccess: () => { edited.current = false; void queryClient.invalidateQueries({ queryKey: ['finance'] }); toast.success('Pengajuan terkirim ke finance.'); navigate('/keuangan'); },
    onError: (error) => toast.error(errorMessage(error)),
  });
  const remove = useMutation({
    mutationFn: () => apiRequest(`/api/v1/finance/requests/${id}`, { method: 'DELETE' }),
    onSuccess: () => { edited.current = false; void queryClient.invalidateQueries({ queryKey: ['finance'] }); toast.success('Draf dihapus.'); navigate('/keuangan'); },
    onError: (error) => toast.error(errorMessage(error)),
  });

  const set = (patch: Partial<DraftInput>) => { edited.current = true; setForm((value) => ({ ...value, ...patch })); };
  const text = (key: keyof DraftInput) => ({ value: String(form[key] ?? ''), onChange: (event: { target: { value: string } }) => set({ [key]: event.target.value }) });
  const isNew = !request;

  return <div className="space-y-6">
    <PageHeader
      title={request?.number || `Pengajuan ${typeLabels[form.type]} baru`}
      description={savedAt ? `Draf tersimpan pukul ${savedAt}.` : 'Isian tersimpan otomatis sebagai draf.'}
      actions={<>
        {request?.status === 'draft' && <Button variant="outline" size="icon" aria-label="Hapus draf" onClick={() => setConfirmRemove(true)}><Trash2 /></Button>}
        <Button variant="outline" nativeButton={false} render={<Link to={listPath} />}><ArrowLeft />Kembali</Button>
      </>}
    />
    {request?.review_note && <p role="status" className="rounded-lg border border-amber-300/60 bg-amber-50 px-3 py-2 text-sm text-amber-950 dark:bg-amber-950/20 dark:text-amber-100"><strong>Catatan finance:</strong> {request.review_note}</p>}
    <Card>
      <CardHeader className="border-b"><CardTitle className="text-base">Data pengajuan</CardTitle></CardHeader>
      <CardContent className="grid gap-4 pt-5 sm:grid-cols-2">
        {teams.length > 1 ? <div className="grid gap-2">
          <Label id="finance-team-label">Tim</Label>
          <Select value={form.team_id} onValueChange={(value) => set({ team_id: value ?? '', regency_id: '' })}>
            <SelectTrigger className="w-full" aria-labelledby="finance-team-label"><SelectValue placeholder="Pilih tim">{team?.name}</SelectValue></SelectTrigger>
            <SelectContent>{teams.map((item) => <SelectItem key={item.id} value={item.id}>{item.name}</SelectItem>)}</SelectContent>
          </Select>
        </div> : <Line label="Tim" value={team?.name ?? ''} />}
        {(team?.regencies.length ?? 0) > 0 && <div className="grid gap-2">
          <Label id="finance-regency-label">Kabupaten</Label>
          <Combobox
            items={team?.regencies ?? []}
            itemToStringLabel={(item: Option) => item.name}
            isItemEqualToValue={(item: Option, value: Option) => item.id === value.id}
            value={team?.regencies.find((item) => item.id === form.regency_id) ?? null}
            onValueChange={(item: Option | null) => set({ regency_id: item?.id ?? '' })}
          >
            <ComboboxInput className="w-full" aria-labelledby="finance-regency-label" placeholder="Cari kabupaten" />
            <ComboboxContent>
              <ComboboxEmpty>Kabupaten tidak ditemukan.</ComboboxEmpty>
              <ComboboxList>{(item: Option) => <ComboboxItem key={item.id} value={item}>{item.name}</ComboboxItem>}</ComboboxList>
            </ComboboxContent>
          </Combobox>
        </div>}
        <div className="grid gap-2">
          <Label id="finance-category-label">Jenis biaya</Label>
          <Select value={form.category} onValueChange={(value) => set({ category: value ?? '' })}>
            <SelectTrigger className="w-full" aria-labelledby="finance-category-label"><SelectValue placeholder="Pilih jenis biaya" /></SelectTrigger>
            <SelectContent>{profile.data?.data.categories.map((item) => <SelectItem key={item} value={item}>{item}</SelectItem>)}</SelectContent>
          </Select>
        </div>
        <FormField label="Kapan dana dibutuhkan" name="needed_date" type="date" required value={form.needed_date} onChange={(event) => {
          const needed = event.target.value;
          const earliest = needed ? earliestSettlement(needed) : '';
          set({ needed_date: needed, settlement_due: form.type === 'kasbon' && form.settlement_due < earliest ? earliest : form.settlement_due });
        }} />
        {form.type === 'kasbon' && <FormField label="Batas lapor kasbon" name="settlement_due" type="date" required min={form.needed_date ? earliestSettlement(form.needed_date) : undefined} hint="Minimal 3 hari setelah dana dibutuhkan." {...text('settlement_due')} />}
        <div className="grid gap-2 sm:col-span-2">
          <Label htmlFor="finance-description">Untuk keperluan apa?</Label>
          <textarea id="finance-description" className={textarea} maxLength={2000} placeholder="Contoh: sewa mobil untuk pengiriman ke Wonogiri" {...text('description')} />
        </div>
      </CardContent>
    </Card>
    {form.type === 'cashless' && <Card>
      <CardHeader className="border-b"><CardTitle className="text-base">Rekening tujuan transfer</CardTitle></CardHeader>
      <CardContent className="pt-5">
        <PayeePicker teamID={form.team_id} value={{ account_name: form.account_name || form.payee_name, bank_name: form.bank_name, account_number: form.account_number }} onChange={({ account_name, bank_name, account_number }) => set({ payee_name: account_name, account_name, bank_name, account_number })} />
      </CardContent>
    </Card>}
    <Card>
      <CardHeader className="border-b"><CardTitle className="text-base">Daftar biaya</CardTitle></CardHeader>
      <CardContent className="pt-5"><ItemsEditor items={form.items} proofs={request ? { request, beforeUpload: () => saveDraft(id, form) } : undefined} onChange={(items) => set({ items })} /></CardContent>
    </Card>
    {isNew
      ? <p className="text-sm text-muted-foreground">Nota dan dokumen pendukung bisa ditambahkan setelah draf tersimpan.</p>
      : <AttachmentsCard request={request} kinds={['evidence']} />}
    <div className="flex flex-wrap justify-end gap-2">
      <Button variant="outline" disabled={save.isPending} onClick={() => { edited.current = true; save.mutate(form); }}><Save />Simpan draf</Button>
      <Button disabled={submit.isPending} onClick={() => submit.mutate()}>{submit.isPending ? 'Mengirim...' : 'Kirim pengajuan'}</Button>
    </div>
    <AlertDialog open={confirmRemove} onOpenChange={setConfirmRemove}><AlertDialogContent aria-label="Hapus draf">
      <AlertDialogHeader><AlertDialogTitle>Hapus draf ini?</AlertDialogTitle><AlertDialogDescription>Isian dan lampirannya ikut terhapus dan tidak bisa dikembalikan.</AlertDialogDescription></AlertDialogHeader>
      <AlertDialogFooter><AlertDialogCancel>Batal</AlertDialogCancel><Button variant="destructive" disabled={remove.isPending} onClick={() => remove.mutate()}>{remove.isPending ? 'Menghapus...' : 'Hapus draf'}</Button></AlertDialogFooter>
    </AlertDialogContent></AlertDialog>
  </div>;
}

function Line({ label, value }: { label: string; value: string }) {
  return <div className="min-w-0"><dt className="text-xs text-muted-foreground">{label}</dt><dd className="text-sm break-words">{value || '-'}</dd></div>;
}

function ItemsTable({ request, items }: { request: FinanceRequest; items: Item[] }) {
  return <ul className="grid gap-2">{items.map((item) => <li key={item.key} className="grid gap-2 rounded-lg border p-3 text-sm">
    <span className="flex flex-wrap justify-between gap-2">
      <span className="min-w-0 break-words">{item.description} <span className="text-muted-foreground">({item.quantity} × {formatRupiah(item.unit_price)}{item.entry === 'kredit' ? ', potongan' : ''}{item.note ? `, ${item.note}` : ''})</span></span>
      <strong>{item.entry === 'kredit' ? '-' : ''}{formatRupiah(item.amount ?? item.quantity * item.unit_price)}</strong>
    </span>
    <ProofThumbs request={request} files={(request.attachments ?? []).filter((file) => file.item_key === item.key)} />
  </li>)}</ul>;
}

/** Posisi pengajuan di alurnya. Ditolak dan dibatalkan tidak punya tahap. */
function StageTrack({ request }: { request: FinanceRequest }) {
  const current = stageIndex(request.type, request.status);
  if (current === null) return null;
  return <ol className="flex gap-1" aria-label="Tahap pengajuan">{stageLabels[request.type].map((label, index) => <li key={label} aria-current={index === current ? 'step' : undefined} className="min-w-0 flex-1">
    <span className={cn('block h-1.5 rounded-full', index <= current ? 'bg-primary' : 'bg-muted')} />
    <span className={cn('mt-1 flex items-center gap-1 truncate text-xs', index === current ? 'font-semibold' : 'text-muted-foreground')}>{index < current && <Check aria-hidden="true" className="size-3 shrink-0 text-primary" />}{label}</span>
  </li>)}</ol>;
}

function RequestView({ request }: { request: FinanceRequest }) {
  const listPath = useListPath();
  const queryClient = useQueryClient();
  const [pending, setPending] = useState<string | null>(null);
  const [note, setNote] = useState('');
  const act = useMutation({
    mutationFn: ({ action, value }: { action: string; value: string }) => runAction(request.id, action, value),
    onSuccess: (result) => {
      queryClient.setQueryData(['finance', 'request', request.id], result);
      void queryClient.invalidateQueries({ queryKey: ['finance'] });
      setPending(null); setNote('');
      toast.success(`Status: ${statusLabels[result.data.status]}.`);
    },
    onError: (error) => toast.error(errorMessage(error)),
  });
  const actions = request.actions ?? [];
  const uploads = request.upload_kinds ?? [];
  const canSettle = uploads.includes('settlement');
  const difference = request.settlement_amount === null ? null : request.total_amount - request.settlement_amount;

  return <div className="space-y-6">
    <PageHeader
      title={request.number || 'Pengajuan'}
      description={`${typeLabels[request.type]} · ${request.team_name} · ${request.requester_name}`}
      context={<div className="pt-1"><FinanceStatus status={request.status} /></div>}
      actions={<Button variant="outline" nativeButton={false} render={<Link to={listPath} />}><ArrowLeft />Kembali</Button>}
    />
    {request.review_note && <p role="status" className="rounded-lg border bg-muted/40 px-3 py-2 text-sm"><strong>Catatan finance:</strong> {request.review_note}</p>}
    <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_22rem]">
      <div className="min-w-0 space-y-5">
        <Card>
          <CardContent className="grid gap-5 pt-5">
            <div className="flex flex-wrap items-end justify-between gap-3">
              <div>
                <p className="text-xs text-muted-foreground">Total pengajuan</p>
                <p className="text-2xl font-bold">{formatRupiah(request.total_amount)}</p>
              </div>
              <Badge variant="outline">{typeLabels[request.type]}</Badge>
            </div>
            <StageTrack request={request} />
            <dl className="grid gap-3 border-t pt-4 sm:grid-cols-2">
              <Line label="Pemohon" value={request.requester_name} />
              <Line label="Tim" value={request.team_name} />
              <Line label="Jenis biaya" value={request.category} />
              <Line label="Dana dibutuhkan" value={formatDate(request.needed_date)} />
              <Line label="Kabupaten" value={request.regency_name} />
              {request.type === 'kasbon' && <Line label="Batas lapor kasbon" value={formatDate(request.settlement_due)} />}
              {request.type === 'cashless' && <>
                <Line label="Pemilik rekening" value={request.account_name || request.payee_name} />
                <Line label="Rekening tujuan" value={`${request.bank_name} ${request.account_number}`} />
              </>}
              <div className="sm:col-span-2"><Line label="Keperluan" value={request.description} /></div>
            </dl>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="border-b"><CardTitle className="text-base">Daftar biaya</CardTitle></CardHeader>
          <CardContent className="pt-5"><ItemsTable request={request} items={request.items ?? []} /></CardContent>
        </Card>
        {canSettle && <SettlementForm request={request} />}
        {!canSettle && (request.settlement_items?.length ?? 0) > 0 && <Card>
          <CardHeader className="border-b"><div className="flex flex-wrap items-center justify-between gap-2"><CardTitle className="text-base">Laporan kasbon</CardTitle><strong>{formatRupiah(request.settlement_amount ?? 0)}</strong></div></CardHeader>
          <CardContent className="grid gap-3 pt-5">
            <ItemsTable request={request} items={request.settlement_items ?? []} />
            {difference !== null && <p className="text-sm">{difference >= 0 ? `Sisa kasbon ${formatRupiah(difference)}` : `Kurang bayar ${formatRupiah(-difference)}`}</p>}
            {request.settlement_note && <p className="text-sm text-muted-foreground">{request.settlement_note}</p>}
          </CardContent>
        </Card>}
        <AttachmentsCard request={request} kinds={['evidence', 'payment', 'settlement']} />
      </div>
      <div className="space-y-5">
        <VoucherCard request={request} />
        {actions.length > 0 && <Card>
          <CardHeader className="border-b"><CardTitle className="text-base">Tindakan</CardTitle></CardHeader>
          <CardContent className="grid gap-2 pt-5">
            {actions.map((action) => <Button key={action} variant={action === 'reject' || action === 'cancel' ? 'outline' : 'default'} disabled={act.isPending} onClick={() => setPending(action)}>{actionLabels[action]}</Button>)}
            {actions.includes('pay') && <p className="text-xs text-muted-foreground">Unggah bukti bayar dulu sebelum menandai dibayar.</p>}
          </CardContent>
        </Card>}
        <Card>
          <CardHeader className="border-b"><CardTitle className="text-base">Riwayat</CardTitle></CardHeader>
          <CardContent className="pt-5">
            {(request.events?.length ?? 0) === 0 ? <p className="text-sm text-muted-foreground">Belum ada riwayat.</p>
              : <ol className="grid gap-4 border-l pl-4">{request.events?.map((event, index) => <li key={index} className="relative text-sm">
                <span aria-hidden="true" className="absolute top-1.5 -left-[1.3rem] size-2 rounded-full bg-primary" />
                <strong>{actionLabels[event.action] ?? event.action}</strong>
                <span className="block text-xs text-muted-foreground">{event.actor_name} · {formatDateTime(event.created_at)}</span>
                {event.note && <span className="block text-muted-foreground">{event.note}</span>}
              </li>)}</ol>}
          </CardContent>
        </Card>
      </div>
    </div>
    <AlertDialog open={pending !== null} onOpenChange={(open) => { if (!open) setPending(null); }}><AlertDialogContent aria-label="Konfirmasi tindakan">
      <AlertDialogHeader><AlertDialogTitle>{pending ? actionLabels[pending] : ''}?</AlertDialogTitle><AlertDialogDescription>{request.number} · {formatRupiah(request.total_amount)}</AlertDialogDescription></AlertDialogHeader>
      {pending && <div className="grid gap-2">
        <Label htmlFor="finance-action-note">Catatan{noteActions.has(pending) ? ' (wajib)' : ' (opsional)'}</Label>
        <textarea id="finance-action-note" className={textarea} maxLength={2000} value={note} onChange={(event) => setNote(event.target.value)} />
      </div>}
      <AlertDialogFooter><AlertDialogCancel>Kembali</AlertDialogCancel><Button disabled={act.isPending || (pending !== null && noteActions.has(pending) && note.trim() === '')} onClick={() => pending && act.mutate({ action: pending, value: note })}>{act.isPending ? 'Memproses...' : 'Lanjutkan'}</Button></AlertDialogFooter>
    </AlertDialogContent></AlertDialog>
  </div>;
}

function SettlementForm({ request }: { request: FinanceRequest }) {
  const queryClient = useQueryClient();
  const [items, setItems] = useState<Item[]>(() => request.settlement_items?.length ? request.settlement_items : (request.items ?? [blankItem()]).map((item) => ({ ...item })));
  const [note, setNote] = useState(request.settlement_note);
  const save = useMutation({
    mutationFn: () => saveSettlement(request.id, note, items),
    onSuccess: (result) => { queryClient.setQueryData(['finance', 'request', request.id], result); toast.success('Laporan tersimpan.'); },
    onError: (error) => toast.error(errorMessage(error)),
  });
  return <Card>
    <CardHeader className="border-b"><CardTitle className="text-base">Laporan kasbon</CardTitle></CardHeader>
    <CardContent className="grid gap-4 pt-5">
      <ItemsEditor items={items} proofs={{ request, beforeUpload: () => saveSettlement(request.id, note, items) }} onChange={setItems} />
      <div className="grid gap-2">
        <Label htmlFor="finance-settlement-note">Catatan</Label>
        <textarea id="finance-settlement-note" className={textarea} maxLength={2000} value={note} onChange={(event) => setNote(event.target.value)} />
      </div>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-xs text-muted-foreground">Unggah nota tiap biaya, simpan laporan, lalu tekan Kirim laporan.</p>
        <Button variant="outline" disabled={save.isPending} onClick={() => save.mutate()}><Save />Simpan laporan</Button>
      </div>
    </CardContent>
  </Card>;
}

function AttachmentsCard({ request, kinds }: { request: FinanceRequest; kinds: string[] }) {
  const queryClient = useQueryClient();
  const inputID = useId();
  const uploadKinds = request.upload_kinds ?? [];
  const uploadKind = kinds.find((kind) => uploadKinds.includes(kind));
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['finance', 'request', request.id] });
  const upload = useMutation({
    mutationFn: (file: File) => uploadAttachment(request.id, uploadKind ?? '', file),
    onSuccess: () => { void refresh(); toast.success('Lampiran tersimpan.'); },
    onError: (error) => toast.error(errorMessage(error)),
  });
  const remove = useMutation({
    mutationFn: (attachmentID: string) => deleteAttachment(request.id, attachmentID),
    onSuccess: () => void refresh(),
    onError: (error) => toast.error(errorMessage(error)),
  });
  const files = (request.attachments ?? []).filter((file) => kinds.includes(file.kind) && !file.item_key);
  return <Card>
    <CardHeader className="border-b"><div className="flex flex-wrap items-center justify-between gap-2">
      <CardTitle className="text-base">{kinds.length === 1 ? 'Dokumen pendukung (opsional)' : 'Dokumen pendukung'}</CardTitle>
      {uploadKind && <>
        <input id={inputID} type="file" accept="image/jpeg,image/png,application/pdf" className="sr-only" disabled={upload.isPending} onChange={(event) => { const file = event.target.files?.[0]; event.target.value = ''; if (file) upload.mutate(file); }} />
        <Button variant="outline" disabled={upload.isPending} nativeButton={false} render={<label htmlFor={inputID} />}><FileUp />{upload.isPending ? 'Mengunggah...' : `Unggah ${kindLabels[uploadKind].toLowerCase()}`}</Button>
      </>}
    </div></CardHeader>
    <CardContent className="pt-5">
      {files.length === 0 ? <p className="text-sm text-muted-foreground">Contoh: penawaran atau kontrak. JPG, PNG, atau PDF, maksimal 15 MB.</p>
        : <ul className="grid gap-2">{files.map((file) => <li key={file.id} className="flex flex-wrap items-center justify-between gap-2 rounded-lg border p-3">
          <span className="min-w-0"><Badge variant="outline">{kindLabels[file.kind]}</Badge><span className="mt-1 block truncate text-xs text-muted-foreground">{file.filename}</span></span>
          <span className="flex gap-1">
            <FilePreviewButton path={attachmentPath(request.id, file.id)} filename={file.filename} variant="ghost" size="sm"><Eye />Lihat</FilePreviewButton>
            {uploadKinds.includes(file.kind) && <Button variant="ghost" size="icon-sm" aria-label={`Hapus ${file.filename}`} disabled={remove.isPending} onClick={() => remove.mutate(file.id)}><Trash2 /></Button>}
          </span>
        </li>)}</ul>}
    </CardContent>
  </Card>;
}
