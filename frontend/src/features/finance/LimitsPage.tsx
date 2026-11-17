import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Save } from 'lucide-react';
import { useState } from 'react';
import { toast } from 'sonner';
import { DataState } from '@/components/DataState';
import { PageHeader } from '@/components/PageHeader';
import { Button } from '@/components/ui/button';
import { fetchLimits, financeKey, formatRupiah, saveLimit, type RegencyLimit } from './api';
import { MoneyInput } from './MoneyInput';
import { errorMessage } from './shared';

const limitsKey = [...financeKey, 'limits'];

/** Finance mengatur limit tiap kabupaten; leader hanya melihat sisanya. */
export function LimitsPage() {
  const limits = useQuery({ queryKey: limitsKey, queryFn: fetchLimits });
  const rows = limits.data?.data ?? [];
  return <div className="space-y-6">
    <PageHeader title="Limit kabupaten" description="Atur limit kasbon dan petty cash. Admin leader hanya melihat sisa saldonya." />
    {limits.isPending && <DataState kind="loading" title="Memuat limit" description="Mengambil kabupaten tim Anda." />}
    {limits.isError && <DataState kind="error" title="Limit belum dapat dimuat" description="Periksa koneksi lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => limits.refetch() }} />}
    {limits.isSuccess && rows.length === 0 && <DataState kind="empty" title="Belum ada kabupaten" description="Kabupaten muncul setelah ditambahkan ke tim di Tim keuangan." />}
    {rows.length > 0 && <ul className="grid gap-3 lg:grid-cols-2" aria-label="Limit kabupaten">{rows.map((row) => <LimitCard key={`${row.regency_id}-${row.kasbon_limit}-${row.petty_cash_limit}`} row={row} />)}</ul>}
  </div>;
}

function LimitCard({ row }: { row: RegencyLimit }) {
  const queryClient = useQueryClient();
  const [kasbon, setKasbon] = useState(row.kasbon_limit);
  const [pettyCash, setPettyCash] = useState(row.petty_cash_limit);
  const save = useMutation({
    mutationFn: () => saveLimit(row.regency_id, kasbon, pettyCash),
    onSuccess: () => { void queryClient.invalidateQueries({ queryKey: financeKey }); toast.success(`Limit ${row.regency_name} tersimpan.`); },
    onError: (error) => toast.error(errorMessage(error)),
  });
  const changed = kasbon !== row.kasbon_limit || pettyCash !== row.petty_cash_limit;
  return <li className="grid gap-3 rounded-xl border bg-card p-4">
    <div className="flex items-center justify-between gap-2">
      <strong className="text-sm">{row.regency_name}</strong>
      <Button size="sm" disabled={!changed || save.isPending} onClick={() => save.mutate()}><Save />{save.isPending ? 'Menyimpan...' : 'Simpan'}</Button>
    </div>
    <div className="grid gap-3 sm:grid-cols-2">
      <div className="grid gap-1">
        <label className="grid gap-1 text-xs font-medium">Limit kasbon (Rp)<MoneyInput value={kasbon} onChange={setKasbon} /></label>
        <p className="text-xs text-muted-foreground">{row.kasbon_limit > 0 ? `Terpakai ${formatRupiah(row.kasbon_used)} · sisa ${formatRupiah(row.kasbon_available)}` : `Tanpa batas · terpakai ${formatRupiah(row.kasbon_used)}`}</p>
      </div>
      <div className="grid gap-1">
        <label className="grid gap-1 text-xs font-medium">Limit petty cash (Rp)<MoneyInput value={pettyCash} onChange={setPettyCash} /></label>
        <p className="text-xs text-muted-foreground">{row.petty_cash_limit > 0 ? `Terpakai ${formatRupiah(row.petty_cash_used)} · sisa ${formatRupiah(row.petty_cash_available)}` : 'Tidak tersedia'}</p>
      </div>
    </div>
  </li>;
}
