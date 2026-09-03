import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { CalendarDays, Edit3, Plus } from 'lucide-react';
import { useState } from 'react';
import { DataTable } from '../../components/DataTable';
import { FormField } from '../../components/FormField';
import { apiRequest } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { SetupDialog } from './SetupDialog';
import { DataResponse, dateInputValue, datePayload, DocumentationTemplate, formatDate, PackageTemplate, Program, Regency, Schedule } from './types';

const today = new Date().toISOString().slice(0, 10);
const empty = { program_id: '', regency_id: '', package_template_version_id: '', documentation_template_version_id: '', name: '', start_date: today, end_date: today, status: 'draft', distribution_number_padding: 4, notes: '' };

export function SchedulesPanel() {
  const canManage = useCan('programs.manage'); const client = useQueryClient();
  const schedules = useQuery({ queryKey: ['program-setup', 'schedules'], queryFn: () => apiRequest<DataResponse<Schedule[]>>('/api/v1/program-setup/schedules') });
  const programs = useQuery({ queryKey: ['program-setup', 'programs'], queryFn: () => apiRequest<DataResponse<Program[]>>('/api/v1/program-setup/programs') });
  const regencies = useQuery({ queryKey: ['program-setup', 'regencies'], queryFn: () => apiRequest<DataResponse<Regency[]>>('/api/v1/program-setup/regencies') });
  const packages = useQuery({ queryKey: ['program-setup', 'package-templates'], queryFn: () => apiRequest<DataResponse<PackageTemplate[]>>('/api/v1/program-setup/package-templates') });
  const documents = useQuery({ queryKey: ['program-setup', 'documentation-templates'], queryFn: () => apiRequest<DataResponse<DocumentationTemplate[]>>('/api/v1/program-setup/documentation-templates') });
  const [open, setOpen] = useState(false); const [editing, setEditing] = useState<Schedule>(); const [values, setValues] = useState(empty);
  const mutation = useMutation({ mutationFn: () => apiRequest(`/api/v1/program-setup/schedules${editing ? `/${editing.id}` : ''}`, { method: editing ? 'PATCH' : 'POST', body: JSON.stringify({ ...values, start_date: datePayload(values.start_date), end_date: datePayload(values.end_date) }) }), onSuccess: () => { setOpen(false); client.invalidateQueries({ queryKey: ['program-setup', 'schedules'] }); } });
  const show = (item?: Schedule) => { setEditing(item); setValues(item ? { program_id: item.program_id, regency_id: item.regency_id, package_template_version_id: item.package_template_version_id, documentation_template_version_id: item.documentation_template_version_id, name: item.name, start_date: dateInputValue(item.start_date), end_date: dateInputValue(item.end_date), status: item.status, distribution_number_padding: item.distribution_number_padding, notes: item.notes ?? '' } : empty); setOpen(true); };
  const selectedProgram = programs.data?.data.find((item) => item.id === values.program_id);
  const compatiblePackages = packages.data?.data.filter((item) => item.status === 'published' && (!selectedProgram || item.program_type === selectedProgram.program_type)) ?? [];
  const compatibleDocuments = documents.data?.data.filter((item) => item.status === 'published' && (!selectedProgram || item.program_type === selectedProgram.program_type)) ?? [];
  const packageName = (item: Schedule) => item.package_template?.name ?? packages.data?.data.find((template) => template.id === item.package_template_version_id)?.name ?? 'Template tidak ditemukan';
  const documentName = (item: Schedule) => item.documentation_template?.name ?? documents.data?.data.find((template) => template.id === item.documentation_template_version_id)?.name ?? 'Template tidak ditemukan';
  return <section className="setupPanel"><header className="panelHeading"><div><h2>Jadwal kabupaten</h2><p>Satu jadwal mengunci program, lokasi, dan versi template yang digunakan.</p></div>{canManage && <button className="primaryButton" onClick={() => show()}><Plus />Tambah jadwal</button>}</header>
    {schedules.data?.data.length ? <DataTable label="Daftar jadwal"><thead><tr><th>Jadwal</th><th>Program</th><th>Kabupaten</th><th>Periode</th><th>Template paket</th><th>Foto</th><th>Status</th>{canManage && <th>Aksi</th>}</tr></thead><tbody>{schedules.data.data.map((item) => <tr key={item.id}><td><span className="entityName"><CalendarDays />{item.name}</span></td><td>{item.program?.name ?? item.program_id}</td><td><strong>{item.regency?.document_code}</strong> {item.regency?.name ?? item.regency_id}</td><td>{formatDate(item.start_date)}<small className="subline">s.d. {formatDate(item.end_date)}</small></td><td>{packageName(item)}</td><td>{documentName(item)}</td><td>{item.status}</td>{canManage && <td><button className="iconButton" aria-label={`Edit ${item.name}`} onClick={() => show(item)}><Edit3 /></button></td>}</tr>)}</tbody></DataTable> : <div className="emptyState">Belum ada jadwal kabupaten.</div>}
    <SetupDialog open={open} onOpenChange={setOpen} title={editing ? `Edit ${editing.name}` : 'Tambah jadwal'} description="Pilih template published agar data lapangan konsisten." pending={mutation.isPending} onSubmit={() => mutation.mutate()}>
      <FormField label="Nama jadwal" name="name" required value={values.name} onChange={(e) => setValues({ ...values, name: e.target.value.toUpperCase() })} />
      <label className="formField"><span>Program</span><select className="selectField" required value={values.program_id} onChange={(e) => setValues({ ...values, program_id: e.target.value, package_template_version_id: '', documentation_template_version_id: '' })}><option value="">Pilih program</option>{programs.data?.data.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
      <label className="formField"><span>Kabupaten</span><select className="selectField" required value={values.regency_id} onChange={(e) => setValues({ ...values, regency_id: e.target.value })}><option value="">Pilih kabupaten</option>{regencies.data?.data.filter((item) => item.is_active).map((item) => <option key={item.id} value={item.id}>{item.document_code} - {item.name}</option>)}</select></label>
      <label className="formField"><span>Template paket</span><select className="selectField" required value={values.package_template_version_id} onChange={(e) => setValues({ ...values, package_template_version_id: e.target.value })}><option value="">Pilih template</option>{compatiblePackages.map((item) => <option key={item.id} value={item.id}>{item.name} v{item.version}</option>)}</select></label>
      <label className="formField"><span>Template dokumentasi</span><select className="selectField" required value={values.documentation_template_version_id} onChange={(e) => setValues({ ...values, documentation_template_version_id: e.target.value })}><option value="">Pilih template</option>{compatibleDocuments.map((item) => <option key={item.id} value={item.id}>{item.name} v{item.version}</option>)}</select></label>
      <label className="formField"><span>Status</span><select className="selectField" value={values.status} onChange={(e) => setValues({ ...values, status: e.target.value })}><option value="draft">Draft</option><option value="active">Aktif</option><option value="completed">Selesai</option><option value="cancelled">Dibatalkan</option></select></label>
      <FormField label="Mulai" name="start_date" type="date" required value={values.start_date} onChange={(e) => setValues({ ...values, start_date: e.target.value })} />
      <FormField label="Selesai" name="end_date" type="date" required value={values.end_date} onChange={(e) => setValues({ ...values, end_date: e.target.value })} />
      <FormField label="Digit nomor pembagian" name="distribution_number_padding" type="number" min={1} max={8} value={values.distribution_number_padding} onChange={(e) => setValues({ ...values, distribution_number_padding: Number(e.target.value) })} />
      <FormField label="Catatan" name="notes" value={values.notes} onChange={(e) => setValues({ ...values, notes: e.target.value })} />
      {mutation.isError && <p className="formNotice">Jadwal belum dapat disimpan. Periksa kecocokan jenis template.</p>}
    </SetupDialog>
  </section>;
}
