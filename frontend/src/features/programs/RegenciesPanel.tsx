import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Edit3, MapPin, Plus } from 'lucide-react';
import { useState } from 'react';
import { DataTable } from '../../components/DataTable';
import { FormField } from '../../components/FormField';
import { apiRequest } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { SetupDialog } from './SetupDialog';
import { DataResponse, Regency } from './types';

const empty = { province_name: '', name: '', document_code: '', is_active: true, notes: '' };

export function RegenciesPanel() {
  const canManage = useCan('programs.manage');
  const client = useQueryClient();
  const query = useQuery({ queryKey: ['program-setup', 'regencies'], queryFn: () => apiRequest<DataResponse<Regency[]>>('/api/v1/program-setup/regencies') });
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<Regency>();
  const [values, setValues] = useState(empty);
  const mutation = useMutation({ mutationFn: () => apiRequest(`/api/v1/program-setup/regencies${editing ? `/${editing.id}` : ''}`, { method: editing ? 'PATCH' : 'POST', body: JSON.stringify(values) }), onSuccess: () => { setOpen(false); client.invalidateQueries({ queryKey: ['program-setup'] }); } });
  const show = (item?: Regency) => { setEditing(item); setValues(item ? { province_name: item.province_name, name: item.name, document_code: item.document_code, is_active: item.is_active, notes: item.notes ?? '' } : empty); setOpen(true); };
  return <section className="setupPanel"><header className="panelHeading"><div><h2>Kabupaten operasional</h2><p>Kode tiga huruf dipakai sebagai bagian penomoran dokumen.</p></div>{canManage && <button className="primaryButton" onClick={() => show()}><Plus />Tambah kabupaten</button>}</header>
    {query.isError ? <div className="errorState">Data kabupaten belum dapat dimuat.</div> : query.data?.data.length ? <DataTable label="Daftar kabupaten"><thead><tr><th>Kabupaten</th><th>Provinsi</th><th>Kode dokumen</th><th>Status</th>{canManage && <th>Aksi</th>}</tr></thead><tbody>{query.data.data.map((item) => <tr key={item.id}><td><span className="entityName"><MapPin />{item.name}</span></td><td>{item.province_name}</td><td><strong className="documentCode">{item.document_code}</strong></td><td><span className={`statusBadge ${item.is_active ? 'statusActive' : 'statusInactive'}`}><span />{item.is_active ? 'Aktif' : 'Nonaktif'}</span></td>{canManage && <td><button className="iconButton" aria-label={`Edit ${item.name}`} title={`Edit ${item.name}`} onClick={() => show(item)}><Edit3 /></button></td>}</tr>)}</tbody></DataTable> : <div className="emptyState">Belum ada kabupaten. Tambahkan lokasi operasional pertama.</div>}
    <SetupDialog open={open} onOpenChange={setOpen} title={editing ? `Edit ${editing.name}` : 'Tambah kabupaten'} description="Tetapkan nama wilayah dan kode dokumen yang unik." pending={mutation.isPending} onSubmit={() => mutation.mutate()}>
      <FormField label="Provinsi" name="province_name" required value={values.province_name} onChange={(e) => setValues({ ...values, province_name: e.target.value })} />
      <FormField label="Kabupaten" name="name" required value={values.name} onChange={(e) => setValues({ ...values, name: e.target.value })} />
      <FormField label="Kode dokumen" name="document_code" required maxLength={3} value={values.document_code} onChange={(e) => setValues({ ...values, document_code: e.target.value.toUpperCase() })} hint="Tepat tiga huruf, misalnya WJO." />
      <FormField label="Catatan" name="notes" value={values.notes} onChange={(e) => setValues({ ...values, notes: e.target.value })} />
      <label className="checkboxField fullField"><input type="checkbox" checked={values.is_active} onChange={(e) => setValues({ ...values, is_active: e.target.checked })} />Kabupaten aktif</label>
      {mutation.isError && <p className="formNotice">Kabupaten belum dapat disimpan.</p>}
    </SetupDialog>
  </section>;
}
