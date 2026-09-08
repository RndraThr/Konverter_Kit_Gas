import { AlertTriangle, CheckCircle2, FileText, PackageCheck, Save } from 'lucide-react';
import { useMutation } from '@tanstack/react-query';
import { FormEvent, useEffect, useState } from 'react';
import { apiRequest } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { formatDate } from '../programs/types';
import type { DataResponse, DistributionRecord, DraftInput, EquipmentOption, RecipientWorkspaceData } from './types';
import { DocumentationSlot } from './DocumentationSlot';
import styles from './Distribution.module.css';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { AlertDialog, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog';

function draftFrom(data: RecipientWorkspaceData): DraftInput {
  return {
    nik: data.nik ?? '', sector_identifier: data.sector_identifier ?? '', address: data.address ?? '', village: data.village ?? '', district: data.district ?? '', phone_number: data.phone_number ?? '', identity_change_reason: '',
    machine_option_code: data.machine_option_code ?? '', machine_serial_number: data.machine_serial_number ?? '',
    hose_option_code: data.hose_option_code ?? '', hose_serial_number: data.hose_serial_number ?? '',
    converter_serial_number: data.converter_serial_number ?? '',
  };
}

export function RecipientWorkspace({ data, onSaved }: { data: RecipientWorkspaceData; onSaved: (data: RecipientWorkspaceData) => void }) {
  const canManage = useCan('distribution.manage');
	const editable = canManage && data.distribution_status !== 'completed';
  const [draft, setDraft] = useState(() => draftFrom(data));
  const [saved, setSaved] = useState(false);
	const [confirmOpen, setConfirmOpen] = useState(false);
	const [completed, setCompleted] = useState(data.distribution_status === 'completed');
	useEffect(() => { setDraft(draftFrom(data)); setSaved(false); setCompleted(data.distribution_status === 'completed'); }, [data.allocation_id, data.distribution_status]);
  const mutation = useMutation({
    mutationFn: () => apiRequest<DataResponse<RecipientWorkspaceData>>(`/api/v1/distribution/allocations/${data.allocation_id}/draft`, { method: 'PATCH', body: JSON.stringify(draft) }),
    onSuccess: ({ data: next }) => { setDraft(draftFrom(next)); onSaved(next); setSaved(true); },
  });
	const completion = useMutation({
		mutationFn: () => apiRequest<DataResponse<DistributionRecord>>(`/api/v1/distribution/allocations/${data.allocation_id}/complete`, { method: 'POST' }),
		onSuccess: () => {
			setConfirmOpen(false);
			setCompleted(true);
			onSaved({ ...data, allocation_status: 'distributed', distribution_status: 'completed' });
		},
	});
  const update = (key: keyof DraftInput, value: string) => { setSaved(false); setDraft((current) => ({ ...current, [key]: value })); };
  const changed = (key: keyof DraftInput, original: string | undefined) => draft[key] !== (original ?? '');
  const submit = (event: FormEvent) => { event.preventDefault(); mutation.mutate(); };
	const identityIncomplete = !data.full_name.trim() || !/^\d{16}$/.test(draft.nik) || !draft.sector_identifier.trim();
  const blocked = identityIncomplete || data.eligibility !== 'eligible' || data.documentation.some((slot) => slot.required && slot.status !== 'complete');
	const packageEntries = Object.entries(data.package_snapshot ?? {}).filter(([, value]) => ['string', 'number'].includes(typeof value)).slice(0, 4);
	const machineOptions = (data.package_snapshot?.machine_options as EquipmentOption[] | undefined) ?? [];
	const hoseOptions = (data.package_snapshot?.hose_options as EquipmentOption[] | undefined) ?? [];

  return <section className={styles.recipientWorkspace} aria-label="Ruang kerja penerima">
    <header className={styles.recipientHeader}><div className={styles.distributionNumber}><small>Nomor pembagian</small><strong>{data.distribution_number}</strong></div><div><h2>{data.full_name}</h2><p>{data.program_name} / {data.regency_name}</p></div><span className={`${styles.eligibilityBanner} ${styles[data.eligibility]}`}>{data.eligibility === 'previously_received' ? <AlertTriangle /> : <CheckCircle2 />}{data.eligibility === 'previously_received' ? 'Penerimaan ulang diblokir' : 'Siap diverifikasi'}</span></header>
    {data.eligibility_reasons.length > 0 && <div className={styles.reasons} role="alert">{data.eligibility_reasons.map((reason) => <p key={reason}>{reason}</p>)}</div>}

    <div className={styles.workspaceColumns}>
      <section className={styles.verificationSection} aria-labelledby="verification-title">
      <form className={styles.identityForm} onSubmit={submit}>
        <div className={styles.sectionTitle}><Badge variant="outline">Langkah 1</Badge><h3 id="verification-title">Verifikasi penerima</h3><p>Lengkapi bagian yang masih kosong berdasarkan dokumen penerima.</p></div>
        <div className={styles.formGroup}><h4>Identitas penerima</h4><div className={styles.fields}>
			<label><span>NIK {changed('nik', data.nik) && <small>Diubah</small>}</span><input value={draft.nik} disabled={!editable} onChange={(event) => update('nik', event.target.value)} /></label>
			<label><span>{data.program_type === 'farmer' ? 'Nomor kartu petani' : 'Nomor KUSUKA'} {changed('sector_identifier', data.sector_identifier) && <small>Diubah</small>}</span><input value={draft.sector_identifier} disabled={!editable} onChange={(event) => update('sector_identifier', event.target.value)} /></label>
			<label className={styles.fieldWide}><span>Alamat {changed('address', data.address) && <small>Diubah</small>}</span><input aria-label="Alamat" value={draft.address} disabled={!editable} onChange={(event) => update('address', event.target.value)} /></label>
			<label><span>Desa / kelurahan {changed('village', data.village) && <small>Diubah</small>}</span><input value={draft.village} disabled={!editable} onChange={(event) => update('village', event.target.value)} /></label>
			<label><span>Kecamatan {changed('district', data.district) && <small>Diubah</small>}</span><input value={draft.district} disabled={!editable} onChange={(event) => update('district', event.target.value)} /></label>
			<label><span>Nomor telepon {changed('phone_number', data.phone_number) && <small>Diubah</small>}</span><input aria-label="Nomor telepon" value={draft.phone_number} disabled={!editable} onChange={(event) => update('phone_number', event.target.value)} /></label>
			{draft.nik !== (data.nik ?? '') && <label className={styles.fieldWide}><span>Alasan perubahan NIK</span><input value={draft.identity_change_reason} disabled={!editable} onChange={(event) => update('identity_change_reason', event.target.value)} /></label>}
        </div></div>
        <div className={styles.formGroup}><h4>Perlengkapan yang diserahkan</h4><div className={styles.fields}>
			<label><span>Merk/Tipe Mesin</span><select aria-label="Merk/Tipe Mesin" value={draft.machine_option_code} disabled={!editable} onChange={(event) => update('machine_option_code', event.target.value)}><option value="">Pilih mesin</option>{machineOptions.map((option) => <option key={option.code} value={option.code}>{option.brand} {option.type}</option>)}</select></label>
			<label><span>Serial Number Mesin</span><input aria-label="Serial Number Mesin" value={draft.machine_serial_number} disabled={!editable} onChange={(event) => update('machine_serial_number', event.target.value)} /></label>
			<label><span>Merk/Spesifikasi Selang</span><select aria-label="Merk/Spesifikasi Selang" value={draft.hose_option_code} disabled={!editable} onChange={(event) => update('hose_option_code', event.target.value)}><option value="">Pilih selang</option>{hoseOptions.map((option) => <option key={option.code} value={option.code}>{option.brand} {option.spec}</option>)}</select></label>
			<label><span>Serial Number Selang</span><input aria-label="Serial Number Selang" value={draft.hose_serial_number} disabled={!editable} onChange={(event) => update('hose_serial_number', event.target.value)} /></label>
			<label><span>Serial Number Konkit/Reducer</span><input aria-label="Serial Number Konkit/Reducer" value={draft.converter_serial_number} disabled={!editable} onChange={(event) => update('converter_serial_number', event.target.value)} /></label>
        </div></div>
		{editable && <div className={styles.formActions}>{saved && <span role="status">Draft penerima tersimpan.</span>}<Button disabled={mutation.isPending} type="submit"><Save />{mutation.isPending ? 'Menyimpan...' : 'Simpan draft'}</Button></div>}
      </form>
      </section>

      <aside className={styles.sourcePanel} aria-labelledby="source-data-title">
        <div className={styles.sectionTitle}><Badge variant="outline">Referensi</Badge><h3 id="source-data-title">Data sumber DCP3</h3><p>Nilai asli dari file dinas tetap dipertahankan.</p></div>
        <dl>{Object.entries(data.source_snapshot).map(([key, value]) => <div key={key}><dt>{key}</dt><dd>{String(value || '-')}</dd></div>)}</dl>
        {data.receipt_history.length > 0 && <details className={styles.history}><summary>Riwayat penerimaan ({data.receipt_history.length})</summary>{data.receipt_history.map((item) => <div key={`${item.completed_at}-${item.regency}`}><FileText aria-hidden="true" /><span><strong>{item.program}</strong><small>{item.regency} / {formatDate(item.completed_at)}</small>{item.bast_number && <code>{item.bast_number}</code>}</span></div>)}</details>}
      </aside>
    </div>
    <section className={styles.documentationSection} aria-labelledby="documentation-title"><div className={styles.sectionTitle}><Badge variant="outline">Langkah 2</Badge><h3 id="documentation-title">Dokumentasi penyerahan</h3><p>Lengkapi setiap slot sesuai template jadwal.</p></div><div className={styles.documentationList}>{data.documentation.map((slot) => <DocumentationSlot key={slot.code} slot={slot} onChanged={(nextSlot) => onSaved({ ...data, documentation: data.documentation.map((item) => item.code === nextSlot.code ? nextSlot : item) })} />)}</div></section>
	{canManage && <footer className={styles.completion}><div><strong>{completed ? 'Distribusi selesai' : 'Konfirmasi distribusi'}</strong><span role={completed ? 'status' : undefined}>{completed ? 'Distribusi berhasil diselesaikan.' : blocked ? 'Selesaikan identitas dan seluruh dokumentasi sebelum konfirmasi.' : 'Semua pemeriksaan awal telah terpenuhi.'}</span></div>{!completed && <Button disabled={blocked} onClick={() => setConfirmOpen(true)}>Selesaikan distribusi</Button>}</footer>}
	<AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}><AlertDialogContent aria-label="Konfirmasi distribusi">
		<AlertDialogHeader><AlertDialogTitle>Konfirmasi distribusi</AlertDialogTitle><AlertDialogDescription>Pastikan penerima dan bukti penyerahan sudah benar. Aksi ini tidak dapat dibatalkan dari halaman ini.</AlertDialogDescription></AlertDialogHeader>
		<div className={styles.confirmSummary}><PackageCheck aria-hidden="true" /><div><strong>{data.full_name}</strong><span>No. {data.distribution_number} / {data.regency_name}</span><small>{data.program_name}</small></div></div><div className={styles.confirmPackage}><strong>Paket efektif</strong>{packageEntries.length === 0 ? <span>Sesuai template paket jadwal</span> : packageEntries.map(([key, value]) => <span key={key}><small>{key.replaceAll('_', ' ')}</small>{String(value)}</span>)}</div><div className={styles.confirmSlots}><strong>Dokumentasi wajib</strong>{data.documentation.filter((slot) => slot.required).map((slot) => <span key={slot.code}><CheckCircle2 aria-hidden="true" />{slot.label}</span>)}</div>
		{completion.isError && <p className={styles.completionError} role="alert">{completion.error.message}</p>}
		<AlertDialogFooter><AlertDialogCancel>Periksa lagi</AlertDialogCancel><Button disabled={completion.isPending} onClick={() => completion.mutate()}>{completion.isPending ? 'Menyelesaikan...' : 'Konfirmasi penyerahan'}</Button></AlertDialogFooter>
	</AlertDialogContent></AlertDialog>
  </section>;
}
