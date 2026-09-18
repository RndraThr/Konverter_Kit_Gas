import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Edit3, MapPin, Plus } from 'lucide-react';
import { useState } from 'react';
import { toast } from 'sonner';
import { DataState } from '../../components/DataState';
import { DataTable } from '../../components/DataTable';
import { FormField } from '../../components/FormField';
import { StatusBadge } from '../../components/StatusBadge';
import { Button } from '../../components/ui/button';
import { Checkbox } from '../../components/ui/checkbox';
import { Alert, AlertDescription } from '../../components/ui/alert';
import { apiRequest } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { SetupDialog } from './SetupDialog';
import { SetupToolbar } from './SetupToolbar';
import { DataResponse, Regency } from './types';

const empty = { province_name: '', name: '', document_code: '', is_active: true, notes: '' };

export function RegenciesPanel() {
  const canManage = useCan('programs.manage');
  const client = useQueryClient();
  const query = useQuery({ queryKey: ['program-setup', 'regencies'], queryFn: () => apiRequest<DataResponse<Regency[]>>('/api/v1/program-setup/regencies') });
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<Regency>();
  const [values, setValues] = useState(empty); const [initialValues, setInitialValues] = useState(empty);
  const [search, setSearch] = useState('');
  const [status, setStatus] = useState('all');
  const mutation = useMutation({ mutationFn: () => apiRequest(`/api/v1/program-setup/regencies${editing ? `/${editing.id}` : ''}`, { method: editing ? 'PATCH' : 'POST', body: JSON.stringify(values) }), onSuccess: () => { setOpen(false); client.invalidateQueries({ queryKey: ['program-setup'] }); toast.success('Kabupaten berhasil disimpan.'); } });
  const show = (item?: Regency) => { const nextValues = item ? { province_name: item.province_name, name: item.name, document_code: item.document_code, is_active: item.is_active, notes: item.notes ?? '' } : empty; setEditing(item); setValues(nextValues); setInitialValues(nextValues); setOpen(true); };
  const allItems = query.data?.data ?? [];
  const normalizedSearch = search.trim().toLocaleLowerCase('id-ID');
  const items = allItems.filter((item) => (!normalizedSearch || `${item.name} ${item.province_name} ${item.document_code}`.toLocaleLowerCase('id-ID').includes(normalizedSearch)) && (status === 'all' || (status === 'active' ? item.is_active : !item.is_active)));

  return <section className="setupPanel" aria-labelledby="regencies-heading"><header className="panelHeading"><div><h2 id="regencies-heading">Kabupaten operasional</h2><p>Kode tiga huruf dipakai sebagai bagian penomoran dokumen.</p></div>{canManage && <Button type="button" onClick={() => show()}><Plus />Tambah kabupaten</Button>}</header>
    {query.isPending ? <DataState kind="loading" title="Memuat data kabupaten" description="Mengambil daftar wilayah operasional terbaru." /> : query.isError ? <DataState kind="error" title="Data kabupaten belum dapat dimuat" description="Periksa koneksi, lalu coba kembali." action={{ label: 'Coba lagi', onClick: () => query.refetch() }} /> : <><SetupToolbar entity="kabupaten" search={search} onSearchChange={setSearch} status={status} onStatusChange={setStatus} options={[{ value: 'all', label: 'Semua status' }, { value: 'active', label: 'Aktif' }, { value: 'inactive', label: 'Nonaktif' }]} shown={items.length} total={allItems.length} />{items.length ? <DataTable label="Daftar kabupaten"><thead><tr><th>Kabupaten</th><th>Provinsi</th><th>Kode dokumen</th><th>Status</th>{canManage && <th className="actionColumn">Aksi</th>}</tr></thead><tbody>{items.map((item) => <tr key={item.id}><td><span className="entityName"><MapPin />{item.name}</span></td><td>{item.province_name}</td><td><strong className="documentCode">{item.document_code}</strong></td><td><StatusBadge active={item.is_active} /></td>{canManage && <td className="actionColumn"><Button type="button" variant="ghost" size="icon" aria-label={`Edit ${item.name}`} title={`Edit ${item.name}`} onClick={() => show(item)}><Edit3 /></Button></td>}</tr>)}</tbody></DataTable> : <DataState kind="empty" title={allItems.length ? 'Tidak ada kabupaten yang sesuai' : 'Belum ada kabupaten'} description={allItems.length ? 'Ubah kata kunci atau filter untuk menampilkan data lain.' : 'Tambahkan lokasi operasional pertama untuk memulai.'} />}</>}
    <SetupDialog open={open} onOpenChange={setOpen} title={editing ? `Edit ${editing.name}` : 'Tambah kabupaten'} description="Tetapkan nama wilayah dan kode dokumen yang unik." pending={mutation.isPending} dirty={JSON.stringify(values) !== JSON.stringify(initialValues)} onSubmit={() => mutation.mutate()}>
      <FormField label="Provinsi" name="province_name" required value={values.province_name} onChange={(e) => setValues({ ...values, province_name: e.target.value.toUpperCase() })} />
      <FormField label="Kabupaten" name="name" required value={values.name} onChange={(e) => setValues({ ...values, name: e.target.value.toUpperCase() })} />
      <FormField label="Kode dokumen" name="document_code" required maxLength={3} value={values.document_code} onChange={(e) => setValues({ ...values, document_code: e.target.value.toUpperCase() })} hint="Tepat tiga huruf, misalnya WJO." />
      <FormField label="Catatan" name="notes" value={values.notes} onChange={(e) => setValues({ ...values, notes: e.target.value })} />
      <label className="flex min-h-11 items-center gap-3 sm:col-span-2"><Checkbox checked={values.is_active} onCheckedChange={(checked) => setValues({ ...values, is_active: checked === true })} />Kabupaten aktif</label>
      {mutation.isError && <Alert className="sm:col-span-2" variant="destructive"><AlertDescription>Kabupaten belum dapat disimpan.</AlertDescription></Alert>}
    </SetupDialog>
  </section>;
}
