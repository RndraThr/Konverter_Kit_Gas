import { AlertTriangle, CheckCircle2, FileText, Save } from 'lucide-react';
import { useMutation } from '@tanstack/react-query';
import { FormEvent, useEffect, useState } from 'react';
import { apiRequest } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { formatDate } from '../programs/types';
import type { DataResponse, DraftInput, RecipientWorkspaceData } from './types';
import styles from './Distribution.module.css';

function draftFrom(data: RecipientWorkspaceData): DraftInput {
  return { nik: data.nik ?? '', sector_identifier: data.sector_identifier ?? '', address: data.address ?? '', village: data.village ?? '', district: data.district ?? '', phone_number: data.phone_number ?? '', identity_change_reason: '' };
}

export function RecipientWorkspace({ data, onSaved }: { data: RecipientWorkspaceData; onSaved: (data: RecipientWorkspaceData) => void }) {
  const canManage = useCan('distribution.manage');
  const [draft, setDraft] = useState(() => draftFrom(data));
  const [saved, setSaved] = useState(false);
  useEffect(() => { setDraft(draftFrom(data)); setSaved(false); }, [data.allocation_id]);
  const mutation = useMutation({
    mutationFn: () => apiRequest<DataResponse<RecipientWorkspaceData>>(`/api/v1/distribution/allocations/${data.allocation_id}/draft`, { method: 'PATCH', body: JSON.stringify(draft) }),
    onSuccess: ({ data: next }) => { setDraft(draftFrom(next)); onSaved(next); setSaved(true); },
  });
  const update = (key: keyof DraftInput, value: string) => { setSaved(false); setDraft((current) => ({ ...current, [key]: value })); };
  const changed = (key: keyof DraftInput, original: string | undefined) => draft[key] !== (original ?? '');
  const submit = (event: FormEvent) => { event.preventDefault(); mutation.mutate(); };
  const blocked = data.eligibility !== 'eligible' || data.documentation.some((slot) => slot.required && slot.status !== 'complete');

  return <section className={styles.recipientWorkspace}>
    <header className={styles.recipientHeader}><div className={styles.distributionNumber}><small>Nomor pembagian</small><strong>{data.distribution_number}</strong></div><div><h2>{data.full_name}</h2><p>{data.program_name} / {data.regency_name}</p></div><span className={`${styles.eligibilityBanner} ${styles[data.eligibility]}`}>{data.eligibility === 'previously_received' ? <AlertTriangle /> : <CheckCircle2 />}{data.eligibility === 'previously_received' ? 'Penerimaan ulang diblokir' : 'Siap diverifikasi'}</span></header>
    {data.eligibility_reasons.length > 0 && <div className={styles.reasons}>{data.eligibility_reasons.map((reason) => <p key={reason}>{reason}</p>)}</div>}

    <div className={styles.workspaceColumns}>
      <form className={styles.identityForm} onSubmit={submit}>
        <div className={styles.sectionTitle}><h3>Verifikasi identitas</h3><p>Lengkapi bagian yang masih kosong berdasarkan dokumen penerima.</p></div>
        <div className={styles.fields}>
          <label><span>NIK {changed('nik', data.nik) && <small>Diubah</small>}</span><input value={draft.nik} disabled={!canManage} onChange={(event) => update('nik', event.target.value)} /></label>
          <label><span>{data.program_type === 'farmer' ? 'Nomor kartu petani' : 'Nomor KUSUKA'} {changed('sector_identifier', data.sector_identifier) && <small>Diubah</small>}</span><input value={draft.sector_identifier} disabled={!canManage} onChange={(event) => update('sector_identifier', event.target.value)} /></label>
          <label className={styles.fieldWide}><span>Alamat {changed('address', data.address) && <small>Diubah</small>}</span><input aria-label="Alamat" value={draft.address} disabled={!canManage} onChange={(event) => update('address', event.target.value)} /></label>
          <label><span>Desa / kelurahan {changed('village', data.village) && <small>Diubah</small>}</span><input value={draft.village} disabled={!canManage} onChange={(event) => update('village', event.target.value)} /></label>
          <label><span>Kecamatan {changed('district', data.district) && <small>Diubah</small>}</span><input value={draft.district} disabled={!canManage} onChange={(event) => update('district', event.target.value)} /></label>
          <label><span>Nomor telepon {changed('phone_number', data.phone_number) && <small>Diubah</small>}</span><input aria-label="Nomor telepon" value={draft.phone_number} disabled={!canManage} onChange={(event) => update('phone_number', event.target.value)} /></label>
          {draft.nik !== (data.nik ?? '') && <label className={styles.fieldWide}><span>Alasan perubahan NIK</span><input value={draft.identity_change_reason} disabled={!canManage} onChange={(event) => update('identity_change_reason', event.target.value)} /></label>}
        </div>
        {canManage && <div className={styles.formActions}>{saved && <span role="status">Draft penerima tersimpan.</span>}<button className="primaryButton" disabled={mutation.isPending} type="submit"><Save />{mutation.isPending ? 'Menyimpan...' : 'Simpan draft'}</button></div>}
      </form>

      <aside className={styles.sourcePanel}>
        <div className={styles.sectionTitle}><h3>Data sumber DCP3</h3><p>Nilai asli dari file dinas tetap dipertahankan.</p></div>
        <dl>{Object.entries(data.source_snapshot).map(([key, value]) => <div key={key}><dt>{key}</dt><dd>{String(value || '-')}</dd></div>)}</dl>
        {data.receipt_history.length > 0 && <details className={styles.history}><summary>Riwayat penerimaan ({data.receipt_history.length})</summary>{data.receipt_history.map((item) => <div key={`${item.completed_at}-${item.regency}`}><FileText aria-hidden="true" /><span><strong>{item.program}</strong><small>{item.regency} / {formatDate(item.completed_at)}</small>{item.bast_number && <code>{item.bast_number}</code>}</span></div>)}</details>}
      </aside>
    </div>
    <footer className={styles.completion}><div><strong>Konfirmasi distribusi</strong><span>{blocked ? 'Selesaikan data dan seluruh dokumentasi sebelum konfirmasi.' : 'Semua pemeriksaan awal telah terpenuhi.'}</span></div><button className="primaryButton" disabled={blocked}>Selesaikan distribusi</button></footer>
  </section>;
}
