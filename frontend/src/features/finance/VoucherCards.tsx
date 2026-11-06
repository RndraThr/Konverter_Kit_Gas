import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Eye, FileText, ImageUp, Plus, Trash2 } from 'lucide-react';
import { useId, useState } from 'react';
import { Link } from 'react-router-dom';
import { toast } from 'sonner';
import { FormField } from '@/components/FormField';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { apiRequest } from '@/lib/api';
import { useCan } from '@/lib/permissions';
import { fetchVoucherSettings, financeKey, formatDate, formatDateTime, voucherFilePath, type Account, type AccountKind, type DataResponse, type FinanceRequest, type VoucherSettings } from './api';
import { FilePreviewButton } from './FilePreviewDialog';
import { errorMessage } from './shared';

const settingsKey = [...financeKey, 'voucher-settings'];

/** Voucher yang memuat pengajuan ini. Penerbitannya ada di halaman Voucher. */
export function VoucherCard({ request }: { request: FinanceRequest }) {
  const canIssue = useCan('finance.review');
  const vouchers = request.vouchers ?? [];
  if (vouchers.length === 0 && !(canIssue && request.status === 'done')) return null;

  return <Card>
    <CardHeader className="border-b"><CardTitle className="flex items-center gap-2 text-base"><FileText className="size-4 text-primary" aria-hidden="true" />Voucher</CardTitle></CardHeader>
    <CardContent className="grid gap-2 pt-5">
      {vouchers.map((voucher) => <div key={voucher.id} className="flex items-center justify-between gap-2 rounded-lg border p-3">
        <span className="min-w-0">
          <strong className="text-sm">{voucher.number}</strong>
          <span className="block text-xs text-muted-foreground">{formatDate(voucher.date)} · dibuat {formatDateTime(voucher.created_at)}</span>
        </span>
        <FilePreviewButton path={voucherFilePath(voucher.id)} filename={`Voucher_Bundle_${voucher.number}.pdf`} variant="ghost" size="sm"><Eye />Lihat</FilePreviewButton>
      </div>)}
      {canIssue && request.status === 'done' && <Button variant="outline" nativeButton={false} render={<Link to="/keuangan-voucher" />}>Buat voucher</Button>}
    </CardContent>
  </Card>;
}

const accountGroups: { kind: AccountKind; title: string; placeholder: string }[] = [
  { kind: 'bank', title: 'Bank', placeholder: 'BSI Operasional' },
  { kind: 'kas', title: 'Kas', placeholder: 'GoPay' },
  { kind: 'lainnya', title: 'Lainnya', placeholder: 'Hutang Reimburse KSO' },
];

/** Kop dan rekening yang tercetak di voucher. */
export function VoucherSettingsCard() {
  const settings = useQuery({ queryKey: settingsKey, queryFn: fetchVoucherSettings });
  if (!settings.data) return null;
  return <VoucherSettingsForm key={settings.dataUpdatedAt} initial={settings.data.data} />;
}

function VoucherSettingsForm({ initial }: { initial: VoucherSettings }) {
  const queryClient = useQueryClient();
  const inputID = useId();
  const [form, setForm] = useState({ ...initial, accounts: initial.accounts.filter((account) => account.is_active).map((account) => ({ ...account, rowId: account.id })) });
  const refresh = () => queryClient.invalidateQueries({ queryKey: settingsKey });
  const save = useMutation({
    mutationFn: () => apiRequest<DataResponse<VoucherSettings>>('/api/v1/finance/voucher-settings', { method: 'PUT', body: JSON.stringify(form) }),
    onSuccess: () => { void refresh(); toast.success('Pengaturan voucher tersimpan.'); },
    onError: (error) => toast.error(errorMessage(error)),
  });
  const logo = useMutation({
    mutationFn: (file: File) => {
      const body = new FormData();
      body.append('file', file, file.name);
      return apiRequest('/api/v1/finance/voucher-settings/logo', { method: 'POST', body });
    },
    onSuccess: () => { void refresh(); toast.success('Logo tersimpan.'); },
    onError: (error) => toast.error(errorMessage(error)),
  });
  const setAccount = (rowId: string, patch: Partial<Account>) => setForm({ ...form, accounts: form.accounts.map((account) => account.rowId === rowId ? { ...account, ...patch } : account) });
  const addAccount = (kind: AccountKind) => setForm({ ...form, accounts: [...form.accounts, { id: '', kind, name: '', account_number: '', is_active: true, rowId: crypto.randomUUID() }] });

  return <Card>
    <CardHeader className="border-b"><CardTitle className="text-base">Kop dan rekening voucher</CardTitle></CardHeader>
    <CardContent className="grid gap-4 pt-5">
      <FormField label="Nama perusahaan" name="voucher_company" required maxLength={200} value={form.company_name} onChange={(event) => setForm({ ...form, company_name: event.target.value })} />
      <div className="grid gap-4 sm:grid-cols-3">
        <FormField label="Alamat baris 1" name="voucher_address1" maxLength={200} value={form.address_line1} onChange={(event) => setForm({ ...form, address_line1: event.target.value })} />
        <FormField label="Alamat baris 2" name="voucher_address2" maxLength={200} value={form.address_line2} onChange={(event) => setForm({ ...form, address_line2: event.target.value })} />
        <FormField label="Alamat baris 3" name="voucher_address3" maxLength={200} value={form.address_line3} onChange={(event) => setForm({ ...form, address_line3: event.target.value })} />
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <input id={inputID} type="file" accept="image/png,image/jpeg" className="sr-only" disabled={logo.isPending} onChange={(event) => { const file = event.target.files?.[0]; event.target.value = ''; if (file) logo.mutate(file); }} />
        <img src="/api/v1/finance/voucher-settings/logo" alt="Logo voucher" className="h-10 w-auto" />
        <Button variant="outline" disabled={logo.isPending} nativeButton={false} render={<label htmlFor={inputID} />}><ImageUp />Ganti logo</Button>
        <span className="text-xs text-muted-foreground">PNG atau JPG, maksimal 2 MB.</span>
      </div>
      {accountGroups.map(({ kind, title, placeholder }) => <fieldset key={kind} className="grid gap-2 border-0 p-0">
        <legend className="mb-1 text-sm font-semibold">{title}</legend>
        {form.accounts.filter((account) => account.kind === kind).map((account) => <div key={account.rowId} className="grid gap-2 rounded-lg border p-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] sm:items-end">
          <label className="grid gap-1 text-xs font-medium">Nama<Input value={account.name} maxLength={80} placeholder={placeholder} onChange={(event) => setAccount(account.rowId, { name: event.target.value })} /></label>
          {kind === 'bank' ? <label className="grid gap-1 text-xs font-medium">Nomor rekening<Input value={account.account_number} maxLength={40} onChange={(event) => setAccount(account.rowId, { account_number: event.target.value })} /></label> : <span />}
          <Button type="button" variant="ghost" size="icon" aria-label={`Hapus ${account.name || title}`} onClick={() => setForm({ ...form, accounts: form.accounts.filter((item) => item.rowId !== account.rowId) })}><Trash2 /></Button>
        </div>)}
        <div><Button type="button" variant="outline" onClick={() => addAccount(kind)}><Plus />Tambah {title.toLowerCase()}</Button></div>
      </fieldset>)}
      <div className="flex justify-end"><Button disabled={save.isPending || !form.company_name.trim()} onClick={() => save.mutate()}>{save.isPending ? 'Menyimpan...' : 'Simpan pengaturan'}</Button></div>
    </CardContent>
  </Card>;
}
