import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Edit3, Plus, Sprout, Waves } from 'lucide-react';
import { useState } from 'react';
import { toast } from 'sonner';
import { DataState } from '../../components/DataState';
import { DataTable } from '../../components/DataTable';
import { FormField } from '../../components/FormField';
import { StatusBadge } from '../../components/StatusBadge';
import { Button } from '../../components/ui/button';
import { Alert, AlertDescription } from '../../components/ui/alert';
import { Badge } from '../../components/ui/badge';
import { Label } from '../../components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../components/ui/select';
import { apiRequest } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { SetupDialog } from './SetupDialog';
import { DataResponse, Program, ProgramType, programTypeLabel } from './types';

const empty = { code: '', name: '', program_type: 'farmer' as ProgramType, fiscal_year: new Date().getFullYear(), status: 'draft', notes: '' };

export function ProgramsPanel() {
  const canManage = useCan('programs.manage'); const client = useQueryClient();
  const query = useQuery({ queryKey: ['program-setup', 'programs'], queryFn: () => apiRequest<DataResponse<Program[]>>('/api/v1/program-setup/programs') });
  const [open, setOpen] = useState(false); const [editing, setEditing] = useState<Program>(); const [values, setValues] = useState(empty);
  const mutation = useMutation({ mutationFn: () => apiRequest(`/api/v1/program-setup/programs${editing ? `/${editing.id}` : ''}`, { method: editing ? 'PATCH' : 'POST', body: JSON.stringify(values) }), onSuccess: () => { setOpen(false); client.invalidateQueries({ queryKey: ['program-setup'] }); toast.success('Program berhasil disimpan.'); } });
  const show = (item?: Program) => { setEditing(item); setValues(item ? { code: item.code, name: item.name, program_type: item.program_type, fiscal_year: item.fiscal_year, status: item.status, notes: item.notes ?? '' } : empty); setOpen(true); };
  return <section className="setupPanel" aria-labelledby="programs-heading"><header className="panelHeading"><div><h2 id="programs-heading">Program bantuan</h2><p>Pisahkan program Petani dan Nelayan per tahun anggaran.</p></div>{canManage && <Button type="button" onClick={() => show()}><Plus />Tambah program</Button>}</header>
    {query.isError ? <DataState kind="error" title="Data program belum dapat dimuat" description="Muat ulang halaman untuk mencoba kembali." /> : query.data?.data.length ? <DataTable label="Daftar program"><thead><tr><th>Program</th><th>Jenis</th><th>Tahun</th><th>Status</th>{canManage && <th>Aksi</th>}</tr></thead><tbody>{query.data.data.map((item) => <tr key={item.id}><td><strong>{item.name}</strong><small className="subline">{item.code}</small></td><td><Badge variant="outline" className={`sectorLabel sector-${item.program_type}`}>{item.program_type === 'farmer' ? <Sprout /> : <Waves />}{programTypeLabel(item.program_type)}</Badge></td><td>{item.fiscal_year}</td><td><StatusBadge active={item.status === 'active'} activeText="Aktif" inactiveText={item.status === 'draft' ? 'Draft' : item.status === 'completed' ? 'Selesai' : 'Arsip'} /></td>{canManage && <td><Button type="button" variant="ghost" size="icon" aria-label={`Edit ${item.name}`} onClick={() => show(item)}><Edit3 /></Button></td>}</tr>)}</tbody></DataTable> : <DataState kind="empty" title="Belum ada program bantuan" description="Tambahkan program untuk mengatur bantuan per tahun anggaran." />}
    <SetupDialog open={open} onOpenChange={setOpen} title={editing ? `Edit ${editing.name}` : 'Tambah program'} description="Jenis program tidak dapat dicampur dalam satu jadwal." pending={mutation.isPending} onSubmit={() => mutation.mutate()}>
      <FormField label="Kode program" name="code" required value={values.code} onChange={(e) => setValues({ ...values, code: e.target.value.toUpperCase() })} />
      <FormField label="Nama program" name="name" required value={values.name} onChange={(e) => setValues({ ...values, name: e.target.value.toUpperCase() })} />
      <div className="grid gap-2"><Label id="program-type-label">Jenis penerima</Label><Select value={values.program_type} onValueChange={(value) => setValues({ ...values, program_type: value as ProgramType })}><SelectTrigger className="w-full" aria-labelledby="program-type-label"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="farmer">Petani</SelectItem><SelectItem value="fisherman">Nelayan</SelectItem></SelectContent></Select></div>
      <FormField label="Tahun anggaran" name="fiscal_year" type="number" required value={values.fiscal_year} onChange={(e) => setValues({ ...values, fiscal_year: Number(e.target.value) })} />
      <div className="grid gap-2"><Label id="program-status-label">Status</Label><Select value={values.status} onValueChange={(value) => setValues({ ...values, status: value })}><SelectTrigger className="w-full" aria-labelledby="program-status-label"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="draft">Draft</SelectItem><SelectItem value="active">Aktif</SelectItem><SelectItem value="completed">Selesai</SelectItem><SelectItem value="archived">Arsip</SelectItem></SelectContent></Select></div>
      <FormField className="sm:col-span-2" label="Catatan" name="notes" value={values.notes} onChange={(e) => setValues({ ...values, notes: e.target.value })} />
      {mutation.isError && <Alert className="sm:col-span-2" variant="destructive"><AlertDescription>Program belum dapat disimpan.</AlertDescription></Alert>}
    </SetupDialog>
  </section>;
}
