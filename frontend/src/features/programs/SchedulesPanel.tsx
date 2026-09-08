import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { CalendarDays, Edit3, Plus } from 'lucide-react';
import { useState } from 'react';
import { toast } from 'sonner';
import { DataState } from '../../components/DataState';
import { DataTable } from '../../components/DataTable';
import { FormField } from '../../components/FormField';
import { StatusBadge } from '../../components/StatusBadge';
import { Button } from '../../components/ui/button';
import { Alert, AlertDescription } from '../../components/ui/alert';
import { Label } from '../../components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../components/ui/select';
import { apiRequest } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { SetupDialog } from './SetupDialog';
import { DataResponse, dateInputValue, datePayload, DocumentationTemplate, formatDate, PackageTemplate, Program, Regency, Schedule } from './types';

const today = new Date().toISOString().slice(0, 10);
const empty = { program_id: '', regency_id: '', package_template_version_id: '', documentation_template_version_id: '', name: '', start_date: today, end_date: today, status: 'draft', distribution_number_padding: 4, supervisor_name: '', notes: '' };

export function SchedulesPanel() {
  const canManage = useCan('programs.manage'); const client = useQueryClient();
  const schedules = useQuery({ queryKey: ['program-setup', 'schedules'], queryFn: () => apiRequest<DataResponse<Schedule[]>>('/api/v1/program-setup/schedules') });
  const programs = useQuery({ queryKey: ['program-setup', 'programs'], queryFn: () => apiRequest<DataResponse<Program[]>>('/api/v1/program-setup/programs') });
  const regencies = useQuery({ queryKey: ['program-setup', 'regencies'], queryFn: () => apiRequest<DataResponse<Regency[]>>('/api/v1/program-setup/regencies') });
  const packages = useQuery({ queryKey: ['program-setup', 'package-templates'], queryFn: () => apiRequest<DataResponse<PackageTemplate[]>>('/api/v1/program-setup/package-templates') });
  const documents = useQuery({ queryKey: ['program-setup', 'documentation-templates'], queryFn: () => apiRequest<DataResponse<DocumentationTemplate[]>>('/api/v1/program-setup/documentation-templates') });
  const [open, setOpen] = useState(false); const [editing, setEditing] = useState<Schedule>(); const [values, setValues] = useState(empty);
  const mutation = useMutation({ mutationFn: () => apiRequest(`/api/v1/program-setup/schedules${editing ? `/${editing.id}` : ''}`, { method: editing ? 'PATCH' : 'POST', body: JSON.stringify({ ...values, start_date: datePayload(values.start_date), end_date: datePayload(values.end_date) }) }), onSuccess: () => { setOpen(false); client.invalidateQueries({ queryKey: ['program-setup', 'schedules'] }); toast.success('Jadwal berhasil disimpan.'); } });
  const show = (item?: Schedule) => { setEditing(item); setValues(item ? { program_id: item.program_id, regency_id: item.regency_id, package_template_version_id: item.package_template_version_id, documentation_template_version_id: item.documentation_template_version_id, name: item.name, start_date: dateInputValue(item.start_date), end_date: dateInputValue(item.end_date), status: item.status, distribution_number_padding: item.distribution_number_padding, supervisor_name: item.supervisor_name ?? '', notes: item.notes ?? '' } : empty); setOpen(true); };
  const selectedProgram = programs.data?.data.find((item) => item.id === values.program_id);
  const compatiblePackages = packages.data?.data.filter((item) => item.status === 'published' && (!selectedProgram || item.program_type === selectedProgram.program_type)) ?? [];
  const compatibleDocuments = documents.data?.data.filter((item) => item.status === 'published' && (!selectedProgram || item.program_type === selectedProgram.program_type)) ?? [];
  const packageName = (item: Schedule) => item.package_template?.name ?? packages.data?.data.find((template) => template.id === item.package_template_version_id)?.name ?? 'Template tidak ditemukan';
  const documentName = (item: Schedule) => item.documentation_template?.name ?? documents.data?.data.find((template) => template.id === item.documentation_template_version_id)?.name ?? 'Template tidak ditemukan';
  return <section className="setupPanel" aria-labelledby="schedules-heading"><header className="panelHeading"><div><h2 id="schedules-heading">Jadwal kabupaten</h2><p>Satu jadwal mengunci program, lokasi, dan versi template yang digunakan.</p></div>{canManage && <Button type="button" onClick={() => show()}><Plus />Tambah jadwal</Button>}</header>
    {schedules.isError ? <DataState kind="error" title="Data jadwal belum dapat dimuat" description="Muat ulang halaman untuk mencoba kembali." /> : schedules.data?.data.length ? <DataTable label="Daftar jadwal" minimumWidth={1024}><thead><tr><th>Jadwal</th><th>Program</th><th>Kabupaten</th><th>Periode</th><th>Template paket</th><th>Foto</th><th>Status</th>{canManage && <th>Aksi</th>}</tr></thead><tbody>{schedules.data.data.map((item) => <tr key={item.id}><td><span className="entityName"><CalendarDays />{item.name}</span></td><td>{item.program?.name ?? item.program_id}</td><td><strong>{item.regency?.document_code}</strong> {item.regency?.name ?? item.regency_id}</td><td>{formatDate(item.start_date)}<small className="subline">s.d. {formatDate(item.end_date)}</small></td><td>{packageName(item)}</td><td>{documentName(item)}</td><td><StatusBadge active={item.status === 'active'} activeText="Aktif" inactiveText={item.status === 'draft' ? 'Draft' : item.status === 'completed' ? 'Selesai' : 'Dibatalkan'} /></td>{canManage && <td><Button type="button" variant="ghost" size="icon" aria-label={`Edit ${item.name}`} onClick={() => show(item)}><Edit3 /></Button></td>}</tr>)}</tbody></DataTable> : <DataState kind="empty" title="Belum ada jadwal kabupaten" description="Tambahkan jadwal setelah program dan template tersedia." />}
    <SetupDialog open={open} onOpenChange={setOpen} title={editing ? `Edit ${editing.name}` : 'Tambah jadwal'} description="Pilih template published agar data lapangan konsisten." pending={mutation.isPending} onSubmit={() => mutation.mutate()}>
      <FormField className="sm:col-span-2" label="Nama jadwal" name="name" required value={values.name} onChange={(e) => setValues({ ...values, name: e.target.value.toUpperCase() })} />
      <div className="grid gap-2"><Label id="schedule-program-label">Program</Label><Select required value={values.program_id} onValueChange={(value) => setValues({ ...values, program_id: value, package_template_version_id: '', documentation_template_version_id: '' })}><SelectTrigger className="w-full" aria-labelledby="schedule-program-label"><SelectValue placeholder="Pilih program" /></SelectTrigger><SelectContent>{programs.data?.data.map((item) => <SelectItem key={item.id} value={item.id}>{item.name}</SelectItem>)}</SelectContent></Select></div>
      <div className="grid gap-2"><Label id="schedule-regency-label">Kabupaten</Label><Select required value={values.regency_id} onValueChange={(value) => setValues({ ...values, regency_id: value })}><SelectTrigger className="w-full" aria-labelledby="schedule-regency-label"><SelectValue placeholder="Pilih kabupaten" /></SelectTrigger><SelectContent>{regencies.data?.data.filter((item) => item.is_active).map((item) => <SelectItem key={item.id} value={item.id}>{item.document_code} - {item.name}</SelectItem>)}</SelectContent></Select></div>
      <div className="grid gap-2"><Label id="schedule-package-label">Template paket</Label><Select required value={values.package_template_version_id} onValueChange={(value) => setValues({ ...values, package_template_version_id: value })}><SelectTrigger className="w-full" aria-labelledby="schedule-package-label"><SelectValue placeholder="Pilih template" /></SelectTrigger><SelectContent>{compatiblePackages.map((item) => <SelectItem key={item.id} value={item.id}>{item.name} v{item.version}</SelectItem>)}</SelectContent></Select></div>
      <div className="grid gap-2"><Label id="schedule-document-label">Template dokumentasi</Label><Select required value={values.documentation_template_version_id} onValueChange={(value) => setValues({ ...values, documentation_template_version_id: value })}><SelectTrigger className="w-full" aria-labelledby="schedule-document-label"><SelectValue placeholder="Pilih template" /></SelectTrigger><SelectContent>{compatibleDocuments.map((item) => <SelectItem key={item.id} value={item.id}>{item.name} v{item.version}</SelectItem>)}</SelectContent></Select></div>
      <div className="grid gap-2"><Label id="schedule-status-label">Status</Label><Select value={values.status} onValueChange={(value) => setValues({ ...values, status: value })}><SelectTrigger className="w-full" aria-labelledby="schedule-status-label"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="draft">Draft</SelectItem><SelectItem value="active">Aktif</SelectItem><SelectItem value="completed">Selesai</SelectItem><SelectItem value="cancelled">Dibatalkan</SelectItem></SelectContent></Select></div>
      <FormField label="Mulai" name="start_date" type="date" required value={values.start_date} onChange={(e) => setValues({ ...values, start_date: e.target.value })} />
      <FormField label="Selesai" name="end_date" type="date" required value={values.end_date} onChange={(e) => setValues({ ...values, end_date: e.target.value })} />
      <FormField label="Digit nomor pembagian" name="distribution_number_padding" type="number" min={1} max={8} value={values.distribution_number_padding} onChange={(e) => setValues({ ...values, distribution_number_padding: Number(e.target.value) })} />
      <FormField label="Konsultan pengawas" name="supervisor_name" value={values.supervisor_name} onChange={(e) => setValues({ ...values, supervisor_name: e.target.value })} />
      <FormField className="sm:col-span-2" label="Catatan" name="notes" value={values.notes} onChange={(e) => setValues({ ...values, notes: e.target.value })} />
      {mutation.isError && <Alert className="sm:col-span-2" variant="destructive"><AlertDescription>Jadwal belum dapat disimpan. Periksa kecocokan jenis template.</AlertDescription></Alert>}
    </SetupDialog>
  </section>;
}
