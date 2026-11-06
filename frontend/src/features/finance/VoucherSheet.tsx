import { Plus, Trash2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { accountBoxLabel, accountLabel, formatRupiah, rebalance, type Account, type AccountKind, type SheetForm, type SheetLine, type VoucherSettings } from './api';
import { MoneyInput } from './MoneyInput';

const boxes: { kind: AccountKind; title: string }[] = [{ kind: 'bank', title: 'BANK' }, { kind: 'kas', title: 'KAS' }, { kind: 'lainnya', title: 'LAINNYA' }];
const signers = [['Prepared By,', 'Admin'], ['Checked By,', 'Tax'], ['Checked By,', 'Accounting'], ['Approved By,', 'Manager'], ['System Entered By,', 'Admin/Accounting']];

const field = 'w-full bg-transparent px-1 py-0.5 text-[0.8rem] text-black outline-none placeholder:text-neutral-400 focus:bg-amber-50';
const money = `${field} text-right`;

type VoucherSheetProps = { form: SheetForm; settings: VoucherSettings; onChange: (form: SheetForm) => void };

/** Lembar yang sama dengan PDF-nya: diisi langsung, baris bank mengikuti baris lain. */
export function VoucherSheet({ form, settings, onChange }: VoucherSheetProps) {
  const accounts = settings.accounts.filter((account) => account.is_active);
  const chosen = accounts.find((account) => account.id === form.account_id);
  const debit = form.lines.reduce((sum, line) => sum + line.debit, 0);
  const kredit = form.lines.reduce((sum, line) => sum + line.kredit, 0);
  const set = (patch: Partial<SheetForm>) => onChange({ ...form, ...patch });
  const setLines = (lines: SheetLine[]) => set({ lines: rebalance(lines) });
  const update = (rowId: string, patch: Partial<SheetLine>) => setLines(form.lines.map((line) => line.rowId === rowId ? { ...line, ...patch } : line));
  const addRow = () => setLines([...form.lines, { rowId: crypto.randomUUID(), item_key: '', description: '', debit: 0, kredit: 0, note: '', project_code: form.lines[0]?.project_code ?? '', is_bank: false }]);

  return <div className="overflow-x-auto rounded-lg bg-white py-4 pr-9 pl-4 font-[Arial,Helvetica,sans-serif] text-black">
    <div className="min-w-[60rem]">
      <div className="grid grid-cols-[8.5rem_minmax(0,1.5fr)_5.25rem_minmax(0,1fr)] border border-black">
        <div className="grid place-items-center border-r border-black p-2"><img src="/api/v1/finance/voucher-settings/logo" alt="Logo perusahaan" className="max-h-16 max-w-full object-contain" /></div>
        <div className="border-r border-black px-2.5 py-2">
          <p className="text-sm font-bold">{settings.company_name}</p>
          {[settings.address_line1, settings.address_line2, settings.address_line3].map((line) => <p key={line} className="text-xs leading-5">{line}</p>)}
        </div>
        <div className="grid grid-rows-2 border-r border-black text-xs font-bold">
          <span className="border-b border-black px-2.5 py-2">Date</span>
          <span className="px-2.5 py-2">No.</span>
        </div>
        <div className="grid grid-rows-2">
          <div className="border-b border-black px-1.5 py-1"><input type="date" aria-label="Tanggal voucher" className={field} value={form.date} onChange={(event) => set({ date: event.target.value })} /></div>
          <div className="px-1.5 py-1"><input aria-label="Nomor voucher" className={field} maxLength={40} placeholder="Isi nomor voucher" value={form.number} onChange={(event) => set({ number: event.target.value.toUpperCase().replace(/\s/g, '') })} /></div>
        </div>
      </div>

      <h2 className="py-2 text-center text-lg font-bold">PAYMENT VOUCHER</h2>

      <div className="grid grid-cols-3 border border-black">
        {boxes.map(({ kind, title }, index) => <div key={kind} className={cn('px-2.5 py-1.5', index > 0 && 'border-l border-black')}>
          <p className="mb-0.5 text-[0.625rem] font-bold">{title}</p>
          {accounts.filter((account) => account.kind === kind).map((account) => <AccountChoice key={account.id} account={account} selected={account.id === form.account_id} onSelect={() => set({ account_id: account.id })} />)}
        </div>)}
      </div>

      <div className="border-x border-b border-black">
        <SheetRow label="Pay To:"><input aria-label="Pay To" className={field} maxLength={200} placeholder="Nama penerima" value={form.pay_to} onChange={(event) => set({ pay_to: event.target.value })} /></SheetRow>
        <SheetRow label="Total Amount:"><span className="px-1 text-[0.8rem]">{formatRupiah(debit)}</span></SheetRow>
        <SheetRow label="Description:" last><input aria-label="Description" className={field} maxLength={2000} placeholder="Keterangan voucher" value={form.description} onChange={(event) => set({ description: event.target.value })} /></SheetRow>
      </div>

      <table className="mt-3 w-full table-fixed border-collapse text-[0.8rem]">
        <colgroup><col className="w-[4.5%]" /><col className="w-[44%]" /><col className="w-[13%]" /><col className="w-[11.5%]" /><col className="w-[15.5%]" /><col className="w-[11.5%]" /></colgroup>
        <thead><tr>{['NO.', 'DESCRIPTION', 'DEBIT', 'KREDIT', 'NOTE', 'PROJECT CODE'].map((title) => <th key={title} scope="col" className="border border-black py-1 text-center text-xs font-bold">{title}</th>)}</tr></thead>
        <tbody>
          {form.lines.map((line, index) => <tr key={line.rowId}>
            <td className="border border-black text-center">{index + 1}</td>
            <td className="border border-black">{line.is_bank
              ? <span className="block truncate px-1">{chosen ? accountLabel(chosen) : 'Pilih rekening di atas'}</span>
              : <input aria-label={`Description baris ${index + 1}`} className={field} maxLength={200} value={line.description} onChange={(event) => update(line.rowId, { description: event.target.value })} />}</td>
            <td className="border border-black"><MoneyInput plain aria-label={`Debit baris ${index + 1}`} className={money} value={line.debit} onChange={(debit) => update(line.rowId, { debit, kredit: debit > 0 ? 0 : line.kredit, manual: line.is_bank })} /></td>
            <td className="border border-black"><MoneyInput plain aria-label={`Kredit baris ${index + 1}`} className={money} value={line.kredit} onChange={(kredit) => update(line.rowId, { kredit, debit: kredit > 0 ? 0 : line.debit, manual: line.is_bank })} /></td>
            <td className="border border-black"><input aria-label={`Note baris ${index + 1}`} className={field} maxLength={200} value={line.note} onChange={(event) => update(line.rowId, { note: event.target.value })} /></td>
            <td className="relative border border-black">
              <input aria-label={`Project code baris ${index + 1}`} className={field} maxLength={200} value={line.project_code} onChange={(event) => update(line.rowId, { project_code: event.target.value })} />
              {!line.is_bank && <button type="button" className="absolute top-1/2 left-full ml-1.5 -translate-y-1/2 rounded-sm p-0.5 text-neutral-500 hover:text-destructive focus-visible:outline-2 focus-visible:outline-ring" aria-label={`Hapus baris ${index + 1}`} onClick={() => setLines(form.lines.filter((row) => row.rowId !== line.rowId))}><Trash2 aria-hidden="true" className="size-4" /></button>}
            </td>
          </tr>)}
          <tr className="font-bold">
            <td className="border border-black" />
            <td className="border border-black py-1 text-center">TOTAL</td>
            <td className="border border-black px-1 text-right">{new Intl.NumberFormat('id-ID').format(debit)}</td>
            <td className="border border-black px-1 text-right">{new Intl.NumberFormat('id-ID').format(kredit)}</td>
            <td className="border border-black" />
            <td className="relative border border-black">
              <button type="button" title="Tambah baris" aria-label="Tambah baris" className="absolute top-1/2 left-full ml-1.5 -translate-y-1/2 rounded-sm p-0.5 text-emerald-700 hover:bg-emerald-50 focus-visible:outline-2 focus-visible:outline-ring" onClick={addRow}><Plus aria-hidden="true" className="size-4" /></button>
            </td>
          </tr>
        </tbody>
      </table>
      {debit !== kredit && <p role="status" className="mt-2 flex items-center justify-end gap-2 text-xs font-semibold text-red-700">
        Selisih {formatRupiah(Math.abs(debit - kredit))}
        <Button type="button" variant="outline" size="sm" className="border-neutral-400 bg-white text-black hover:bg-neutral-100" onClick={() => setLines(form.lines.map((line) => ({ ...line, manual: false })))}>Samakan otomatis</Button>
      </p>}

      <div className="mt-3 grid grid-cols-5 border border-black text-xs">
        {signers.map(([title, role], index) => <div key={role} className={cn('px-4 py-2 text-center', index > 0 && 'border-l border-black')}>
          <p>{title}</p>
          <p className="mt-9 border-t border-black pt-1">{role}</p>
        </div>)}
      </div>
    </div>
  </div>;
}

function AccountChoice({ account, selected, onSelect }: { account: Account; selected: boolean; onSelect: () => void }) {
  return <button type="button" aria-pressed={selected} className={cn('block w-full rounded-sm px-0.5 text-left text-xs leading-5 hover:bg-amber-50', selected && 'font-semibold')} onClick={onSelect}>
    {selected ? '[X]' : '[ ]'} {accountBoxLabel(account)}
  </button>;
}

function SheetRow({ label, last = false, children }: { label: string; last?: boolean; children: React.ReactNode }) {
  return <div className={cn('grid grid-cols-[10.5rem_minmax(0,1fr)] items-center', !last && 'border-b border-black')}>
    <span className="border-r border-black px-2.5 py-1 text-xs font-bold">{label}</span>
    <span className="px-1.5">{children}</span>
  </div>;
}
