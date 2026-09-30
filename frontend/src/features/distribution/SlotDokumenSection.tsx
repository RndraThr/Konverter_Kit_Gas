import { FormEvent, useState } from 'react';
import { Lock, UserCheck } from 'lucide-react';
import { useMutation } from '@tanstack/react-query';
import { apiRequest, ApiError } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import type { CandidateMatch, DataResponse, DistributionSlot, LinkSlotInput } from './types';
import { DocumentationSlot } from './DocumentationSlot';
import { PosSectionShell } from './PosSectionShell';
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
      return <PosSectionShell label="POS Dokumen" badge="POS Dokumen" icon={<Lock aria-hidden="true" />} title="Menunggu penerima" state="locked" />;
    }
    return <PosSectionShell label="POS Dokumen" badge="POS Dokumen" icon={<UserCheck aria-hidden="true" />} title="Hubungkan penerima" state="active">
      <form className="flex flex-col gap-3 sm:flex-row sm:items-end" onSubmit={submitLookup}>
        <FormField className="flex-1" label="NIK Penerima" name="nik" maxLength={16} value={nik} onChange={(event) => setNik(event.target.value.replace(/\D/g, ''))} />
        <Button className="shrink-0" type="submit" disabled={nik.length !== 16 || lookup.isPending}>{lookup.isPending ? 'Mencari...' : 'Cari di DCP3'}</Button>
      </form>
      {lookup.isError && <Alert variant="destructive"><AlertDescription>{lookup.error instanceof ApiError ? lookup.error.message : 'Kandidat tidak ditemukan.'}</AlertDescription></Alert>}
      {candidate && <form className="grid gap-3 sm:grid-cols-2" onSubmit={submitLink}>
        <FormField className="sm:col-span-2" label="Nama" name="candidate_full_name" value={candidate.full_name} disabled onChange={() => {}} />
        <FormField label={candidate.program_type === 'farmer' ? 'Nomor kartu petani' : 'Nomor KUSUKA'} name="sector_identifier" value={linkInput.sector_identifier} onChange={(event) => setLinkInput({ ...linkInput, sector_identifier: event.target.value.toUpperCase() })} />
        <FormField label="Nomor telepon" name="phone_number" value={linkInput.phone_number} onChange={(event) => setLinkInput({ ...linkInput, phone_number: event.target.value })} />
        <FormField className="sm:col-span-2" label="Alamat" name="address" value={linkInput.address} onChange={(event) => setLinkInput({ ...linkInput, address: event.target.value })} />
        <FormField label="Desa/kelurahan" name="village" value={linkInput.village} onChange={(event) => setLinkInput({ ...linkInput, village: event.target.value })} />
        <FormField label="Kecamatan" name="district" value={linkInput.district} onChange={(event) => setLinkInput({ ...linkInput, district: event.target.value })} />
        {link.isError && <Alert className="sm:col-span-2" variant="destructive"><AlertDescription>{link.error instanceof ApiError ? link.error.message : 'Slot belum dapat dihubungkan.'}</AlertDescription></Alert>}
        <Button className="sm:col-span-2" disabled={link.isPending} type="submit">{link.isPending ? 'Menghubungkan...' : 'Hubungkan ke Nomor Bagi Ini'}</Button>
      </form>}
    </PosSectionShell>;
  }

  return <PosSectionShell label="POS Dokumen" badge="POS Dokumen" icon={<UserCheck aria-hidden="true" />} title={slot.full_name ?? ''} state="done" status="Terhubung">
    <dl className="grid grid-cols-2 gap-x-4 gap-y-3 sm:grid-cols-3">
      <div className="min-w-0"><dt className="text-xs text-muted-foreground">NIK</dt><dd className="text-sm font-medium wrap-break-word">{slot.nik}</dd></div>
    </dl>
    {documentation.length > 0 && <div className="grid gap-4 sm:grid-cols-2">{documentation.map((item) => <DocumentationSlot key={item.code} slot={item} onChanged={updateDocumentation} />)}</div>}
  </PosSectionShell>;
}
