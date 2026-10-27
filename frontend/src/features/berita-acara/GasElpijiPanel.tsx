import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Download, Eye, EyeOff, FileCheck2, Files, Fuel, Pencil, RefreshCw, Signature, Trash2 } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { toast } from 'sonner';
import { DataState } from '@/components/DataState';
import { FormField } from '@/components/FormField';
import { AlertDialog, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { apiBlobRequest, ApiError, apiRequest } from '@/lib/api';
import { useCan } from '@/lib/permissions';
import type { ProgramType } from '../programs/types';
import { PdfPreview } from './PdfPreview';
import { SignatureSettingsPanel } from './SignatureSettingsPanel';
import type { AggregateDocument, GasElpijiInput, GasElpijiReceipt, GasElpijiSummary, ScheduleSettings } from './types';
import { useWetSignatureStatus, WetSignatureBar } from './WetSignatureBar';
import { wetSignatureKey } from './wetScan';

type Props = { scheduleID: string; regencyName: string; programType: ProgramType; defaultDate: string };
type DataResponse<T> = { data: T };
type ReceiptForm = Omit<GasElpijiInput, 'schedule_id' | 'document_date' | 'cylinder_count'> & { cylinder_count: string };

const emptyForm: ReceiptForm = { warehouse_location: '', warehouse_address: '', pic: '', cylinder_count: '', remarks: '', receiver_name: '', pertamina_name: '' };

const textFields: { key: Exclude<keyof ReceiptForm, 'cylinder_count'>; label: string; required: boolean }[] = [
  { key: 'warehouse_location', label: 'Lokasi gudang', required: true },
  { key: 'warehouse_address', label: 'Alamat gudang', required: true },
  { key: 'pic', label: 'PIC', required: true },
];

const formatDate = (value: string) => new Intl.DateTimeFormat('id-ID', { day: 'numeric', month: 'long', year: 'numeric' }).format(new Date(`${value}T00:00:00`));
const formatCount = (value: number) => new Intl.NumberFormat('id-ID').format(value);

function formFromInput(input: GasElpijiInput): ReceiptForm {
  const { warehouse_location, warehouse_address, pic, remarks, receiver_name, pertamina_name, cylinder_count } = input;
  return { warehouse_location, warehouse_address, pic, remarks, receiver_name, pertamina_name, cylinder_count: cylinder_count > 0 ? String(cylinder_count) : '' };
}

const documentKey = 'gas-elpiji';

// GasElpijiPanel: LPG satu kabupaten biasanya datang beberapa kali. Tiap
// penerimaan difinalisasi per tanggal, lalu semuanya digabung menjadi satu BA
// Gas LPG (urut tanggal) yang disimpan ke 13. GAS ELPIJI.
export function GasElpijiPanel({ scheduleID, regencyName, programType, defaultDate }: Props) {
  const canManage = useCan('bast.manage');
  const queryClient = useQueryClient();
  const [documentDate, setDocumentDate] = useState(defaultDate);
  const [form, setForm] = useState<ReceiptForm>(emptyForm);
  const [showMerged, setShowMerged] = useState(false);
  const [cancelTarget, setCancelTarget] = useState<GasElpijiReceipt | null>(null);
  const [showSignatures, setShowSignatures] = useState(false);
  const loadedKey = useRef('');
  // edited: pengguna mengubah isian sejak dimuat; hanya isian yang diubah yang
  // disimpan sebagai draf (bukan isian kosong saat panel baru dimuat).
  const [edited, setEdited] = useState(false);

  const summary = useQuery({
    queryKey: ['bast', 'gas-elpiji', 'summary', scheduleID],
    queryFn: () => apiRequest<DataResponse<GasElpijiSummary>>(`/api/v1/bast/gas-elpiji/summary?schedule_id=${encodeURIComponent(scheduleID)}`),
    enabled: programType === 'farmer',
  });
  // Nama penanda tangan yang dikosongkan diambil dari Penandatangan jadwal.
  const scheduleSettings = useQuery({
    queryKey: ['bast', 'schedule-settings', scheduleID],
    queryFn: () => apiRequest<DataResponse<ScheduleSettings>>(`/api/v1/bast/schedules/${encodeURIComponent(scheduleID)}/settings`),
    enabled: programType === 'farmer',
  });
  const wetStatus = useWetSignatureStatus(scheduleID, documentKey, documentDate).data?.data;
  const wetEnabled = Boolean(wetStatus?.enabled);
  const receipts = summary.data?.data.receipts ?? [];
  const current = receipts.find((receipt) => receipt.document.document_date === documentDate);

  // Isi formulir dari draf tanggal terpilih, lalu dari penerimaan final tanggal
  // itu. Tanggal baru mempertahankan isian sebelumnya (gudang dan nama
  // biasanya sama), kecuali jumlah dan keterangan yang dikosongkan.
  useEffect(() => {
    if (!summary.data) return;
    const { receipts: loaded, drafts = [] } = summary.data.data;
    const match = loaded.find((receipt) => receipt.document.document_date === documentDate);
    const draft = drafts.find((item) => item.document_date === documentDate);
    const key = `${documentDate}:${match?.document.id ?? ''}`;
    if (loadedKey.current === key) return;
    const first = loadedKey.current === '';
    loadedKey.current = key;
    const source = draft ?? match?.input;
    setEdited(false);
    if (source) setForm(formFromInput(source));
    else if (first && loaded.length > 0) setForm({ ...formFromInput(loaded[loaded.length - 1].input), cylinder_count: '', remarks: '' });
    else setForm((value) => ({ ...value, cylinder_count: '', remarks: '' }));
  }, [summary.data, documentDate]);

  const input: GasElpijiInput = { ...form, schedule_id: scheduleID, document_date: documentDate, cylinder_count: Number(form.cylinder_count) || 0 };
  const [previewInput, setPreviewInput] = useState(input);
  const inputKey = JSON.stringify(input);
  useEffect(() => {
    const timer = setTimeout(() => setPreviewInput(JSON.parse(inputKey) as GasElpijiInput), 600);
    return () => clearTimeout(timer);
  }, [inputKey]);

  // Mode tanda tangan basah: halaman yang dicetak dirender dari isian tersimpan,
  // jadi isian yang berubah disimpan sebagai draf (setelah jeda mengetik).
  useEffect(() => {
    if (!canManage || !wetEnabled || !edited || previewInput.document_date === '' || JSON.stringify(previewInput) !== inputKey) return;
    apiRequest('/api/v1/bast/gas-elpiji/draft', { method: 'PUT', body: JSON.stringify(previewInput) })
      .then(() => queryClient.invalidateQueries({ queryKey: wetSignatureKey(scheduleID, documentKey, previewInput.document_date) }))
      .catch((error) => toast.error(`Draf belum tersimpan: ${errorMessage(error)}`));
  }, [canManage, wetEnabled, edited, previewInput, inputKey, queryClient, scheduleID]);

  const preview = useQuery({
    queryKey: ['bast', 'gas-elpiji', 'preview', previewInput],
    queryFn: () => apiBlobRequest('/api/v1/bast/gas-elpiji/preview', { method: 'POST', body: JSON.stringify(previewInput) }),
    enabled: programType === 'farmer' && previewInput.document_date !== '',
  });
  const mergedPreview = useQuery({
    queryKey: ['bast', 'gas-elpiji', 'merged-preview', scheduleID, receipts.map((receipt) => receipt.document.checksum)],
    queryFn: () => apiBlobRequest('/api/v1/bast/gas-elpiji/merged/preview', { method: 'POST', body: JSON.stringify({ schedule_id: scheduleID }) }),
    enabled: programType === 'farmer' && showMerged && receipts.length > 0,
  });
  const documents = useQuery({
    queryKey: ['bast', 'gas-elpiji', 'documents', scheduleID, documentDate],
    queryFn: () => apiRequest<DataResponse<AggregateDocument[]>>(`/api/v1/bast/gas-elpiji/documents?schedule_id=${encodeURIComponent(scheduleID)}&date=${documentDate}`),
    enabled: programType === 'farmer' && documentDate !== '',
  });

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['bast', 'gas-elpiji'] });
  const finalize = useMutation({
    mutationFn: () => apiRequest<DataResponse<AggregateDocument>>('/api/v1/bast/gas-elpiji/finalize', { method: 'POST', body: JSON.stringify(input) }),
    onSuccess: () => { invalidate(); toast.success(`Penerimaan LPG ${formatDate(documentDate)} berhasil difinalisasi.`); },
    onError: (error) => toast.error(errorMessage(error)),
  });
  const finalizeMerged = useMutation({
    mutationFn: () => apiRequest<DataResponse<AggregateDocument>>('/api/v1/bast/gas-elpiji/merged/finalize', { method: 'POST', body: JSON.stringify({ schedule_id: scheduleID }) }),
    onSuccess: () => { invalidate(); toast.success('BA Gas LPG gabungan berhasil difinalisasi dan disinkronkan.'); },
    onError: (error) => toast.error(errorMessage(error)),
  });
  const cancel = useMutation({
    mutationFn: (id: string) => apiRequest(`/api/v1/bast/gas-elpiji/documents/${encodeURIComponent(id)}/cancel`, { method: 'POST', body: '{}' }),
    onSuccess: () => { setCancelTarget(null); invalidate(); toast.success('Penerimaan dibatalkan.'); },
    onError: (error) => toast.error(errorMessage(error)),
  });

  if (programType === 'fisherman') return <DataState kind="empty" title="BA Gas Elpiji Nelayan belum tersedia" description="Fase pertama hanya mendukung program Petani." />;
  if (summary.isPending) return <DataState kind="loading" title="Memuat BA Gas Elpiji" description="Mengambil daftar penerimaan LPG." />;
  if (summary.isError) return <DataState kind="error" title="BA Gas Elpiji belum dapat dimuat" description="Periksa konfigurasi program atau koneksi, lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => summary.refetch() }} />;

  const { total_cylinders: totalCylinders, merged, merged_current: mergedCurrent } = summary.data.data;
  const defaults = scheduleSettings.data?.data;
  const receiverDefault = defaults?.installer_name?.trim() ?? '';
  const pertaminaDefault = defaults?.pertamina_rep_name?.trim() ?? '';
  const complete = form.warehouse_location.trim() !== '' && form.warehouse_address.trim() !== '' && form.pic.trim() !== '' && input.cylinder_count > 0
    && (form.receiver_name.trim() !== '' || receiverDefault !== '') && (form.pertamina_name.trim() !== '' || pertaminaDefault !== '');
  const wetBlocked = wetEnabled && !wetStatus?.scan?.valid;
  const unsignedReceipts = wetEnabled ? receipts.filter((receipt) => !receipt.wet_scanned).length : 0;
  const versions = documents.data?.data ?? [];
  const setField = (key: keyof ReceiptForm) => (event: { target: { value: string } }) => {
    setEdited(true);
    setForm((value) => ({ ...value, [key]: event.target.value }));
  };

  return <div className="grid gap-5">
    {showSignatures && <SignatureSettingsPanel scheduleID={scheduleID} date={documentDate} document={documentKey} documentLabel="Gas LPG" />}

    <div className="grid gap-5 xl:grid-cols-[24rem_minmax(0,1fr)]">
      <Card className="self-start">
        <CardHeader className="border-b"><CardTitle className="flex items-center gap-2 text-base"><Fuel className="size-4 text-primary" aria-hidden="true" />Penerimaan LPG</CardTitle></CardHeader>
        <CardContent className="grid gap-4 pt-5">
          <FormField label="Tanggal penerimaan" name="gas_document_date" type="date" required value={documentDate} onChange={(e) => setDocumentDate(e.target.value)}
            hint={current ? `Sudah final (versi ${current.document.version}). Finalisasi ulang membuat versi baru.` : 'Belum ada penerimaan pada tanggal ini.'} />
          {textFields.map((field) => <FormField key={field.key} label={field.label} name={`gas_${field.key}`} required={field.required} maxLength={300} value={form[field.key]} disabled={!canManage} onChange={setField(field.key)} />)}
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-1">
            <FormField label="Jumlah tabung" name="gas_cylinder_count" type="number" inputMode="numeric" min={1} required value={form.cylinder_count} disabled={!canManage} onChange={setField('cylinder_count')} />
            <FormField label="Keterangan" name="gas_remarks" maxLength={300} value={form.remarks} disabled={!canManage} onChange={setField('remarks')} />
          </div>
          <FormField label="Nama penerima (PT pelaksana)" name="gas_receiver_name" maxLength={300} value={form.receiver_name} disabled={!canManage} onChange={setField('receiver_name')}
            placeholder={receiverDefault || undefined} hint={receiverDefault ? 'Kosongkan untuk memakai nama Pelaksana di Penandatangan.' : 'Wajib diisi, atau atur nama Pelaksana di Penandatangan.'} />
          <FormField label="Nama petugas PT Pertamina Patra Niaga" name="gas_pertamina_name" maxLength={300} value={form.pertamina_name} disabled={!canManage} onChange={setField('pertamina_name')}
            placeholder={pertaminaDefault || undefined} hint={pertaminaDefault ? 'Kosongkan untuk memakai nama Pertamina di Penandatangan.' : 'Wajib diisi, atau atur nama Pertamina di Penandatangan.'} />
          {!complete && <p className="text-xs text-muted-foreground">Lengkapi lokasi gudang, alamat, PIC, jumlah tabung, dan nama kedua penanda tangan sebelum finalisasi. Keterangan boleh kosong.</p>}
          <div className="grid gap-2 border-t pt-4">
            <Button variant="outline" onClick={() => setShowSignatures((value) => !value)}><Signature />{showSignatures ? 'Tutup penandatangan' : 'Penandatangan'}</Button>
            {canManage && <Button disabled={!complete || documentDate === '' || finalize.isPending || wetBlocked} onClick={() => finalize.mutate()}><RefreshCw className={finalize.isPending ? 'animate-spin' : ''} />{finalize.isPending ? 'Menyinkronkan...' : 'Finalisasi penerimaan'}</Button>}
          </div>
        </CardContent>
      </Card>

      <Card className="min-w-0">
        <CardHeader className="border-b"><div className="flex flex-wrap items-center justify-between gap-3"><div><CardTitle className="text-base">BA Gas Elpiji · {documentDate ? formatDate(documentDate) : 'pilih tanggal'}</CardTitle><p className="mt-1 text-sm text-muted-foreground">{regencyName} · disimpan ke 13. GAS ELPIJI</p></div>{current && <Badge variant="outline">Final v{current.document.version}</Badge>}</div></CardHeader>
        <CardContent className="grid gap-4 pt-5">
          {documentDate !== '' && <WetSignatureBar scheduleID={scheduleID} document={documentKey} date={documentDate} label="BA Gas Elpiji" />}
          {versions.length > 1 && <div className="grid gap-2 rounded-lg border p-3">
            <strong className="text-sm">Riwayat versi tanggal ini</strong>
            {versions.filter((doc) => doc.status !== 'active').map((doc) => <div className="flex items-center justify-between gap-3 text-sm" key={doc.id}><span className="text-muted-foreground">Versi {doc.version} · {formatCount(doc.recipient_count)} tabung</span><Button nativeButton={false} render={<a href={`/api/v1/bast/gas-elpiji/documents/${doc.id}/content`} />} variant="ghost" size="sm"><Download />Unduh</Button></div>)}
          </div>}
          {documentDate !== '' && <PdfPreview blob={preview.data} isPending={preview.isPending} isError={preview.isError} label="BA Gas Elpiji" />}
        </CardContent>
      </Card>
    </div>

    <Card>
      <CardHeader className="border-b"><div className="flex flex-wrap items-center justify-between gap-3"><div><CardTitle className="flex items-center gap-2 text-base"><Files className="size-4 text-primary" aria-hidden="true" />Daftar penerimaan & BA gabungan</CardTitle><p className="mt-1 text-sm text-muted-foreground">Semua penerimaan final digabung urut tanggal menjadi satu BA Gas LPG.</p></div><Badge variant="outline">{receipts.length} penerimaan · {formatCount(totalCylinders)} tabung</Badge></div></CardHeader>
      <CardContent className="grid gap-4 pt-5">
        {receipts.length === 0
          ? <p className="rounded-md bg-muted px-3 py-2 text-sm text-muted-foreground">Belum ada penerimaan yang difinalisasi.</p>
          : <ul className="grid gap-2" aria-label="Penerimaan LPG">
            {receipts.map((receipt) => <li key={receipt.document.id} className="grid gap-2 rounded-lg border p-3 data-active:border-primary sm:grid-cols-[1fr_auto] sm:items-center" data-active={receipt.document.document_date === documentDate || undefined}>
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2"><FileCheck2 className="size-4 text-emerald-600" aria-hidden="true" /><strong className="text-sm">{formatDate(receipt.document.document_date)}</strong><span className="text-sm text-muted-foreground">· {formatCount(receipt.input.cylinder_count)} tabung · v{receipt.document.version}</span>{receipt.wet_scanned && <Badge variant="secondary">TTD basah</Badge>}</div>
                <p className="mt-1 truncate text-xs text-muted-foreground">{receipt.input.warehouse_location}</p>
              </div>
              <div className="flex flex-wrap gap-1">
                <Button variant="ghost" size="sm" onClick={() => setDocumentDate(receipt.document.document_date)}><Pencil />Buka</Button>
                <Button nativeButton={false} render={<a href={`/api/v1/bast/gas-elpiji/documents/${receipt.document.id}/content`} />} variant="ghost" size="sm"><Download />Unduh</Button>
                {canManage && <Button variant="ghost" size="sm" className="text-destructive" onClick={() => setCancelTarget(receipt)}><Trash2 />Batalkan</Button>}
              </div>
            </li>)}
          </ul>}

        <div className="grid gap-3 rounded-lg border p-4 sm:grid-cols-[1fr_auto] sm:items-center">
          <div>
            <div className="flex flex-wrap items-center gap-2"><strong className="text-sm">BA Gas LPG gabungan</strong>
              {!merged ? <Badge variant="outline">Belum dibuat</Badge> : mergedCurrent ? <Badge>Terbaru</Badge> : <Badge variant="destructive">Perlu digabung ulang</Badge>}</div>
            <p className="mt-1 text-xs text-muted-foreground">{merged ? `${merged.filename} · ${merged.page_count} halaman · ${formatCount(merged.recipient_count)} tabung` : 'Gabungkan setelah semua penerimaan kabupaten ini final.'}</p>
            {unsignedReceipts > 0 && <p className="mt-1 text-xs text-amber-700 dark:text-amber-300">Mode tanda tangan basah aktif: {unsignedReceipts} penerimaan belum memakai halaman bertanda tangan basah. Unggah pindaiannya lalu finalisasi ulang penerimaan itu sebelum menggabungkan.</p>}
          </div>
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" disabled={receipts.length === 0} onClick={() => setShowMerged((value) => !value)}>{showMerged ? <EyeOff /> : <Eye />}{showMerged ? 'Tutup preview' : 'Preview gabungan'}</Button>
            {merged && <Button nativeButton={false} render={<a href={`/api/v1/bast/gas-elpiji/documents/${merged.id}/content`} />} variant="outline"><Download />Unduh gabungan</Button>}
            {canManage && <Button disabled={receipts.length === 0 || mergedCurrent || unsignedReceipts > 0 || finalizeMerged.isPending} onClick={() => finalizeMerged.mutate()}><RefreshCw className={finalizeMerged.isPending ? 'animate-spin' : ''} />{finalizeMerged.isPending ? 'Menggabungkan...' : 'Gabungkan & finalisasi'}</Button>}
          </div>
        </div>
        {showMerged && receipts.length > 0 && <PdfPreview blob={mergedPreview.data} isPending={mergedPreview.isPending} isError={mergedPreview.isError} label="BA Gas LPG gabungan" />}
      </CardContent>
    </Card>

    <AlertDialog open={cancelTarget !== null} onOpenChange={(open) => !open && setCancelTarget(null)}><AlertDialogContent aria-label="Batalkan penerimaan">
      <AlertDialogHeader><AlertDialogTitle>Batalkan penerimaan?</AlertDialogTitle><AlertDialogDescription>Penerimaan {cancelTarget ? formatDate(cancelTarget.document.document_date) : ''} akan dihapus dari daftar dan berkasnya dihapus dari Drive. BA gabungan perlu digabung ulang.</AlertDialogDescription></AlertDialogHeader>
      <AlertDialogFooter><AlertDialogCancel>Kembali</AlertDialogCancel><Button variant="destructive" disabled={cancel.isPending} onClick={() => cancelTarget && cancel.mutate(cancelTarget.document.id)}>{cancel.isPending ? 'Membatalkan...' : 'Batalkan penerimaan'}</Button></AlertDialogFooter>
    </AlertDialogContent></AlertDialog>
  </div>;
}

function errorMessage(error: unknown) {
  return error instanceof ApiError || error instanceof Error ? error.message : 'Permintaan belum dapat diproses.';
}
