import { Plus, Save, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { FormField } from '@/components/FormField';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { templateEntryLabel } from './TemplateEntrySelect';
import type { PemeriksaanChecklist, PemeriksaanForm, TemplateEntry } from './types';

type Props = { forms: PemeriksaanForm[]; entries: TemplateEntry[]; canManage: boolean; saving: boolean; onChange: (forms: PemeriksaanForm[]) => void; onSave: () => void };

type ChoiceKey = Exclude<keyof PemeriksaanChecklist, 'documents' | 'other_document'>;

const singleChoices: { key: ChoiceKey; label: string; options: [string, string][] }[] = [
  { key: 'packaging', label: 'Kemasan', options: [['baik', 'Baik'], ['rusak', 'Rusak']] },
  { key: 'quantity', label: 'Jumlah Barang', options: [['lengkap', 'Lengkap'], ['kurang', 'Kurang']] },
  { key: 'specification', label: 'Jenis Dan Spesifikasi', options: [['sesuai', 'Sesuai'], ['tidak_sesuai', 'Tidak Sesuai']] },
  { key: 'condition', label: 'Kondisi Barang', options: [['baru_baik', 'Baru & Baik'], ['tidak_baik', 'Tidak Baik'], ['bekas', 'Bekas']] },
  { key: 'function_test', label: 'Dilakukan Uji Fungsi', options: [['ya', 'Ya'], ['tidak', 'Tidak']] },
  { key: 'conclusion', label: 'Kesimpulan', options: [['diterima', 'Diterima'], ['tidak_diterima', 'Tidak Di Terima']] },
];

const documentChoices: [string, string][] = [['sertifikat', 'Sertifikat'], ['manual_book', 'Manual Book'], ['garansi', 'Garansi'], ['lainnya', 'Dokumen Lainnya']];

export const pemeriksaanFormsValid = (forms: PemeriksaanForm[]) => {
  const codes = new Set<string>();
  return forms.length > 0 && forms.every((form) => {
    if (!/^[a-z0-9_]{1,40}$/.test(form.code) || codes.has(form.code) || form.title.trim() === '') return false;
    codes.add(form.code);
    return true;
  });
};

const slug = (value: string) => value.toLowerCase().normalize('NFKD').replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '').slice(0, 40);

/** Editor daftar form BA Pemeriksaan per program: pilih form di kiri, ubah isinya di kanan. */
export function PemeriksaanFormsEditor({ forms, entries, canManage, saving, onChange, onSave }: Props) {
  const [selected, setSelected] = useState(0);
  const index = Math.min(selected, forms.length - 1);
  const form = forms[index];
  const update = (patch: Partial<PemeriksaanForm>) => onChange(forms.map((item, i) => (i === index ? { ...item, ...patch } : item)));
  // Baris disimpan sebagai referensi barang template; urutan cetak mengikuti template.
  const toggleEntry = (ref: string, checked: boolean) => update({ rows: checked ? [...form.rows, { ref, description: '', unit: '', quantity_per_package: 0, notes: '' }] : form.rows.filter((row) => row.ref !== ref) });
  const updateChecklist = (patch: Partial<PemeriksaanChecklist>) => update({ checklist: { ...form.checklist, ...patch } });
  const addForm = () => {
    const base = 'form_baru';
    let code = base;
    for (let n = 2; forms.some((item) => item.code === code); n++) code = `${base}_${n}`;
    onChange([...forms, { code, title: 'Form Baru', rows: [], note: 'Barang diterima dengan baik dan sesuai spesifikasi dalam PO', checklist: { packaging: 'baik', quantity: 'lengkap', specification: 'sesuai', condition: 'baru_baik', documents: [], other_document: '', function_test: 'ya', conclusion: 'diterima' } }]);
    setSelected(forms.length);
  };

  return <Card>
    <CardHeader className="border-b"><CardTitle className="text-base">Daftar form BA Pemeriksaan program</CardTitle><p className="text-sm text-muted-foreground">Berlaku untuk semua kabupaten pada program ini. Satu form = satu PDF. Setiap baris menunjuk barang Template Paket; merk dan jumlah mengikuti template jadwal, No. PO dari tab Zona.</p></CardHeader>
    <CardContent className="grid gap-5 pt-5 lg:grid-cols-[14rem_minmax(0,1fr)]">
      <nav aria-label="Form BA Pemeriksaan" className="grid content-start gap-1">
        {forms.map((item, i) => <Button key={item.code + i} variant={i === index ? 'secondary' : 'ghost'} className="justify-start" onClick={() => setSelected(i)}>{item.title || 'Tanpa judul'}</Button>)}
        {canManage && <Button variant="outline" className="mt-2 justify-start" onClick={addForm}><Plus />Tambah form</Button>}
      </nav>

      {form && <div className="grid min-w-0 gap-5">
        <div className="grid gap-1.5">
          <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_14rem_auto] sm:items-end">
            <FormField label="Judul form" name="pemeriksaan_form_title" value={form.title} disabled={!canManage} onChange={(e) => update({ title: e.target.value })} />
            <FormField label="Kode (file & dokumen)" name="pemeriksaan_form_code" value={form.code} disabled={!canManage} placeholder="huruf_kecil_angka" onChange={(e) => update({ code: slug(e.target.value) })} />
            {canManage && forms.length > 1 && <Button variant="ghost" className="text-destructive hover:text-destructive" onClick={() => { onChange(forms.filter((_, i) => i !== index)); setSelected(0); }}><Trash2 />Hapus form</Button>}
          </div>
          <p className="text-xs text-muted-foreground">Kode dipakai untuk nama file dan jenis dokumen: huruf kecil, angka, dan garis bawah.</p>
        </div>

        <fieldset className="grid gap-2">
          <legend className="mb-1"><strong className="text-sm">A. Berdasarkan</strong><span className="block text-xs text-muted-foreground">Centang barang Template Paket yang diperiksa di form ini. Deskripsi, unit, dan jumlah otomatis dari template; keterangan dari Dokumen Pendukung.</span></legend>
          {entries.length === 0 ? <p className="text-xs text-muted-foreground">Template jadwal belum punya barang.</p>
            : <div className="grid gap-1.5 sm:grid-cols-2">{entries.map((entry) => {
              const checked = form.rows.some((row) => row.ref === entry.ref);
              return <label key={entry.ref} className={`flex min-h-10 items-center gap-2 rounded-md border px-3 text-sm ${checked ? 'border-primary bg-primary/5' : ''}`}>
                <Checkbox disabled={!canManage} checked={checked} onCheckedChange={(value) => toggleEntry(entry.ref, value === true)} />
                <span className="min-w-0 truncate">{templateEntryLabel(entry, entry.ref)}</span>
              </label>;
            })}</div>}
          {form.rows.filter((row) => !entries.some((entry) => entry.ref === row.ref)).map((row) => <p key={row.ref} className="text-xs text-amber-700 dark:text-amber-300">{templateEntryLabel(undefined, row.ref)} <button type="button" className="underline" disabled={!canManage} onClick={() => toggleEntry(row.ref, false)}>hapus</button></p>)}
        </fieldset>

        <section className="grid gap-3" aria-label="B. Hasil Pemeriksaan Barang">
          <strong className="text-sm">B. Hasil Pemeriksaan Barang (tercentang default)</strong>
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
            {singleChoices.map((choice) => <div key={choice.key} className="grid gap-1.5">
              <Label id={`checklist-${choice.key}`}>{choice.label}</Label>
              <Select disabled={!canManage} value={form.checklist[choice.key]} onValueChange={(value) => updateChecklist({ [choice.key]: value ?? '' })}>
                <SelectTrigger className="w-full" aria-labelledby={`checklist-${choice.key}`}><SelectValue placeholder="Tidak dicentang" /></SelectTrigger>
                <SelectContent>{choice.options.map(([value, label]) => <SelectItem key={value} value={value}>{label}</SelectItem>)}</SelectContent>
              </Select>
            </div>)}
          </div>
          <fieldset className="grid gap-2"><legend className="mb-1 text-sm font-medium">Dokumen Pendukung</legend>
            <div className="flex flex-wrap gap-4">{documentChoices.map(([value, label]) => <label key={value} className="flex min-h-11 items-center gap-2 text-sm">
              <Checkbox disabled={!canManage} checked={form.checklist.documents.includes(value)} onCheckedChange={(checked) => updateChecklist({ documents: checked ? [...form.checklist.documents, value] : form.checklist.documents.filter((item) => item !== value) })} />{label}
            </label>)}</div>
            {form.checklist.documents.includes('lainnya') && <Input aria-label="Dokumen lainnya" className="max-w-md" value={form.checklist.other_document} disabled={!canManage} onChange={(e) => updateChecklist({ other_document: e.target.value })} placeholder="Nama dokumen lainnya" />}
          </fieldset>
          <FormField label="Catatan" name="pemeriksaan_note" value={form.note} disabled={!canManage} onChange={(e) => update({ note: e.target.value })} />
        </section>

        {canManage && <div className="flex justify-end"><Button disabled={!pemeriksaanFormsValid(forms) || saving} onClick={onSave}><Save />{saving ? 'Menyimpan...' : 'Simpan daftar form'}</Button></div>}
        {!pemeriksaanFormsValid(forms) && <p className="text-xs text-muted-foreground">Setiap form wajib punya judul dan kode unik.</p>}
      </div>}
    </CardContent>
  </Card>;
}
