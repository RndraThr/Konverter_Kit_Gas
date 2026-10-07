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
import { uppercaseBusinessText } from '../../lib/text';
import { SetupDialog, SetupFormSection } from './SetupDialog';
import { SetupToolbar } from './SetupToolbar';
import { ConverterOption, DataResponse, DocumentationSlot, DocumentationTemplate, HoseOption, MachineOption, PackageComponent, PackageTemplate, ProgramType, programTypeLabel } from './types';

const newSlot = (index = 0): DocumentationSlot => ({ slot_code: '', label: '', stage: 'mesin', is_required: true, min_files: 1, max_files: 1, input_source: 'both', media_kind: 'image', require_location: false, require_captured_at: false, sort_order: (index + 1) * 10 });
const newMachineOption = (): MachineOption => ({ code: '', brand: '', type: '', power: '', fuel_type: '' });
const newHoseOption = (): HoseOption => ({ code: '', suction_brand: '', suction_spec: '', discharge_brand: '', discharge_spec: '' });
const newConverterOption = (): ConverterOption => ({ code: '', brand: '' });
const newComponent = (): PackageComponent => ({ code: '', label: '', quantity: 1, unit: '' });

const emptyPackageValues = { template_code: '', name: '', program_type: 'farmer' as ProgramType, status: 'draft', machine_options: [] as MachineOption[], hose_options: [] as HoseOption[], converter_options: [] as ConverterOption[], components: [] as PackageComponent[] };
const createEmptyDocumentValues = () => ({ template_code: '', name: '', program_type: 'farmer' as ProgramType, status: 'draft', slots: [newSlot()] });

function asArray<T>(value: unknown): T[] {
  return Array.isArray(value) ? value as T[] : [];
}

function legacyPair(value = '') {
  const parts = value.split(/\s*(?:\/|;|\n)\s*/).filter(Boolean);
  return [parts[0] ?? '', parts[1] ?? parts[0] ?? ''] as const;
}

function normalizeMachineOption(option: Partial<MachineOption>): MachineOption {
  return { code: option.code ?? '', brand: option.brand ?? '', type: option.type ?? '', power: option.power ?? '', fuel_type: option.fuel_type ?? '' };
}

function normalizeHoseOption(option: Partial<HoseOption>): HoseOption {
  const brands = legacyPair(option.brand);
  const specs = legacyPair(option.spec);
  return {
    code: option.code ?? '',
    suction_brand: option.suction_brand ?? brands[0],
    suction_spec: option.suction_spec ?? specs[0],
    discharge_brand: option.discharge_brand ?? brands[1],
    discharge_spec: option.discharge_spec ?? specs[1],
  };
}

function normalizeDocumentationSlot(slot: Partial<DocumentationSlot>, index: number): DocumentationSlot {
  const mediaKind = slot.media_kind === 'video' || slot.media_kind === 'image_video' ? slot.media_kind : 'image';
  return { ...newSlot(index), ...slot, media_kind: mediaKind };
}

function revealAddedField(id: string) {
  window.requestAnimationFrame(() => {
    const field = document.getElementById(id);
    if (!(field instanceof HTMLElement)) return;
    if (typeof field.scrollIntoView === 'function') field.scrollIntoView({ block: 'nearest', inline: 'nearest' });
    field.focus({ preventScroll: true });
  });
}

function RepeatableEditorHead({ title, count, addLabel, onAdd }: { title: string; count: number; addLabel: string; onAdd: () => void }) {
  return <div className="slotEditorHead">
    <div className="slotEditorMeta"><span>{title}</span><small aria-live="polite">{count} item</small></div>
    <Button type="button" variant="outline" onClick={onAdd}><Plus />{addLabel}</Button>
  </div>;
}

export function TemplatesPanel() {
  const canManage = useCan('programs.manage'); const client = useQueryClient();
  const packages = useQuery({ queryKey: ['program-setup', 'package-templates'], queryFn: () => apiRequest<DataResponse<PackageTemplate[]>>('/api/v1/program-setup/package-templates') });
  const documents = useQuery({ queryKey: ['program-setup', 'documentation-templates'], queryFn: () => apiRequest<DataResponse<DocumentationTemplate[]>>('/api/v1/program-setup/documentation-templates') });
  const [packageOpen, setPackageOpen] = useState(false); const [packageEditing, setPackageEditing] = useState<PackageTemplate>();
  const [packageValues, setPackageValues] = useState(emptyPackageValues);
  const [initialPackageValues, setInitialPackageValues] = useState(emptyPackageValues);
  const [documentOpen, setDocumentOpen] = useState(false); const [documentEditing, setDocumentEditing] = useState<DocumentationTemplate>();
  const [documentValues, setDocumentValues] = useState(createEmptyDocumentValues);
  const [initialDocumentValues, setInitialDocumentValues] = useState(createEmptyDocumentValues);
  const [search, setSearch] = useState(''); const [status, setStatus] = useState('all');
  const packageMutation = useMutation({
    mutationFn: () => apiRequest(`/api/v1/program-setup/package-templates${packageEditing ? `/${packageEditing.id}` : ''}`, {
      method: packageEditing ? 'PATCH' : 'POST',
      body: JSON.stringify({
        template_code: packageValues.template_code, name: packageValues.name, program_type: packageValues.program_type, status: packageValues.status,
        values: { machine_options: packageValues.machine_options, hose_options: packageValues.hose_options, converter_options: packageValues.converter_options, components: packageValues.components },
      }),
    }),
    onSuccess: () => { setPackageOpen(false); client.invalidateQueries({ queryKey: ['program-setup', 'package-templates'] }); toast.success('Template paket berhasil disimpan.'); },
  });
  const documentMutation = useMutation({ mutationFn: () => apiRequest(`/api/v1/program-setup/documentation-templates${documentEditing ? `/${documentEditing.id}` : ''}`, { method: documentEditing ? 'PATCH' : 'POST', body: JSON.stringify(documentValues) }), onSuccess: () => { setDocumentOpen(false); client.invalidateQueries({ queryKey: ['program-setup', 'documentation-templates'] }); toast.success('Template dokumentasi berhasil disimpan.'); } });
  const showPackage = (item?: PackageTemplate) => { const nextValues = item ? { template_code: item.template_code, name: item.name, program_type: item.program_type, status: item.status, machine_options: asArray<Partial<MachineOption>>(item.values.machine_options).map(normalizeMachineOption), hose_options: asArray<Partial<HoseOption>>(item.values.hose_options).map(normalizeHoseOption), converter_options: asArray<ConverterOption>(item.values.converter_options), components: asArray<PackageComponent>(item.values.components) } : emptyPackageValues; setPackageEditing(item); setPackageValues(nextValues); setInitialPackageValues(nextValues); setPackageOpen(true); };
  const showDocument = (item?: DocumentationTemplate) => { const nextValues = item ? { template_code: item.template_code, name: item.name, program_type: item.program_type, status: item.status, slots: item.slots.map(normalizeDocumentationSlot) } : createEmptyDocumentValues(); setDocumentEditing(item); setDocumentValues(nextValues); setInitialDocumentValues(nextValues); setDocumentOpen(true); };
  const updateSlot = (index: number, patch: Partial<DocumentationSlot>) => {
    const normalizedPatch = {
      ...patch,
      ...(typeof patch.label === 'string' ? { label: uppercaseBusinessText(patch.label) } : {}),
      ...(typeof patch.instructions === 'string' ? { instructions: uppercaseBusinessText(patch.instructions) } : {}),
    };
    setDocumentValues({ ...documentValues, slots: documentValues.slots.map((slot, slotIndex) => slotIndex === index ? { ...slot, ...normalizedPatch } : slot) });
  };
  const updateMachineOption = (index: number, patch: Partial<MachineOption>) => setPackageValues({ ...packageValues, machine_options: packageValues.machine_options.map((option, optionIndex) => optionIndex === index ? { ...option, ...patch } : option) });
  const updateHoseOption = (index: number, patch: Partial<HoseOption>) => setPackageValues({ ...packageValues, hose_options: packageValues.hose_options.map((option, optionIndex) => optionIndex === index ? { ...option, ...patch } : option) });
  const updateConverterOption = (index: number, patch: Partial<ConverterOption>) => setPackageValues({ ...packageValues, converter_options: packageValues.converter_options.map((option, optionIndex) => optionIndex === index ? { ...option, ...patch } : option) });
  const updateComponent = (index: number, patch: Partial<PackageComponent>) => setPackageValues({ ...packageValues, components: packageValues.components.map((component, componentIndex) => componentIndex === index ? { ...component, ...patch } : component) });
  const addMachineOption = () => { const index = packageValues.machine_options.length; setPackageValues({ ...packageValues, machine_options: [...packageValues.machine_options, newMachineOption()] }); revealAddedField(`machine_code_${index}`); };
  const addHoseOption = () => { const index = packageValues.hose_options.length; setPackageValues({ ...packageValues, hose_options: [...packageValues.hose_options, newHoseOption()] }); revealAddedField(`hose_code_${index}`); };
  const addConverterOption = () => { const index = packageValues.converter_options.length; setPackageValues({ ...packageValues, converter_options: [...packageValues.converter_options, newConverterOption()] }); revealAddedField(`converter_code_${index}`); };
  const addComponent = () => { const index = packageValues.components.length; setPackageValues({ ...packageValues, components: [...packageValues.components, newComponent()] }); revealAddedField(`component_code_${index}`); };
  const addDocumentationSlot = () => { const index = documentValues.slots.length; setDocumentValues({ ...documentValues, slots: [...documentValues.slots, newSlot(index)] }); revealAddedField(`slot_code_${index}`); };
  const normalizedSearch = search.trim().toLocaleLowerCase('id-ID');
  const packageItems = (packages.data?.data ?? []).filter((item) => (!normalizedSearch || `${item.name} ${item.template_code} ${programTypeLabel(item.program_type)}`.toLocaleLowerCase('id-ID').includes(normalizedSearch)) && (status === 'all' || item.status === status));
  const documentItems = (documents.data?.data ?? []).filter((item) => (!normalizedSearch || `${item.name} ${item.template_code} ${programTypeLabel(item.program_type)}`.toLocaleLowerCase('id-ID').includes(normalizedSearch)) && (status === 'all' || item.status === status));
  const totalTemplates = (packages.data?.data.length ?? 0) + (documents.data?.data.length ?? 0);
  return <div className="templateGrid"><SetupToolbar entity="template" search={search} onSearchChange={setSearch} status={status} onStatusChange={setStatus} options={[{ value: 'all', label: 'Semua status' }, { value: 'published', label: 'Terbit' }, { value: 'draft', label: 'Draf' }, { value: 'retired', label: 'Diarsipkan' }]} shown={packageItems.length + documentItems.length} total={totalTemplates} /><section className="setupPanel" aria-labelledby="package-templates-heading"><header className="panelHeading"><div><h2 id="package-templates-heading"><PackageCheck />Template paket</h2><p>Nilai baku komponen yang disalin ke setiap alokasi.</p></div>{canManage && <Button type="button" onClick={() => showPackage()}><Plus />Tambah paket</Button>}</header>
    {packages.isPending ? <DataState kind="loading" title="Memuat template paket" description="Mengambil versi template paket terbaru." /> : packages.isError ? <DataState kind="error" title="Template paket belum dapat dimuat" description="Periksa koneksi, lalu coba kembali." action={{ label: 'Coba lagi', onClick: () => packages.refetch() }} /> : packageItems.length ? <DataTable label="Daftar template paket"><thead><tr><th>Template</th><th>Jenis</th><th>Versi</th><th>Status</th>{canManage && <th className="actionColumn">Aksi</th>}</tr></thead><tbody>{packageItems.map((item) => <tr key={item.id}><td><strong>{item.name}</strong><small className="subline">{item.template_code}</small></td><td><Badge variant="outline">{programTypeLabel(item.program_type)}</Badge></td><td>v{item.version}</td><td><StatusBadge active={item.status === 'published'} activeText="Terbit" inactiveText={item.status === 'draft' ? 'Draf' : 'Diarsipkan'} /></td>{canManage && <td className="actionColumn"><Button type="button" variant="ghost" size="icon" aria-label={`Edit ${item.name}`} title={`Edit ${item.name}`} onClick={() => showPackage(item)}><Edit3 /></Button></td>}</tr>)}</tbody></DataTable> : <DataState kind="empty" title={(packages.data?.data.length ?? 0) ? 'Tidak ada template paket yang sesuai' : 'Belum ada template paket'} description={(packages.data?.data.length ?? 0) ? 'Ubah kata kunci atau filter untuk menampilkan data lain.' : 'Tambahkan nilai baku komponen untuk program bantuan.'} />}
  </section><Separator /><section className="setupPanel" aria-labelledby="documentation-templates-heading"><header className="panelHeading"><div><h2 id="documentation-templates-heading"><Camera />Template dokumentasi</h2><p>Slot foto dapat berbeda untuk setiap jenis program.</p></div>{canManage && <Button type="button" onClick={() => showDocument()}><Plus />Tambah dokumentasi</Button>}</header>
    {documents.isPending ? <DataState kind="loading" title="Memuat template dokumentasi" description="Mengambil konfigurasi slot bukti terbaru." /> : documents.isError ? <DataState kind="error" title="Template dokumentasi belum dapat dimuat" description="Periksa koneksi, lalu coba kembali." action={{ label: 'Coba lagi', onClick: () => documents.refetch() }} /> : documentItems.length ? <DataTable label="Daftar template dokumentasi"><thead><tr><th>Template</th><th>Jenis</th><th>Versi</th><th>Slot</th><th>Status</th>{canManage && <th className="actionColumn">Aksi</th>}</tr></thead><tbody>{documentItems.map((item) => <tr key={item.id}><td><strong>{item.name}</strong><small className="subline">{item.template_code}</small></td><td><Badge variant="outline">{programTypeLabel(item.program_type)}</Badge></td><td>v{item.version}</td><td>{item.slots.length}</td><td><StatusBadge active={item.status === 'published'} activeText="Terbit" inactiveText={item.status === 'draft' ? 'Draf' : 'Diarsipkan'} /></td>{canManage && <td className="actionColumn"><Button type="button" variant="ghost" size="icon" aria-label={`Edit ${item.name}`} title={`Edit ${item.name}`} onClick={() => showDocument(item)}><Edit3 /></Button></td>}</tr>)}</tbody></DataTable> : <DataState kind="empty" title={(documents.data?.data.length ?? 0) ? 'Tidak ada template dokumentasi yang sesuai' : 'Belum ada template dokumentasi'} description={(documents.data?.data.length ?? 0) ? 'Ubah kata kunci atau filter untuk menampilkan data lain.' : 'Tambahkan slot foto untuk mendukung pelaporan lapangan.'} />}
    </section>
    <SetupDialog layout="workspace" open={packageOpen} onOpenChange={setPackageOpen} title={packageEditing ? `Edit ${packageEditing.name}` : 'Tambah template paket'} description="Perubahan pada versi terbit akan membuat versi draf baru." pending={packageMutation.isPending} dirty={JSON.stringify(packageValues) !== JSON.stringify(initialPackageValues)} onSubmit={() => packageMutation.mutate()}>
      <SetupFormSection title="Identitas template" description="Kode, nama, jenis program, dan status versi template.">
      <FormField label="Kode template" name="package_code" required readOnly={Boolean(packageEditing)} value={packageValues.template_code} onChange={(e) => setPackageValues({ ...packageValues, template_code: e.target.value.toUpperCase() })} hint={packageEditing ? 'Kode template menjadi identitas versi dan tidak dapat diubah.' : 'Gunakan kode yang singkat dan unik.'} />
      <FormField label="Nama template" name="package_name" required value={packageValues.name} onChange={(e) => setPackageValues({ ...packageValues, name: uppercaseBusinessText(e.target.value) })} />
      <div className="grid min-w-0 gap-2"><Label id="package-type-label">Jenis program</Label><Select value={packageValues.program_type} onValueChange={(value) => setPackageValues({ ...packageValues, program_type: (value ?? 'farmer') as ProgramType })}><SelectTrigger className="w-full" aria-labelledby="package-type-label"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="farmer">Petani</SelectItem><SelectItem value="fisherman">Nelayan</SelectItem></SelectContent></Select></div>
      <div className="grid min-w-0 gap-2"><Label id="package-status-label">Status</Label><Select value={packageValues.status} onValueChange={(value) => setPackageValues({ ...packageValues, status: value ?? 'draft' })}><SelectTrigger className="w-full" aria-labelledby="package-status-label"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="draft">Draf</SelectItem><SelectItem value="published">Terbit</SelectItem><SelectItem value="retired">Diarsipkan</SelectItem></SelectContent></Select></div>
      </SetupFormSection>

      <SetupFormSection title="Opsi mesin" description="Daftar merk dan tipe mesin yang dapat dipilih pada alokasi."><div className="slotEditor fullField"><RepeatableEditorHead title="Merk dan tipe mesin" count={packageValues.machine_options.length} addLabel="Tambah opsi mesin" onAdd={addMachineOption} />
        {packageValues.machine_options.map((option, index) => <div className="equipmentRow" role="group" aria-label={`Opsi mesin ${index + 1}`} key={index}>
          <div className="repeatableRowHeader"><strong>Opsi mesin {index + 1}</strong><Button type="button" variant="ghost" size="sm" className="dangerIcon" aria-label={`Hapus opsi mesin ${index + 1}`} onClick={() => setPackageValues({ ...packageValues, machine_options: packageValues.machine_options.filter((_, optionIndex) => optionIndex !== index) })}><Trash2 />Hapus</Button></div>
          <FormField label={`Kode mesin ${index + 1}`} name={`machine_code_${index}`} required value={option.code} onChange={(e) => updateMachineOption(index, { code: e.target.value.toLowerCase() })} />
          <FormField label={`Merk mesin ${index + 1}`} name={`machine_brand_${index}`} required value={option.brand} onChange={(e) => updateMachineOption(index, { brand: uppercaseBusinessText(e.target.value) })} />
          <FormField label={`Tipe mesin ${index + 1}`} name={`machine_type_${index}`} required value={option.type} onChange={(e) => updateMachineOption(index, { type: uppercaseBusinessText(e.target.value) })} />
          <FormField label={`Daya mesin ${index + 1}`} name={`machine_power_${index}`} required value={option.power} onChange={(e) => updateMachineOption(index, { power: uppercaseBusinessText(e.target.value) })} />
          <FormField label={`Jenis BBM ${index + 1}`} name={`machine_fuel_type_${index}`} required value={option.fuel_type} onChange={(e) => updateMachineOption(index, { fuel_type: uppercaseBusinessText(e.target.value) })} />
        </div>)}
      </div></SetupFormSection>

      <SetupFormSection title="Opsi selang" description="Satu opsi berisi pasangan selang hisap dan selang buang yang diserahkan bersama."><div className="slotEditor fullField"><RepeatableEditorHead title="Pasangan selang hisap dan buang" count={packageValues.hose_options.length} addLabel="Tambah opsi selang" onAdd={addHoseOption} />
        {packageValues.hose_options.map((option, index) => <div className="equipmentRow" role="group" aria-label={`Opsi selang ${index + 1}`} key={index}>
          <div className="repeatableRowHeader"><strong>Opsi selang {index + 1}</strong><Button type="button" variant="ghost" size="sm" className="dangerIcon" aria-label={`Hapus opsi selang ${index + 1}`} onClick={() => setPackageValues({ ...packageValues, hose_options: packageValues.hose_options.filter((_, optionIndex) => optionIndex !== index) })}><Trash2 />Hapus</Button></div>
          <FormField label={`Kode selang ${index + 1}`} name={`hose_code_${index}`} required value={option.code} onChange={(e) => updateHoseOption(index, { code: e.target.value.toLowerCase() })} />
          <FormField label={`Merk selang hisap ${index + 1}`} name={`hose_suction_brand_${index}`} required value={option.suction_brand} onChange={(e) => updateHoseOption(index, { suction_brand: uppercaseBusinessText(e.target.value) })} />
          <FormField label={`Spesifikasi selang hisap ${index + 1}`} name={`hose_suction_spec_${index}`} required value={option.suction_spec} onChange={(e) => updateHoseOption(index, { suction_spec: uppercaseBusinessText(e.target.value) })} />
          <FormField label={`Merk selang buang ${index + 1}`} name={`hose_discharge_brand_${index}`} required value={option.discharge_brand} onChange={(e) => updateHoseOption(index, { discharge_brand: uppercaseBusinessText(e.target.value) })} />
          <FormField label={`Spesifikasi selang buang ${index + 1}`} name={`hose_discharge_spec_${index}`} required value={option.discharge_spec} onChange={(e) => updateHoseOption(index, { discharge_spec: uppercaseBusinessText(e.target.value) })} />
        </div>)}
      </div></SetupFormSection>

      <SetupFormSection title="Opsi konkit/reducer" description="Daftar merk konkit/reducer yang dapat dipilih pada alokasi."><div className="slotEditor fullField"><RepeatableEditorHead title="Merk konkit/reducer" count={packageValues.converter_options.length} addLabel="Tambah opsi konkit" onAdd={addConverterOption} />
        {packageValues.converter_options.map((option, index) => <div className="equipmentRow" role="group" aria-label={`Opsi konkit ${index + 1}`} key={index}>
          <div className="repeatableRowHeader"><strong>Opsi konkit {index + 1}</strong><Button type="button" variant="ghost" size="sm" className="dangerIcon" aria-label={`Hapus opsi konkit ${index + 1}`} onClick={() => setPackageValues({ ...packageValues, converter_options: packageValues.converter_options.filter((_, optionIndex) => optionIndex !== index) })}><Trash2 />Hapus</Button></div>
          <FormField label={`Kode konkit ${index + 1}`} name={`converter_code_${index}`} required value={option.code} onChange={(e) => updateConverterOption(index, { code: e.target.value.toLowerCase() })} />
          <FormField label={`Merk konkit ${index + 1}`} name={`converter_brand_${index}`} required value={option.brand} onChange={(e) => updateConverterOption(index, { brand: uppercaseBusinessText(e.target.value) })} />
        </div>)}
      </div></SetupFormSection>

      <SetupFormSection title="Komponen paket" description="Aksesoris dan kelengkapan lain beserta jumlah serta satuannya."><div className="slotEditor fullField"><RepeatableEditorHead title="Komponen, aksesoris, dan kelengkapan" count={packageValues.components.length} addLabel="Tambah komponen" onAdd={addComponent} />
        {packageValues.components.map((component, index) => <div className="componentRow" role="group" aria-label={`Komponen ${index + 1}`} key={index}>
          <div className="repeatableRowHeader"><strong>Komponen {index + 1}</strong><Button type="button" variant="ghost" size="sm" className="dangerIcon" aria-label={`Hapus komponen ${index + 1}`} onClick={() => setPackageValues({ ...packageValues, components: packageValues.components.filter((_, componentIndex) => componentIndex !== index) })}><Trash2 />Hapus</Button></div>
          <FormField label={`Kode komponen ${index + 1}`} name={`component_code_${index}`} required value={component.code} onChange={(e) => updateComponent(index, { code: e.target.value.toLowerCase() })} />
          <FormField label={`Nama komponen ${index + 1}`} name={`component_label_${index}`} required value={component.label} onChange={(e) => updateComponent(index, { label: uppercaseBusinessText(e.target.value) })} />
          <FormField label={`Jumlah komponen ${index + 1}`} name={`component_quantity_${index}`} type="number" min={1} required value={component.quantity} onChange={(e) => updateComponent(index, { quantity: Number(e.target.value) })} />
          <FormField label={`Satuan komponen ${index + 1}`} name={`component_unit_${index}`} required value={component.unit} onChange={(e) => updateComponent(index, { unit: uppercaseBusinessText(e.target.value) })} />
        </div>)}
      </div></SetupFormSection>
      {packageMutation.isError && <Alert className="sm:col-span-2" variant="destructive"><AlertDescription>Template paket belum dapat disimpan.</AlertDescription></Alert>}
    </SetupDialog>
    <SetupDialog layout="workspace" open={documentOpen} onOpenChange={setDocumentOpen} title={documentEditing ? `Edit ${documentEditing.name}` : 'Tambah template dokumentasi'} description="Perubahan pada versi terbit akan membuat versi draf baru." pending={documentMutation.isPending} dirty={JSON.stringify(documentValues) !== JSON.stringify(initialDocumentValues)} onSubmit={() => documentMutation.mutate()}>
      <SetupFormSection title="Identitas template" description="Kode, nama, jenis program, dan status versi dokumentasi.">
      <FormField label="Kode template" name="document_code" required readOnly={Boolean(documentEditing)} value={documentValues.template_code} onChange={(e) => setDocumentValues({ ...documentValues, template_code: e.target.value.toUpperCase() })} hint={documentEditing ? 'Kode template menjadi identitas versi dan tidak dapat diubah.' : 'Gunakan kode yang singkat dan unik.'} />
      <FormField label="Nama template" name="document_name" required value={documentValues.name} onChange={(e) => setDocumentValues({ ...documentValues, name: uppercaseBusinessText(e.target.value) })} />
      <div className="grid min-w-0 gap-2"><Label id="document-type-label">Jenis program</Label><Select value={documentValues.program_type} onValueChange={(value) => setDocumentValues({ ...documentValues, program_type: (value ?? 'farmer') as ProgramType })}><SelectTrigger className="w-full" aria-labelledby="document-type-label"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="farmer">Petani</SelectItem><SelectItem value="fisherman">Nelayan</SelectItem></SelectContent></Select></div>
      <div className="grid min-w-0 gap-2"><Label id="document-status-label">Status</Label><Select value={documentValues.status} onValueChange={(value) => setDocumentValues({ ...documentValues, status: value ?? 'draft' })}><SelectTrigger className="w-full" aria-labelledby="document-status-label"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="draft">Draf</SelectItem><SelectItem value="published">Terbit</SelectItem><SelectItem value="retired">Diarsipkan</SelectItem></SelectContent></Select></div>
      </SetupFormSection>
      <SetupFormSection title="Slot dokumentasi" description="Tentukan bukti foto atau video, sumber unggahan, dan jumlah minimal file."><div className="slotEditor fullField"><RepeatableEditorHead title="Daftar slot bukti" count={documentValues.slots.length} addLabel="Tambah slot" onAdd={addDocumentationSlot} />{documentValues.slots.map((slot, index) => <div className="slotRow" role="group" aria-label={`Slot bukti ${index + 1}`} key={index}><div className="repeatableRowHeader"><strong>Slot bukti {index + 1}</strong><Button type="button" variant="ghost" size="sm" className="dangerIcon" aria-label={`Hapus slot ${slot.label || index + 1}`} onClick={() => setDocumentValues({ ...documentValues, slots: documentValues.slots.filter((_, slotIndex) => slotIndex !== index) })}><Trash2 />Hapus</Button></div><FormField label="Kode slot" name={`slot_code_${index}`} required value={slot.slot_code} onChange={(e) => updateSlot(index, { slot_code: e.target.value.toLowerCase() })} /><FormField label="Judul" name={`slot_label_${index}`} required value={slot.label} onChange={(e) => updateSlot(index, { label: e.target.value })} /><div className="grid min-w-0 gap-2"><Label id={`slot-media-kind-${index}`}>Jenis media</Label><Select value={slot.media_kind} onValueChange={(value) => updateSlot(index, { media_kind: (value ?? 'image') as DocumentationSlot['media_kind'] })}><SelectTrigger className="w-full" aria-labelledby={`slot-media-kind-${index}`}><SelectValue /></SelectTrigger><SelectContent><SelectItem value="image">Foto saja</SelectItem><SelectItem value="video">Video saja</SelectItem><SelectItem value="image_video">Foto & video</SelectItem></SelectContent></Select></div><div className="grid min-w-0 gap-2"><Label id={`slot-source-${index}`}>Sumber</Label><Select value={slot.input_source} onValueChange={(value) => updateSlot(index, { input_source: (value ?? 'both') as DocumentationSlot['input_source'] })}><SelectTrigger className="w-full" aria-labelledby={`slot-source-${index}`}><SelectValue /></SelectTrigger><SelectContent><SelectItem value="both">Kamera & galeri</SelectItem><SelectItem value="camera">Kamera</SelectItem><SelectItem value="gallery">Galeri</SelectItem></SelectContent></Select></div><div className="grid min-w-0 gap-2"><Label id={`slot-stage-${index}`}>Pos</Label><Select value={slot.stage} onValueChange={(value) => updateSlot(index, { stage: (value ?? 'mesin') as DocumentationSlot['stage'] })}><SelectTrigger className="w-full" aria-labelledby={`slot-stage-${index}`}><SelectValue /></SelectTrigger><SelectContent><SelectItem value="mesin">POS Mesin</SelectItem><SelectItem value="dokumen">POS Dokumen</SelectItem><SelectItem value="penyerahan">POS Penyerahan</SelectItem></SelectContent></Select></div><FormField label="Minimal" name={`slot_min_${index}`} type="number" min={0} value={slot.min_files} onChange={(e) => updateSlot(index, { min_files: Number(e.target.value) })} /><label className="flex min-h-11 items-center gap-3"><Checkbox checked={slot.is_required} onCheckedChange={(checked) => updateSlot(index, { is_required: checked === true })} />Wajib</label></div>)}</div></SetupFormSection>
      {documentMutation.isError && <Alert className="sm:col-span-2" variant="destructive"><AlertDescription>Template dokumentasi belum dapat disimpan.</AlertDescription></Alert>}
    </SetupDialog>
  </div>;
}
