import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Pencil, Plus, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { toast } from 'sonner';
import { DataState } from '@/components/DataState';
import { FormField } from '@/components/FormField';
import { PageHeader } from '@/components/PageHeader';
import { StatusBadge } from '@/components/StatusBadge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { apiRequest } from '@/lib/api';
import { useCan } from '@/lib/permissions';
import { financeKey, type DataResponse, type Option, type Team, type TeamOptions } from './api';
import { errorMessage } from './shared';
import { VoucherSettingsCard } from './VoucherCards';

type TeamForm = { id: string; name: string; is_active: boolean; leader_ids: string[]; finance_ids: string[]; regency_ids: string[] };
const emptyForm: TeamForm = { id: '', name: '', is_active: true, leader_ids: [], finance_ids: [], regency_ids: [] };

function formFrom(team: Team): TeamForm {
  return { id: team.id, name: team.name, is_active: team.is_active, leader_ids: team.leaders.map((item) => item.id), finance_ids: team.finance.map((item) => item.id), regency_ids: team.regencies.map((item) => item.id) };
}

function Choices({ legend, options, value, empty, onChange }: { legend: string; options: Option[]; value: string[]; empty: string; onChange: (value: string[]) => void }) {
  return <fieldset className="grid gap-1 border-0 p-0">
    <legend className="mb-1 text-sm font-semibold">{legend}</legend>
    {options.length === 0 && <p className="text-sm text-muted-foreground">{empty}</p>}
    <div className="grid max-h-52 gap-1 overflow-y-auto sm:grid-cols-2">{options.map((option) => <label key={option.id} className="flex min-h-10 items-center gap-3 text-sm">
      <Checkbox checked={value.includes(option.id)} onCheckedChange={() => onChange(value.includes(option.id) ? value.filter((id) => id !== option.id) : [...value, option.id])} />
      <span className="min-w-0 break-words">{option.name}</span>
    </label>)}</div>
  </fieldset>;
}

/** Tim keuangan: leader yang mengajukan, finance yang menangani, dan kabupatennya. */
export function TeamsPage() {
  const queryClient = useQueryClient();
  const canManage = useCan('finance.manage');
  const [form, setForm] = useState<TeamForm | null>(null);
  const teams = useQuery({ queryKey: [...financeKey, 'teams'], queryFn: () => apiRequest<DataResponse<Team[]>>('/api/v1/finance/teams') });
  const options = useQuery({ queryKey: [...financeKey, 'team-options'], queryFn: () => apiRequest<DataResponse<TeamOptions>>('/api/v1/finance/team-options') });
  const refresh = () => queryClient.invalidateQueries({ queryKey: financeKey });
  const save = useMutation({
    mutationFn: ({ id, ...input }: TeamForm) => apiRequest(`/api/v1/finance/teams${id ? `/${id}` : ''}`, { method: id ? 'PUT' : 'POST', body: JSON.stringify(input) }),
    onSuccess: () => { void refresh(); setForm(null); toast.success('Tim tersimpan.'); },
    onError: (error) => toast.error(errorMessage(error)),
  });
  const remove = useMutation({
    mutationFn: (id: string) => apiRequest(`/api/v1/finance/teams/${id}`, { method: 'DELETE' }),
    onSuccess: () => { void refresh(); toast.success('Tim dihapus.'); },
    onError: (error) => toast.error(errorMessage(error)),
  });
  const rows = teams.data?.data ?? [];

  return <div className="space-y-6">
    <PageHeader
      title="Tim keuangan"
      description="Atur leader yang mengajukan dan finance yang menangani setiap tim."
      actions={canManage ? <Button onClick={() => setForm(emptyForm)}><Plus />Tambah tim</Button> : undefined}
    />
    {teams.isPending && <DataState kind="loading" title="Memuat tim" description="Mengambil daftar tim." />}
    {teams.isError && <DataState kind="error" title="Tim belum dapat dimuat" description="Periksa koneksi lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => teams.refetch() }} />}
    {teams.isSuccess && rows.length === 0 && <DataState kind="empty" title="Belum ada tim" description="Tambahkan tim agar leader bisa mengajukan." />}
    <div className="grid gap-3 lg:grid-cols-2">{rows.map((team) => <Card key={team.id}><CardContent className="grid gap-2 pt-5">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <strong className="min-w-0 break-words">{team.name}</strong>
        <StatusBadge active={team.is_active} />
      </div>
      <p className="text-sm text-muted-foreground">Leader: {team.leaders.map((item) => item.name).join(', ') || '-'}</p>
      <p className="text-sm text-muted-foreground">Finance: {team.finance.map((item) => item.name).join(', ') || 'Belum ada, pengajuan belum bisa dikirim'}</p>
      <p className="text-sm text-muted-foreground">Kabupaten: {team.regencies.map((item) => item.name).join(', ') || '-'}</p>
      {canManage && <div className="flex justify-end gap-1">
        <Button variant="ghost" size="sm" onClick={() => setForm(formFrom(team))}><Pencil />Ubah</Button>
        <Button variant="ghost" size="sm" className="text-destructive" disabled={remove.isPending} onClick={() => remove.mutate(team.id)}><Trash2 />Hapus</Button>
      </div>}
    </CardContent></Card>)}</div>
    {canManage && <VoucherSettingsCard />}
    <Dialog open={form !== null} onOpenChange={(open) => { if (!open) setForm(null); }}>
      <DialogContent aria-label="Tim keuangan" className="max-h-[calc(100dvh-2rem)] max-w-2xl overflow-y-auto">
        <DialogHeader><DialogTitle>{form?.id ? 'Ubah tim' : 'Tambah tim'}</DialogTitle><DialogDescription>Satu finance bisa menangani banyak tim.</DialogDescription></DialogHeader>
        {form && <div className="grid gap-4">
          <FormField label="Nama tim" name="team_name" required maxLength={120} value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} />
          <Choices legend="Leader" options={options.data?.data.leaders ?? []} value={form.leader_ids} empty="Belum ada pengguna dengan izin Ajukan Keuangan." onChange={(leader_ids) => setForm({ ...form, leader_ids })} />
          <Choices legend="Finance" options={options.data?.data.finance ?? []} value={form.finance_ids} empty="Belum ada pengguna dengan role Finance." onChange={(finance_ids) => setForm({ ...form, finance_ids })} />
          <Choices legend="Kabupaten" options={options.data?.data.regencies ?? []} value={form.regency_ids} empty="Belum ada kabupaten." onChange={(regency_ids) => setForm({ ...form, regency_ids })} />
          <label className="flex min-h-10 items-center gap-3 text-sm"><Checkbox checked={form.is_active} onCheckedChange={(checked) => setForm({ ...form, is_active: checked === true })} />Tim aktif</label>
        </div>}
        <DialogFooter>
          <Button variant="outline" onClick={() => setForm(null)}>Batal</Button>
          <Button disabled={save.isPending || !form?.name.trim()} onClick={() => form && save.mutate(form)}>{save.isPending ? 'Menyimpan...' : 'Simpan tim'}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>;
}
