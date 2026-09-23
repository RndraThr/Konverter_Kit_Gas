import { FormEvent, useState } from 'react';
import { CheckCircle2, Lock, UserCheck } from 'lucide-react';
import { useMutation } from '@tanstack/react-query';
import { apiRequest, ApiError } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import type { CandidateMatch, DataResponse, DistributionSlot, LinkSlotInput } from './types';
import { DocumentationSlot } from './DocumentationSlot';
import styles from './Distribution.module.css';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { FormField } from '@/components/FormField';
import { Alert, AlertDescription } from '@/components/ui/alert';

const emptyLinkInput = (scheduleID: string, slotNumber: number): LinkSlotInput => ({ schedule_id: scheduleID, slot_number: slotNumber, nik: '', address: '', village: '', district: '', phone_number: '', sector_identifier: '' });

export function SlotDokumenSection({ slot, onChanged }: { slot: DistributionSlot; onChanged: (slot: DistributionSlot) => void }) {
  const canLink = useCan('distribution.pos_dokumen');
  const documentation = slot.documentation.filter((item) => item.stage === 'dokumen');
  const updateDocumentation = (next: DistributionSlot['documentation'][number]) => onChanged({ ...slot, documentation: slot.documentation.map((item) => item.code === next.code ? next : item) });

  const [nik, setNik] = useState('');
  const [candidate, setCandidate] = useState<CandidateMatch | null>(null);
  const [linkInput, setLinkInput] = useState<LinkSlotInput>(() => emptyLinkInput(slot.schedule_id, slot.slot_number));

  const lookup = useMutation({
    mutationFn: () => apiRequest<DataResponse<CandidateMatch>>(`/api/v1/distribution/candidates?schedule_id=${encodeURIComponent(slot.schedule_id)}&nik=${encodeURIComponent(nik)}`),
    onSuccess: ({ data }) => { setCandidate(data); setLinkInput({ schedule_id: slot.schedule_id, slot_number: slot.slot_number, nik: data.nik, address: data.address, village: data.village, district: data.district, phone_number: data.phone_number, sector_identifier: data.sector_identifier }); },
  });

  const link = useMutation({
    mutationFn: () => apiRequest<DataResponse<DistributionSlot>>(`/api/v1/distribution/slots/${slot.slot_number}/link?schedule_id=${encodeURIComponent(slot.schedule_id)}`, { method: 'POST', body: JSON.stringify(linkInput) }),
    onSuccess: ({ data }) => onChanged(data),
  });

  const submitLookup = (event: FormEvent) => { event.preventDefault(); setCandidate(null); lookup.mutate(); };
  const submitLink = (event: FormEvent) => { event.preventDefault(); link.mutate(); };

  if (slot.status === 'open') {
    if (!canLink) {
      return <section className={styles.slotSection} data-state="locked" aria-label="POS Dokumen"><header className={styles.slotSectionHeader}><div><Badge variant="outline">POS Dokumen</Badge><h3><Lock aria-hidden="true" />Menunggu penerima</h3></div></header></section>;
    }
    return <section className={styles.slotSection} data-state="active" aria-label="POS Dokumen">
      <header className={styles.slotSectionHeader}><div><Badge variant="outline">POS Dokumen</Badge><h3><UserCheck aria-hidden="true" />Hubungkan penerima</h3></div></header>
      <form className={styles.fields} onSubmit={submitLookup}>
        <FormField className={styles.fieldWide} label="NIK Penerima" name="nik" maxLength={16} value={nik} onChange={(event) => setNik(event.target.value.replace(/\D/g, ''))} />
        <Button type="submit" disabled={nik.length !== 16 || lookup.isPending}>{lookup.isPending ? 'Mencari...' : 'Cari di DCP3'}</Button>
      </form>
      {lookup.isError && <Alert variant="destructive"><AlertDescription>{lookup.error instanceof ApiError ? lookup.error.message : 'Kandidat tidak ditemukan.'}</AlertDescription></Alert>}
      {candidate && <form className={styles.fields} onSubmit={submitLink}>
        <FormField className={styles.fieldWide} label="Nama" name="candidate_full_name" value={candidate.full_name} disabled onChange={() => {}} />
        <FormField label={candidate.program_type === 'farmer' ? 'Nomor kartu petani' : 'Nomor KUSUKA'} name="sector_identifier" value={linkInput.sector_identifier} onChange={(event) => setLinkInput({ ...linkInput, sector_identifier: event.target.value.toUpperCase() })} />
        <FormField label="Nomor telepon" name="phone_number" value={linkInput.phone_number} onChange={(event) => setLinkInput({ ...linkInput, phone_number: event.target.value })} />
        <FormField className={styles.fieldWide} label="Alamat" name="address" value={linkInput.address} onChange={(event) => setLinkInput({ ...linkInput, address: event.target.value })} />
        <FormField label="Desa/kelurahan" name="village" value={linkInput.village} onChange={(event) => setLinkInput({ ...linkInput, village: event.target.value })} />
        <FormField label="Kecamatan" name="district" value={linkInput.district} onChange={(event) => setLinkInput({ ...linkInput, district: event.target.value })} />
        {link.isError && <Alert className={styles.fieldWide} variant="destructive"><AlertDescription>{link.error instanceof ApiError ? link.error.message : 'Slot belum dapat dihubungkan.'}</AlertDescription></Alert>}
        <Button className={styles.fieldWide} disabled={link.isPending} type="submit">{link.isPending ? 'Menghubungkan...' : 'Hubungkan ke Nomor Bagi Ini'}</Button>
      </form>}
    </section>;
  }

  return <section className={styles.slotSection} data-state="done" aria-label="POS Dokumen">
    <header className={styles.slotSectionHeader}>
      <div><Badge variant="outline">POS Dokumen</Badge><h3><UserCheck aria-hidden="true" />{slot.full_name}</h3></div>
      <span className={styles.slotSectionStatus}><CheckCircle2 aria-hidden="true" />Terhubung</span>
    </header>
    <dl className={styles.slotSummaryList}><div><dt>NIK</dt><dd>{slot.nik}</dd></div></dl>
    <div className={styles.sectionDocumentation}>{documentation.map((item) => <DocumentationSlot key={item.code} slot={item} onChanged={updateDocumentation} />)}</div>
  </section>;
}
