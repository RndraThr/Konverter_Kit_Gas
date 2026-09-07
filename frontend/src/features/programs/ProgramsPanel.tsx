import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Edit3, Plus, Sprout, Waves } from 'lucide-react';
import { useState } from 'react';
import { DataTable } from '../../components/DataTable';
import { FormField } from '../../components/FormField';
import { apiRequest } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { SetupDialog } from './SetupDialog';
import { DataResponse, Program, ProgramType, programTypeLabel } from './types';

const empty = { code: '', name: '', program_type: 'farmer' as ProgramType, fiscal_year: new Date().getFullYear(), status: 'draft', notes: '' };

export function ProgramsPanel() {
  const canManage = useCan('programs.manage'); const client = useQueryClient();
  const query = useQuery({ queryKey: ['program-setup', 'programs'], queryFn: () => apiRequest<DataResponse<Program[]>>('/api/v1/program-setup/programs') });
  const [open, setOpen] = useState(false); const [editing, setEditing] = useState<Program>(); const [values, setValues] = useState(empty);
  const mutation = useMutation({ mutationFn: () => apiRequest(`/api/v1/program-setup/programs${editing ? `/${editing.id}` : ''}`, { method: editing ? 'PATCH' : 'POST', body: JSON.stringify(values) }), onSuccess: () => { setOpen(false); client.invalidateQueries({ queryKey: ['program-setup'] }); } });
  const show = (item?: Program) => { setEditing(item); setValues(item ? { code: item.code, name: item.name, program_type: item.program_type, fiscal_year: item.fiscal_year, status: item.status, notes: item.notes ?? '' } : empty); setOpen(true); };
  return <section className="setupPanel"><header className="panelHeading"><div><h2>Program bantuan</h2><p>Pisahkan program Petani dan Nelayan per tahun anggaran.</p></div>{canManage && <button className="primaryButton" onClick={() => show()}><Plus />Tambah program</button>}</header>
    {query.data?.data.length ? <DataTable label="Daftar program"><thead><tr><th>Program</th><th>Jenis</th><th>Tahun</th><th>Status</th>{canManage && <th>Aksi</th>}</tr></thead><tbody>{query.data.data.map((item) => <tr key={item.id}><td><strong>{item.name}</strong><small className="subline">{item.code}</small></td><td><span className={`sectorLabel sector-${item.program_type}`}>{item.program_type === 'farmer' ? <Sprout /> : <Waves />}{programTypeLabel(item.program_type)}</span></td><td>{item.fiscal_year}</td><td>{item.status}</td>{canManage && <td><button className="iconButton" aria-label={`Edit ${item.name}`} onClick={() => show(item)}><Edit3 /></button></td>}</tr>)}</tbody></DataTable> : <div className="emptyState">Belum ada program bantuan.</div>}
    <SetupDialog open={open} onOpenChange={setOpen} title={editing ? `Edit ${editing.name}` : 'Tambah program'} description="Jenis program tidak dapat dicampur dalam satu jadwal." pending={mutation.isPending} onSubmit={() => mutation.mutate()}>
      <FormField label="Kode program" name="code" required value={values.code} onChange={(e) => setValues({ ...values, code: e.target.value.toUpperCase() })} />
      <FormField label="Nama program" name="name" required value={values.name} onChange={(e) => setValues({ ...values, name: e.target.value.toUpperCase() })} />
      <label className="formField"><span>Jenis penerima</span><select className="selectField" value={values.program_type} onChange={(e) => setValues({ ...values, program_type: e.target.value as ProgramType })}><option value="farmer">Petani</option><option value="fisherman">Nelayan</option></select></label>
      <FormField label="Tahun anggaran" name="fiscal_year" type="number" required value={values.fiscal_year} onChange={(e) => setValues({ ...values, fiscal_year: Number(e.target.value) })} />
      <label className="formField"><span>Status</span><select className="selectField" value={values.status} onChange={(e) => setValues({ ...values, status: e.target.value })}><option value="draft">Draft</option><option value="active">Aktif</option><option value="completed">Selesai</option><option value="archived">Arsip</option></select></label>
      <FormField label="Catatan" name="notes" value={values.notes} onChange={(e) => setValues({ ...values, notes: e.target.value })} />
      {mutation.isError && <p className="formNotice">Program belum dapat disimpan.</p>}
    </SetupDialog>
  </section>;
}
