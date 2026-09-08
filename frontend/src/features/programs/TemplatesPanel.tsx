import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Camera, Edit3, PackageCheck, Plus, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { toast } from 'sonner';
import { DataState } from '../../components/DataState';
import { DataTable } from '../../components/DataTable';
import { FormField } from '../../components/FormField';
import { StatusBadge } from '../../components/StatusBadge';
import { Button } from '../../components/ui/button';
import { Alert, AlertDescription } from '../../components/ui/alert';
import { Badge } from '../../components/ui/badge';
import { Checkbox } from '../../components/ui/checkbox';
import { Label } from '../../components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../components/ui/select';
import { Separator } from '../../components/ui/separator';
import { apiRequest } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { SetupDialog } from './SetupDialog';
import { DataResponse, DocumentationSlot, DocumentationTemplate, HoseOption, MachineOption, PackageComponent, PackageTemplate, ProgramType, programTypeLabel } from './types';

const newSlot = (index = 0): DocumentationSlot => ({ slot_code: '', label: '', stage: 'distribution', is_required: true, min_files: 1, max_files: 1, input_source: 'both', require_location: false, require_captured_at: false, sort_order: (index + 1) * 10 });
const newMachineOption = (): MachineOption => ({ code: '', brand: '', type: '' });
const newHoseOption = (): HoseOption => ({ code: '', brand: '', spec: '' });
const newComponent = (): PackageComponent => ({ code: '', label: '', quantity: 1, unit: '' });

const emptyPackageValues = { template_code: '', name: '', program_type: 'farmer' as ProgramType, status: 'draft', converter_brand: '', machine_options: [] as MachineOption[], hose_options: [] as HoseOption[], components: [] as PackageComponent[] };

function asString(value: unknown): string {
  return typeof value === 'string' ? value : '';
}
function asArray<T>(value: unknown): T[] {
  return Array.isArray(value) ? value as T[] : [];
}

export function TemplatesPanel() {
  const canManage = useCan('programs.manage'); const client = useQueryClient();
  const packages = useQuery({ queryKey: ['program-setup', 'package-templates'], queryFn: () => apiRequest<DataResponse<PackageTemplate[]>>('/api/v1/program-setup/package-templates') });
  const documents = useQuery({ queryKey: ['program-setup', 'documentation-templates'], queryFn: () => apiRequest<DataResponse<DocumentationTemplate[]>>('/api/v1/program-setup/documentation-templates') });
  const [packageOpen, setPackageOpen] = useState(false); const [packageEditing, setPackageEditing] = useState<PackageTemplate>();
  const [packageValues, setPackageValues] = useState(emptyPackageValues);
  const [documentOpen, setDocumentOpen] = useState(false); const [documentEditing, setDocumentEditing] = useState<DocumentationTemplate>();
  const [documentValues, setDocumentValues] = useState({ template_code: '', name: '', program_type: 'farmer' as ProgramType, status: 'draft', slots: [newSlot()] });
  const packageMutation = useMutation({
    mutationFn: () => apiRequest(`/api/v1/program-setup/package-templates${packageEditing ? `/${packageEditing.id}` : ''}`, {
      method: packageEditing ? 'PATCH' : 'POST',
      body: JSON.stringify({
        template_code: packageValues.template_code, name: packageValues.name, program_type: packageValues.program_type, status: packageValues.status,
        values: { converter_brand: packageValues.converter_brand, machine_options: packageValues.machine_options, hose_options: packageValues.hose_options, components: packageValues.components },
      }),
    }),
    onSuccess: () => { setPackageOpen(false); client.invalidateQueries({ queryKey: ['program-setup', 'package-templates'] }); toast.success('Template paket berhasil disimpan.'); },
  });
  const documentMutation = useMutation({ mutationFn: () => apiRequest(`/api/v1/program-setup/documentation-templates${documentEditing ? `/${documentEditing.id}` : ''}`, { method: documentEditing ? 'PATCH' : 'POST', body: JSON.stringify(documentValues) }), onSuccess: () => { setDocumentOpen(false); client.invalidateQueries({ queryKey: ['program-setup', 'documentation-templates'] }); toast.success('Template dokumentasi berhasil disimpan.'); } });
  const showPackage = (item?: PackageTemplate) => { setPackageEditing(item); setPackageValues(item ? { template_code: item.template_code, name: item.name, program_type: item.program_type, status: item.status, converter_brand: asString(item.values.converter_brand), machine_options: asArray<MachineOption>(item.values.machine_options), hose_options: asArray<HoseOption>(item.values.hose_options), components: asArray<PackageComponent>(item.values.components) } : emptyPackageValues); setPackageOpen(true); };
  const showDocument = (item?: DocumentationTemplate) => { setDocumentEditing(item); setDocumentValues(item ? { template_code: item.template_code, name: item.name, program_type: item.program_type, status: item.status, slots: item.slots } : { template_code: '', name: '', program_type: 'farmer', status: 'draft', slots: [newSlot()] }); setDocumentOpen(true); };
  const updateSlot = (index: number, patch: Partial<DocumentationSlot>) => setDocumentValues({ ...documentValues, slots: documentValues.slots.map((slot, slotIndex) => slotIndex === index ? { ...slot, ...patch } : slot) });
  const updateMachineOption = (index: number, patch: Partial<MachineOption>) => setPackageValues({ ...packageValues, machine_options: packageValues.machine_options.map((option, optionIndex) => optionIndex === index ? { ...option, ...patch } : option) });
  const updateHoseOption = (index: number, patch: Partial<HoseOption>) => setPackageValues({ ...packageValues, hose_options: packageValues.hose_options.map((option, optionIndex) => optionIndex === index ? { ...option, ...patch } : option) });
  const updateComponent = (index: number, patch: Partial<PackageComponent>) => setPackageValues({ ...packageValues, components: packageValues.components.map((component, componentIndex) => componentIndex === index ? { ...component, ...patch } : component) });
  return <div className="templateGrid"><section className="setupPanel" aria-labelledby="package-templates-heading"><header className="panelHeading"><div><h2 id="package-templates-heading"><PackageCheck />Template paket</h2><p>Nilai baku komponen yang disalin ke setiap alokasi.</p></div>{canManage && <Button type="button" onClick={() => showPackage()}><Plus />Tambah paket</Button>}</header>
    {packages.isError ? <DataState kind="error" title="Template paket belum dapat dimuat" description="Muat ulang halaman untuk mencoba kembali." /> : packages.data?.data.length ? <DataTable label="Daftar template paket"><thead><tr><th>Template</th><th>Jenis</th><th>Versi</th><th>Status</th>{canManage && <th>Aksi</th>}</tr></thead><tbody>{packages.data.data.map((item) => <tr key={item.id}><td><strong>{item.name}</strong><small className="subline">{item.template_code}</small></td><td><Badge variant="outline">{programTypeLabel(item.program_type)}</Badge></td><td>v{item.version}</td><td><StatusBadge active={item.status === 'published'} activeText="Published" inactiveText={item.status === 'draft' ? 'Draft' : 'Retired'} /></td>{canManage && <td><Button type="button" variant="ghost" size="icon" aria-label={`Edit ${item.name}`} onClick={() => showPackage(item)}><Edit3 /></Button></td>}</tr>)}</tbody></DataTable> : <DataState kind="empty" title="Belum ada template paket" description="Tambahkan nilai baku komponen untuk program bantuan." />}
  </section><Separator /><section className="setupPanel" aria-labelledby="documentation-templates-heading"><header className="panelHeading"><div><h2 id="documentation-templates-heading"><Camera />Template dokumentasi</h2><p>Slot foto dapat berbeda untuk setiap jenis program.</p></div>{canManage && <Button type="button" onClick={() => showDocument()}><Plus />Tambah dokumentasi</Button>}</header>
    {documents.isError ? <DataState kind="error" title="Template dokumentasi belum dapat dimuat" description="Muat ulang halaman untuk mencoba kembali." /> : documents.data?.data.length ? <DataTable label="Daftar template dokumentasi"><thead><tr><th>Template</th><th>Jenis</th><th>Versi</th><th>Slot</th><th>Status</th>{canManage && <th>Aksi</th>}</tr></thead><tbody>{documents.data.data.map((item) => <tr key={item.id}><td><strong>{item.name}</strong><small className="subline">{item.template_code}</small></td><td><Badge variant="outline">{programTypeLabel(item.program_type)}</Badge></td><td>v{item.version}</td><td>{item.slots.length}</td><td><StatusBadge active={item.status === 'published'} activeText="Published" inactiveText={item.status === 'draft' ? 'Draft' : 'Retired'} /></td>{canManage && <td><Button type="button" variant="ghost" size="icon" aria-label={`Edit ${item.name}`} onClick={() => showDocument(item)}><Edit3 /></Button></td>}</tr>)}</tbody></DataTable> : <DataState kind="empty" title="Belum ada template dokumentasi" description="Tambahkan slot foto untuk mendukung pelaporan lapangan." />}
    </section>
    <SetupDialog open={packageOpen} onOpenChange={setPackageOpen} title={packageEditing ? `Edit ${packageEditing.name}` : 'Tambah template paket'} description="Perubahan pada versi published akan membuat draft versi baru." pending={packageMutation.isPending} onSubmit={() => packageMutation.mutate()}>
      <FormField label="Kode template" name="package_code" required disabled={Boolean(packageEditing)} value={packageValues.template_code} onChange={(e) => setPackageValues({ ...packageValues, template_code: e.target.value.toUpperCase() })} />
      <FormField label="Nama template" name="package_name" required value={packageValues.name} onChange={(e) => setPackageValues({ ...packageValues, name: e.target.value.toUpperCase() })} />
      <div className="grid gap-2"><Label id="package-type-label">Jenis program</Label><Select value={packageValues.program_type} onValueChange={(value) => setPackageValues({ ...packageValues, program_type: value as ProgramType })}><SelectTrigger className="w-full" aria-labelledby="package-type-label"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="farmer">Petani</SelectItem><SelectItem value="fisherman">Nelayan</SelectItem></SelectContent></Select></div>
      <div className="grid gap-2"><Label id="package-status-label">Status</Label><Select value={packageValues.status} onValueChange={(value) => setPackageValues({ ...packageValues, status: value })}><SelectTrigger className="w-full" aria-labelledby="package-status-label"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="draft">Draft</SelectItem><SelectItem value="published">Published</SelectItem><SelectItem value="retired">Retired</SelectItem></SelectContent></Select></div>
      <FormField label="Merk Konkit/Reducer" name="converter_brand" value={packageValues.converter_brand} onChange={(e) => setPackageValues({ ...packageValues, converter_brand: e.target.value.toUpperCase() })} />

      <div className="slotEditor fullField"><div className="slotEditorHead"><strong>Opsi merk/tipe mesin</strong><Button type="button" variant="outline" onClick={() => setPackageValues({ ...packageValues, machine_options: [...packageValues.machine_options, newMachineOption()] })}><Plus />Tambah opsi mesin</Button></div>
        {packageValues.machine_options.map((option, index) => <div className="equipmentRow" key={index}>
          <FormField label={`Kode mesin ${index + 1}`} name={`machine_code_${index}`} required value={option.code} onChange={(e) => updateMachineOption(index, { code: e.target.value.toLowerCase() })} />
          <FormField label={`Merk mesin ${index + 1}`} name={`machine_brand_${index}`} required value={option.brand} onChange={(e) => updateMachineOption(index, { brand: e.target.value.toUpperCase() })} />
          <FormField label={`Tipe mesin ${index + 1}`} name={`machine_type_${index}`} required value={option.type} onChange={(e) => updateMachineOption(index, { type: e.target.value.toUpperCase() })} />
          <Button type="button" variant="destructive" size="icon" className="dangerIcon" aria-label={`Hapus opsi mesin ${index + 1}`} onClick={() => setPackageValues({ ...packageValues, machine_options: packageValues.machine_options.filter((_, optionIndex) => optionIndex !== index) })}><Trash2 /></Button>
        </div>)}
      </div>

      <div className="slotEditor fullField"><div className="slotEditorHead"><strong>Opsi merk/spesifikasi selang</strong><Button type="button" variant="outline" onClick={() => setPackageValues({ ...packageValues, hose_options: [...packageValues.hose_options, newHoseOption()] })}><Plus />Tambah opsi selang</Button></div>
        {packageValues.hose_options.map((option, index) => <div className="equipmentRow" key={index}>
          <FormField label={`Kode selang ${index + 1}`} name={`hose_code_${index}`} required value={option.code} onChange={(e) => updateHoseOption(index, { code: e.target.value.toLowerCase() })} />
          <FormField label={`Merk selang ${index + 1}`} name={`hose_brand_${index}`} required value={option.brand} onChange={(e) => updateHoseOption(index, { brand: e.target.value.toUpperCase() })} />
          <FormField label={`Spesifikasi selang ${index + 1}`} name={`hose_spec_${index}`} required value={option.spec} onChange={(e) => updateHoseOption(index, { spec: e.target.value.toUpperCase() })} />
          <Button type="button" variant="destructive" size="icon" className="dangerIcon" aria-label={`Hapus opsi selang ${index + 1}`} onClick={() => setPackageValues({ ...packageValues, hose_options: packageValues.hose_options.filter((_, optionIndex) => optionIndex !== index) })}><Trash2 /></Button>
        </div>)}
      </div>

      <div className="slotEditor fullField"><div className="slotEditorHead"><strong>Komponen paket, aksesoris & kelengkapan</strong><Button type="button" variant="outline" onClick={() => setPackageValues({ ...packageValues, components: [...packageValues.components, newComponent()] })}><Plus />Tambah komponen</Button></div>
        {packageValues.components.map((component, index) => <div className="componentRow" key={index}>
          <FormField label={`Kode komponen ${index + 1}`} name={`component_code_${index}`} required value={component.code} onChange={(e) => updateComponent(index, { code: e.target.value.toLowerCase() })} />
          <FormField label={`Nama komponen ${index + 1}`} name={`component_label_${index}`} required value={component.label} onChange={(e) => updateComponent(index, { label: e.target.value })} />
          <FormField label={`Jumlah komponen ${index + 1}`} name={`component_quantity_${index}`} type="number" min={1} required value={component.quantity} onChange={(e) => updateComponent(index, { quantity: Number(e.target.value) })} />
          <FormField label={`Satuan komponen ${index + 1}`} name={`component_unit_${index}`} required value={component.unit} onChange={(e) => updateComponent(index, { unit: e.target.value })} />
          <Button type="button" variant="destructive" size="icon" className="dangerIcon" aria-label={`Hapus komponen ${index + 1}`} onClick={() => setPackageValues({ ...packageValues, components: packageValues.components.filter((_, componentIndex) => componentIndex !== index) })}><Trash2 /></Button>
        </div>)}
      </div>
      {packageMutation.isError && <Alert className="sm:col-span-2" variant="destructive"><AlertDescription>Template paket belum dapat disimpan.</AlertDescription></Alert>}
    </SetupDialog>
    <SetupDialog open={documentOpen} onOpenChange={setDocumentOpen} title={documentEditing ? `Edit ${documentEditing.name}` : 'Tambah template dokumentasi'} description="Perubahan pada versi published akan membuat draft versi baru." pending={documentMutation.isPending} onSubmit={() => documentMutation.mutate()}>
      <FormField label="Kode template" name="document_code" required disabled={Boolean(documentEditing)} value={documentValues.template_code} onChange={(e) => setDocumentValues({ ...documentValues, template_code: e.target.value.toUpperCase() })} />
      <FormField label="Nama template" name="document_name" required value={documentValues.name} onChange={(e) => setDocumentValues({ ...documentValues, name: e.target.value.toUpperCase() })} />
      <div className="grid gap-2"><Label id="document-type-label">Jenis program</Label><Select value={documentValues.program_type} onValueChange={(value) => setDocumentValues({ ...documentValues, program_type: value as ProgramType })}><SelectTrigger className="w-full" aria-labelledby="document-type-label"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="farmer">Petani</SelectItem><SelectItem value="fisherman">Nelayan</SelectItem></SelectContent></Select></div>
      <div className="grid gap-2"><Label id="document-status-label">Status</Label><Select value={documentValues.status} onValueChange={(value) => setDocumentValues({ ...documentValues, status: value })}><SelectTrigger className="w-full" aria-labelledby="document-status-label"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="draft">Draft</SelectItem><SelectItem value="published">Published</SelectItem><SelectItem value="retired">Retired</SelectItem></SelectContent></Select></div>
      <div className="slotEditor fullField"><div className="slotEditorHead"><strong>Slot dokumentasi</strong><Button type="button" variant="outline" onClick={() => setDocumentValues({ ...documentValues, slots: [...documentValues.slots, newSlot(documentValues.slots.length)] })}><Plus />Tambah slot</Button></div>{documentValues.slots.map((slot, index) => <div className="slotRow" key={index}><FormField label="Kode slot" name={`slot_code_${index}`} required value={slot.slot_code} onChange={(e) => updateSlot(index, { slot_code: e.target.value.toLowerCase() })} /><FormField label="Judul" name={`slot_label_${index}`} required value={slot.label} onChange={(e) => updateSlot(index, { label: e.target.value })} /><div className="grid gap-2"><Label id={`slot-source-${index}`}>Sumber</Label><Select value={slot.input_source} onValueChange={(value) => updateSlot(index, { input_source: value as DocumentationSlot['input_source'] })}><SelectTrigger className="w-full" aria-labelledby={`slot-source-${index}`}><SelectValue /></SelectTrigger><SelectContent><SelectItem value="both">Kamera & galeri</SelectItem><SelectItem value="camera">Kamera</SelectItem><SelectItem value="gallery">Galeri</SelectItem></SelectContent></Select></div><FormField label="Minimal" name={`slot_min_${index}`} type="number" min={0} value={slot.min_files} onChange={(e) => updateSlot(index, { min_files: Number(e.target.value) })} /><label className="flex min-h-11 items-center gap-3"><Checkbox checked={slot.is_required} onCheckedChange={(checked) => updateSlot(index, { is_required: checked === true })} />Wajib</label><Button type="button" variant="destructive" size="icon" className="dangerIcon" aria-label={`Hapus slot ${slot.label || index + 1}`} onClick={() => setDocumentValues({ ...documentValues, slots: documentValues.slots.filter((_, slotIndex) => slotIndex !== index) })}><Trash2 /></Button></div>)}</div>
      {documentMutation.isError && <Alert className="sm:col-span-2" variant="destructive"><AlertDescription>Template dokumentasi belum dapat disimpan.</AlertDescription></Alert>}
    </SetupDialog>
  </div>;
}
