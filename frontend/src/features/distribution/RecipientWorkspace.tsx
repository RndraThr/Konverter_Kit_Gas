import { Dialog } from '@base-ui/react/dialog';
import { AlertTriangle, CheckCircle2, FileText, PackageCheck, Save, X } from 'lucide-react';
import { useMutation } from '@tanstack/react-query';
import { FormEvent, useEffect, useState } from 'react';
import { apiRequest } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { formatDate } from '../programs/types';
import type { DataResponse, DistributionRecord, DraftInput, RecipientWorkspaceData } from './types';
import { DocumentationSlot } from './DocumentationSlot';
import styles from './Distribution.module.css';

function draftFrom(data: RecipientWorkspaceData): DraftInput {
  return { nik: data.nik ?? '', sector_identifier: data.sector_identifier ?? '', address: data.address ?? '', village: data.village ?? '', district: data.district ?? '', phone_number: data.phone_number ?? '', identity_change_reason: '' };
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

  return <section className={styles.recipientWorkspace}>
    <header className={styles.recipientHeader}><div className={styles.distributionNumber}><small>Nomor pembagian</small><strong>{data.distribution_number}</strong></div><div><h2>{data.full_name}</h2><p>{data.program_name} / {data.regency_name}</p></div><span className={`${styles.eligibilityBanner} ${styles[data.eligibility]}`}>{data.eligibility === 'previously_received' ? <AlertTriangle /> : <CheckCircle2 />}{data.eligibility === 'previously_received' ? 'Penerimaan ulang diblokir' : 'Siap diverifikasi'}</span></header>
    {data.eligibility_reasons.length > 0 && <div className={styles.reasons}>{data.eligibility_reasons.map((reason) => <p key={reason}>{reason}</p>)}</div>}

    <div className={styles.workspaceColumns}>
      <form className={styles.identityForm} onSubmit={submit}>
        <div className={styles.sectionTitle}><h3>Verifikasi identitas</h3><p>Lengkapi bagian yang masih kosong berdasarkan dokumen penerima.</p></div>
        <div className={styles.fields}>
			<label><span>NIK {changed('nik', data.nik) && <small>Diubah</small>}</span><input value={draft.nik} disabled={!editable} onChange={(event) => update('nik', event.target.value)} /></label>
			<label><span>{data.program_type === 'farmer' ? 'Nomor kartu petani' : 'Nomor KUSUKA'} {changed('sector_identifier', data.sector_identifier) && <small>Diubah</small>}</span><input value={draft.sector_identifier} disabled={!editable} onChange={(event) => update('sector_identifier', event.target.value)} /></label>
			<label className={styles.fieldWide}><span>Alamat {changed('address', data.address) && <small>Diubah</small>}</span><input aria-label="Alamat" value={draft.address} disabled={!editable} onChange={(event) => update('address', event.target.value)} /></label>
			<label><span>Desa / kelurahan {changed('village', data.village) && <small>Diubah</small>}</span><input value={draft.village} disabled={!editable} onChange={(event) => update('village', event.target.value)} /></label>
			<label><span>Kecamatan {changed('district', data.district) && <small>Diubah</small>}</span><input value={draft.district} disabled={!editable} onChange={(event) => update('district', event.target.value)} /></label>
			<label><span>Nomor telepon {changed('phone_number', data.phone_number) && <small>Diubah</small>}</span><input aria-label="Nomor telepon" value={draft.phone_number} disabled={!editable} onChange={(event) => update('phone_number', event.target.value)} /></label>
			{draft.nik !== (data.nik ?? '') && <label className={styles.fieldWide}><span>Alasan perubahan NIK</span><input value={draft.identity_change_reason} disabled={!editable} onChange={(event) => update('identity_change_reason', event.target.value)} /></label>}
        </div>
		{editable && <div className={styles.formActions}>{saved && <span role="status">Draft penerima tersimpan.</span>}<button className="primaryButton" disabled={mutation.isPending} type="submit"><Save />{mutation.isPending ? 'Menyimpan...' : 'Simpan draft'}</button></div>}
      </form>

      <aside className={styles.sourcePanel}>
        <div className={styles.sectionTitle}><h3>Data sumber DCP3</h3><p>Nilai asli dari file dinas tetap dipertahankan.</p></div>
        <dl>{Object.entries(data.source_snapshot).map(([key, value]) => <div key={key}><dt>{key}</dt><dd>{String(value || '-')}</dd></div>)}</dl>
        {data.receipt_history.length > 0 && <details className={styles.history}><summary>Riwayat penerimaan ({data.receipt_history.length})</summary>{data.receipt_history.map((item) => <div key={`${item.completed_at}-${item.regency}`}><FileText aria-hidden="true" /><span><strong>{item.program}</strong><small>{item.regency} / {formatDate(item.completed_at)}</small>{item.bast_number && <code>{item.bast_number}</code>}</span></div>)}</details>}
      </aside>
    </div>
    <section className={styles.documentationSection}><div className={styles.sectionTitle}><h3>Dokumentasi pembagian</h3><p>Lengkapi setiap slot sesuai template jadwal.</p></div><div className={styles.documentationList}>{data.documentation.map((slot) => <DocumentationSlot key={slot.code} slot={slot} onChanged={(nextSlot) => onSaved({ ...data, documentation: data.documentation.map((item) => item.code === nextSlot.code ? nextSlot : item) })} />)}</div></section>
	{canManage && <footer className={styles.completion}><div><strong>{completed ? 'Distribusi selesai' : 'Konfirmasi distribusi'}</strong><span role={completed ? 'status' : undefined}>{completed ? 'Distribusi berhasil diselesaikan.' : blocked ? 'Selesaikan identitas dan seluruh dokumentasi sebelum konfirmasi.' : 'Semua pemeriksaan awal telah terpenuhi.'}</span></div>{!completed && <button className="primaryButton" disabled={blocked} onClick={() => setConfirmOpen(true)}>Selesaikan distribusi</button>}</footer>}
	<Dialog.Root open={confirmOpen} onOpenChange={setConfirmOpen}><Dialog.Portal><Dialog.Backdrop className="dialogBackdrop" /><Dialog.Popup className="dialogPopup" aria-label="Konfirmasi distribusi">
		<header className="dialogHeader"><div><Dialog.Title>Konfirmasi distribusi</Dialog.Title><Dialog.Description>Pastikan penerima dan bukti penyerahan sudah benar. Aksi ini tidak dapat dibatalkan dari halaman ini.</Dialog.Description></div><Dialog.Close className="iconButton" aria-label="Tutup"><X /></Dialog.Close></header>
		<div className="dialogBody"><div className={styles.confirmSummary}><PackageCheck aria-hidden="true" /><div><strong>{data.full_name}</strong><span>No. {data.distribution_number} / {data.regency_name}</span><small>{data.program_name}</small></div></div><div className={styles.confirmPackage}><strong>Paket efektif</strong>{packageEntries.length === 0 ? <span>Sesuai template paket jadwal</span> : packageEntries.map(([key, value]) => <span key={key}><small>{key.replaceAll('_', ' ')}</small>{String(value)}</span>)}</div><div className={styles.confirmSlots}><strong>Dokumentasi wajib</strong>{data.documentation.filter((slot) => slot.required).map((slot) => <span key={slot.code}><CheckCircle2 aria-hidden="true" />{slot.label}</span>)}</div></div>
		{completion.isError && <p className={styles.completionError} role="alert">{completion.error.message}</p>}
		<footer className="dialogActions"><Dialog.Close className="secondaryButton">Periksa lagi</Dialog.Close><button className="primaryButton" disabled={completion.isPending} onClick={() => completion.mutate()}>{completion.isPending ? 'Menyelesaikan...' : 'Konfirmasi penyerahan'}</button></footer>
	</Dialog.Popup></Dialog.Portal></Dialog.Root>
  </section>;
}
