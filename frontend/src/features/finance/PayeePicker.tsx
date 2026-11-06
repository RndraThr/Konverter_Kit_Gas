import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Check, Plus, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { toast } from 'sonner';
import { FormField } from '@/components/FormField';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { deletePayee, fetchPayees, savePayee, type Payee } from './api';
import { errorMessage } from './shared';

const blank = { account_name: '', bank_name: '', account_number: '' };
export type PayeeValue = typeof blank;

const sameAccount = (a: PayeeValue, b: PayeeValue) => a.bank_name === b.bank_name && a.account_number === b.account_number;
const validNumber = (value: string) => /^\d{5,34}$/.test(value.replace(/[\s-]/g, ''));

type PayeePickerProps = { teamID: string; value: PayeeValue; onChange: (payee: PayeeValue) => void };

/** Rekening tujuan: pilih yang pernah dipakai tim, atau tambah baru. */
export function PayeePicker({ teamID, value, onChange }: PayeePickerProps) {
  const queryClient = useQueryClient();
  const queryKey = ['finance', 'payees', teamID];
  const [adding, setAdding] = useState(false);
  const [form, setForm] = useState(blank);
  const payees = useQuery({ queryKey, queryFn: () => fetchPayees(teamID), enabled: teamID !== '' });
  const rows = payees.data?.data ?? [];
  const chosen = value.account_number !== '';
  const listed = rows.some((row) => sameAccount(row, value));

  const add = useMutation({
    mutationFn: () => savePayee(teamID, form),
    onSuccess: (result) => {
      void queryClient.invalidateQueries({ queryKey });
      onChange(result.data);
      setForm(blank);
      setAdding(false);
    },
    onError: (error) => toast.error(errorMessage(error)),
  });
  const remove = useMutation({
    mutationFn: (payee: Payee) => deletePayee(payee.id),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey }),
    onError: (error) => toast.error(errorMessage(error)),
  });
  const field = (key: keyof PayeeValue) => ({ value: form[key], onChange: (event: { target: { value: string } }) => setForm((current) => ({ ...current, [key]: event.target.value })) });
  const ready = form.account_name.trim() !== '' && form.bank_name.trim() !== '' && validNumber(form.account_number);

  return <div className="grid gap-3">
    {chosen && !listed && <AccountRow payee={value} selected />}
    {rows.length > 0 && <ul className="grid gap-2" aria-label="Rekening tersimpan">
      {rows.map((payee) => <li key={payee.id}>
        <AccountRow payee={payee} selected={sameAccount(payee, value)} onSelect={() => onChange(payee)} onRemove={() => remove.mutate(payee)} removing={remove.isPending} />
      </li>)}
    </ul>}
    {adding
      ? <div className="grid items-start gap-4 rounded-lg border p-3 sm:grid-cols-3">
        <FormField label="Nama pemilik rekening" name="account_name" required maxLength={200} placeholder="Sesuai buku tabungan" {...field('account_name')} />
        <FormField label="Bank" name="bank_name" required maxLength={200} placeholder="Contoh: BRI" {...field('bank_name')} />
        <FormField label="Nomor rekening" name="account_number" required inputMode="numeric" maxLength={40} {...field('account_number')} />
        <div className="flex flex-wrap items-end gap-2 sm:col-span-3">
          <Button type="button" disabled={!ready || add.isPending} onClick={() => add.mutate()}>{add.isPending ? 'Menyimpan...' : 'Simpan rekening'}</Button>
          <Button type="button" variant="ghost" onClick={() => { setForm(blank); setAdding(false); }}>Batal</Button>
        </div>
      </div>
      : <Button type="button" variant="outline" className="justify-self-start" disabled={teamID === ''} onClick={() => setAdding(true)}><Plus />Tambah rekening baru</Button>}
  </div>;
}

type AccountRowProps = { payee: PayeeValue; selected: boolean; onSelect?: () => void; onRemove?: () => void; removing?: boolean };

function AccountRow({ payee, selected, onSelect, onRemove, removing }: AccountRowProps) {
  const label = <span className="min-w-0">
    <strong className="block truncate text-sm">{payee.account_name}</strong>
    <span className="block truncate text-xs text-muted-foreground">{payee.bank_name} · {payee.account_number}</span>
  </span>;
  const frame = cn('flex min-w-0 flex-1 items-center justify-between gap-2 p-3 text-left');
  return <div className={cn('flex items-center gap-1 rounded-lg border', selected && 'border-primary bg-primary/5')}>
    {onSelect
      ? <button type="button" aria-pressed={selected} className={cn(frame, 'rounded-lg hover:bg-muted/50 focus-visible:outline-2 focus-visible:outline-ring')} onClick={onSelect}>{label}{selected && <Check aria-hidden="true" className="size-4 shrink-0 text-primary" />}</button>
      : <div className={frame}>{label}<Check aria-hidden="true" className="size-4 shrink-0 text-primary" /></div>}
    {onRemove && <Button type="button" variant="ghost" size="icon-sm" className="mr-1" aria-label={`Hapus rekening ${payee.account_name}`} disabled={removing} onClick={onRemove}><Trash2 /></Button>}
  </div>;
}
