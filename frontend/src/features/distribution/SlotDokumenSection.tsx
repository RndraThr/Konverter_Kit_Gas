import { type FormEvent, type KeyboardEvent, useEffect, useMemo, useState } from 'react';
import { FileText, Pencil, UserCheck } from 'lucide-react';
import { useMutation } from '@tanstack/react-query';
import { apiRequest, ApiError } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import type { CandidateMatch, DataResponse, DistributionSlot, EquipmentOption, LinkSlotInput, ReplaceRecipientInput, UpdateEquipmentInput, UpdateRecipientInput } from './types';
import { DocumentationSlot } from './DocumentationSlot';
import { PosSectionShell } from './PosSectionShell';
import { RevisionDialog } from './RevisionDialog';
import { Button } from '@/components/ui/button';
import { FormField } from '@/components/FormField';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { uppercaseBusinessText } from '@/lib/text';

const emptyLinkInput = (scheduleID: string, slotNumber: number): LinkSlotInput => ({ schedule_id: scheduleID, slot_number: slotNumber, nik: '', address: '', village: '', district: '', phone_number: '', sector_identifier: '' });

type Props = {
  slot: DistributionSlot;
  machineOptions?: EquipmentOption[];
  converterOptions?: EquipmentOption[];
  hoseOptions?: EquipmentOption[];
  onChanged: (slot: DistributionSlot) => void;
};

function withLegacyOption(options: EquipmentOption[], code?: string): EquipmentOption[] {
  if (!code || options.some((option) => option.code === code)) return options;
  return [...options, { code, brand: code }];
}

export function SlotDokumenSection({ slot, machineOptions = [], converterOptions = [], hoseOptions = [], onChanged }: Props) {
  const canManage = useCan('distribution.pos_dokumen');
  const [currentSlot, setCurrentSlot] = useState(slot);
  const [date, setDate] = useState(slot.distribution_date ?? '');
  const [revisionOpen, setRevisionOpen] = useState(false);
  const [replaceMode, setReplaceMode] = useState(false);
  const editable = canManage && currentSlot.status !== 'completed';

  const [nik, setNik] = useState('');
  const [candidate, setCandidate] = useState<CandidateMatch | null>(null);
  const [suggestions, setSuggestions] = useState<CandidateMatch[]>([]);
  const [activeSuggestion, setActiveSuggestion] = useState(-1);
  const [isSearching, setIsSearching] = useState(false);
  const [hasSearched, setHasSearched] = useState(false);
  const [suggestionError, setSuggestionError] = useState<Error | null>(null);
  const [linkInput, setLinkInput] = useState<LinkSlotInput>(() => emptyLinkInput(slot.schedule_id, slot.slot_number));
  const [recipient, setRecipient] = useState<UpdateRecipientInput>({ schedule_id: slot.schedule_id, slot_number: slot.slot_number, address: slot.address ?? '', village: slot.village ?? '', district: slot.district ?? '', phone_number: slot.phone_number ?? '', sector_identifier: slot.sector_identifier ?? '' });
  const [equipment, setEquipment] = useState<UpdateEquipmentInput>({ machine_option_code: slot.machine_option_code ?? '', machine_serial_number: slot.machine_serial_number ?? '', hose_option_code: slot.hose_option_code ?? '', hose_serial_number: slot.hose_serial_number ?? '-', converter_option_code: slot.converter_option_code ?? '', converter_serial_number: slot.converter_serial_number ?? '' });

  useEffect(() => {
    setCurrentSlot(slot);
    setDate(slot.distribution_date ?? '');
    setRecipient({ schedule_id: slot.schedule_id, slot_number: slot.slot_number, address: slot.address ?? '', village: slot.village ?? '', district: slot.district ?? '', phone_number: slot.phone_number ?? '', sector_identifier: slot.sector_identifier ?? '' });
    setEquipment({ machine_option_code: slot.machine_option_code ?? '', machine_serial_number: slot.machine_serial_number ?? '', hose_option_code: slot.hose_option_code ?? '', hose_serial_number: slot.hose_serial_number ?? '-', converter_option_code: slot.converter_option_code ?? '', converter_serial_number: slot.converter_serial_number ?? '' });
  }, [slot]);

  const publish = (next: DistributionSlot) => {
    setCurrentSlot(next);
    setDate(next.distribution_date ?? '');
    onChanged(next);
  };
  const documentation = currentSlot.documentation.filter((item) => item.stage === 'dokumen');
  const mediaStates = useMemo(() => currentSlot.documentation.flatMap((item) => item.files ?? []).map((file) => file.storage_state), [currentSlot.documentation]);
  const movingCount = mediaStates.filter((state) => state === 'moving' || state === 'staging').length;
  const failedCount = mediaStates.filter((state) => state === 'move_failed').length;

  useEffect(() => {
    if (!editable || nik.length < 4 || candidate?.nik === nik) {
      setSuggestions([]); setActiveSuggestion(-1); setIsSearching(false); setHasSearched(false); setSuggestionError(null); return;
    }
    let cancelled = false;
    setIsSearching(true); setSuggestionError(null);
    const timer = window.setTimeout(() => {
      void apiRequest<DataResponse<CandidateMatch[]>>(`/api/v1/distribution/candidate-suggestions?schedule_id=${encodeURIComponent(currentSlot.schedule_id)}&nik_prefix=${encodeURIComponent(nik)}`)
        .then(({ data }) => { if (!cancelled) { setSuggestions(data); setActiveSuggestion(-1); setHasSearched(true); } })
        .catch((error: unknown) => { if (!cancelled) { setSuggestions([]); setSuggestionError(error instanceof Error ? error : new Error('Pencarian penerima gagal.')); setHasSearched(true); } })
        .finally(() => { if (!cancelled) setIsSearching(false); });
    }, 300);
    return () => { cancelled = true; window.clearTimeout(timer); };
  }, [candidate?.nik, currentSlot.schedule_id, editable, nik]);

  const dateUpdate = useMutation({ mutationFn: () => apiRequest<DataResponse<DistributionSlot>>(`/api/v1/distribution/slots/${currentSlot.slot_number}/date?schedule_id=${encodeURIComponent(currentSlot.schedule_id)}`, { method: 'PATCH', body: JSON.stringify({ distribution_date: date }) }), onSuccess: ({ data }) => publish(data) });
  const link = useMutation({ mutationFn: () => apiRequest<DataResponse<DistributionSlot>>(`/api/v1/distribution/slots/${currentSlot.slot_number}/link?schedule_id=${encodeURIComponent(currentSlot.schedule_id)}`, { method: 'POST', body: JSON.stringify(linkInput) }), onSuccess: ({ data }) => publish(data) });
  const recipientUpdate = useMutation({ mutationFn: () => apiRequest<DataResponse<DistributionSlot>>(`/api/v1/distribution/slots/${currentSlot.slot_number}/recipient?schedule_id=${encodeURIComponent(currentSlot.schedule_id)}`, { method: 'PATCH', body: JSON.stringify(recipient) }), onSuccess: ({ data }) => publish(data) });
  const recipientReplace = useMutation({ mutationFn: () => apiRequest<DataResponse<DistributionSlot>>(`/api/v1/distribution/slots/${currentSlot.slot_number}/replace-recipient?schedule_id=${encodeURIComponent(currentSlot.schedule_id)}`, { method: 'POST', body: JSON.stringify(linkInput satisfies ReplaceRecipientInput) }), onSuccess: ({ data }) => { publish(data); setReplaceMode(false); setCandidate(null); setNik(''); } });
  const equipmentUpdate = useMutation({ mutationFn: () => apiRequest<DataResponse<DistributionSlot>>(`/api/v1/distribution/slots/${currentSlot.slot_number}/equipment?schedule_id=${encodeURIComponent(currentSlot.schedule_id)}`, { method: 'PATCH', body: JSON.stringify(equipment) }), onSuccess: ({ data }) => publish(data) });

  const selectCandidate = (next: CandidateMatch) => {
    setNik(next.nik); setCandidate(next); setSuggestions([]); setActiveSuggestion(-1);
    setLinkInput({ schedule_id: currentSlot.schedule_id, slot_number: currentSlot.slot_number, nik: next.nik, address: next.address, village: next.village, district: next.district, phone_number: next.phone_number, sector_identifier: next.sector_identifier });
  };
  const handleNIKKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Escape') { setSuggestions([]); setActiveSuggestion(-1); return; }
    if (!suggestions.length) return;
    if (event.key === 'ArrowDown') { event.preventDefault(); setActiveSuggestion((current) => (current + 1) % suggestions.length); }
    else if (event.key === 'ArrowUp') { event.preventDefault(); setActiveSuggestion((current) => current <= 0 ? suggestions.length - 1 : current - 1); }
    else if (event.key === 'Enter' && activeSuggestion >= 0) { event.preventDefault(); selectCandidate(suggestions[activeSuggestion]); }
  };
  const updateDocumentation = (next: DistributionSlot['documentation'][number]) => publish({ ...currentSlot, documentation: currentSlot.documentation.map((item) => item.code === next.code ? next : item) });

  const candidatePicker = (replacement: boolean) => <div className="space-y-3">
    <div className="relative">
      <FormField label={replacement ? 'NIK pengganti' : 'NIK Penerima'} name={replacement ? 'replacement_nik' : 'nik'} maxLength={16} inputMode="numeric" autoComplete="off" role="combobox" aria-autocomplete="list" aria-expanded={suggestions.length > 0} aria-controls={`nik-suggestions-${currentSlot.id}`} aria-activedescendant={activeSuggestion >= 0 ? `nik-suggestion-${suggestions[activeSuggestion]?.allocation_id}` : undefined} value={nik} hint={nik.length < 4 ? 'Ketik minimal 4 digit NIK.' : isSearching ? 'Mencari penerima...' : undefined} onKeyDown={handleNIKKeyDown} onChange={(event) => { setNik(event.target.value.replace(/\D/g, '')); setCandidate(null); setLinkInput(emptyLinkInput(currentSlot.schedule_id, currentSlot.slot_number)); }} />
      {suggestions.length > 0 && <div id={`nik-suggestions-${currentSlot.id}`} role="listbox" aria-label="Pilihan penerima" className="absolute z-20 mt-1 max-h-64 w-full overflow-y-auto rounded-lg border bg-popover p-1 text-popover-foreground shadow-md">{suggestions.map((item, index) => <button id={`nik-suggestion-${item.allocation_id}`} key={item.allocation_id} type="button" role="option" aria-label={`${item.nik} ${item.full_name}`} aria-selected={index === activeSuggestion} className="flex min-h-11 w-full flex-col items-start rounded-md px-3 py-2 text-left hover:bg-muted focus:bg-muted focus:outline-none aria-selected:bg-muted" onMouseEnter={() => setActiveSuggestion(index)} onClick={() => selectCandidate(item)}><span className="text-sm font-medium">{item.nik}</span><span className="text-xs text-muted-foreground">{item.full_name}</span></button>)}</div>}
    </div>
    {nik.length >= 4 && hasSearched && !isSearching && !suggestions.length && !candidate && !suggestionError && <p className="text-sm text-muted-foreground" role="status">Penerima dengan awalan NIK tersebut tidak ditemukan.</p>}
    {suggestionError && <Alert variant="destructive"><AlertDescription>{suggestionError instanceof ApiError ? suggestionError.message : 'Pencarian penerima belum dapat dilakukan.'}</AlertDescription></Alert>}
    {candidate && <form className="grid gap-3 sm:grid-cols-2" onSubmit={(event: FormEvent) => { event.preventDefault(); replacement ? recipientReplace.mutate() : link.mutate(); }}>
      <FormField className="sm:col-span-2" label="Nama" name="candidate_full_name" value={candidate.full_name} disabled onChange={() => {}} />
      <FormField label={candidate.program_type === 'farmer' ? 'Nomor kartu petani' : 'Nomor KUSUKA'} name="sector_identifier" value={linkInput.sector_identifier} onChange={(event) => setLinkInput({ ...linkInput, sector_identifier: uppercaseBusinessText(event.target.value) })} />
      <FormField label="Nomor telepon" name="phone_number" value={linkInput.phone_number} onChange={(event) => setLinkInput({ ...linkInput, phone_number: event.target.value })} />
      <FormField className="sm:col-span-2" label="Alamat" name="address" value={linkInput.address} onChange={(event) => setLinkInput({ ...linkInput, address: uppercaseBusinessText(event.target.value) })} />
      <FormField label="Desa/kelurahan" name="village" value={linkInput.village} onChange={(event) => setLinkInput({ ...linkInput, village: uppercaseBusinessText(event.target.value) })} />
      <FormField label="Kecamatan" name="district" value={linkInput.district} onChange={(event) => setLinkInput({ ...linkInput, district: uppercaseBusinessText(event.target.value) })} />
      <Button className="sm:col-span-2" disabled={link.isPending || recipientReplace.isPending} type="submit">{replacement ? 'Pasang penerima pengganti' : 'Hubungkan ke Nomor Bagi Ini'}</Button>
    </form>}
  </div>;

  const machineItems = withLegacyOption(machineOptions, equipment.machine_option_code);
  const converterItems = withLegacyOption(converterOptions, equipment.converter_option_code);
  const hoseItems = withLegacyOption(hoseOptions, equipment.hose_option_code);

  return <PosSectionShell label="POS Dokumen" badge="POS Dokumen" icon={<FileText aria-hidden="true" />} title={currentSlot.status === 'open' ? 'Hubungkan penerima' : currentSlot.full_name || 'Lengkapi data distribusi'} state={currentSlot.status === 'completed' ? 'done' : 'active'} status={currentSlot.status === 'completed' ? 'Selesai' : currentSlot.status === 'linked' ? 'Terhubung' : undefined}>
    {canManage && currentSlot.status === 'completed' && <div className="flex justify-end"><Button type="button" variant="outline" onClick={() => setRevisionOpen(true)}><Pencil aria-hidden="true" />Buka revisi POS Dokumen</Button></div>}

    <section aria-label="Pengaturan tanggal" className="space-y-3 rounded-lg border p-4"><h4 className="font-medium">Tanggal distribusi</h4><div className="flex flex-wrap items-end gap-3"><div className="grid min-w-52 flex-1 gap-2"><Label htmlFor={`distribution-date-${currentSlot.id}`}>Tanggal distribusi</Label><Input id={`distribution-date-${currentSlot.id}`} type="date" value={date} disabled={!editable} onChange={(event) => setDate(event.target.value)} /></div>{editable && <Button type="button" variant="outline" disabled={!date || dateUpdate.isPending} onClick={() => dateUpdate.mutate()}>Simpan tanggal distribusi</Button>}</div></section>

    <section aria-labelledby={`recipient-heading-${currentSlot.id}`} className="space-y-3 rounded-lg border p-4"><h4 id={`recipient-heading-${currentSlot.id}`} className="font-medium">Penerima</h4>
      {currentSlot.status === 'open' ? (editable ? candidatePicker(false) : <p className="text-sm text-muted-foreground">Menunggu penerima</p>) : <>
        <form className="grid gap-3 sm:grid-cols-2" onSubmit={(event) => { event.preventDefault(); recipientUpdate.mutate(); }}>
          <FormField className="sm:col-span-2" label="NIK terpasang" name="mounted_nik" value={currentSlot.nik ?? ''} disabled onChange={() => {}} />
          <p className="sr-only">{currentSlot.nik}</p>
          <FormField label="Nomor kartu/KUSUKA" name="mounted_sector_identifier" value={recipient.sector_identifier} disabled={!editable} onChange={(event) => setRecipient({ ...recipient, sector_identifier: uppercaseBusinessText(event.target.value) })} />
          <FormField label="Nomor telepon" name="mounted_phone_number" value={recipient.phone_number} disabled={!editable} onChange={(event) => setRecipient({ ...recipient, phone_number: event.target.value })} />
          <FormField className="sm:col-span-2" label="Alamat" name="mounted_address" value={recipient.address} disabled={!editable} onChange={(event) => setRecipient({ ...recipient, address: uppercaseBusinessText(event.target.value) })} />
          <FormField label="Desa/kelurahan" name="mounted_village" value={recipient.village} disabled={!editable} onChange={(event) => setRecipient({ ...recipient, village: uppercaseBusinessText(event.target.value) })} />
          <FormField label="Kecamatan" name="mounted_district" value={recipient.district} disabled={!editable} onChange={(event) => setRecipient({ ...recipient, district: uppercaseBusinessText(event.target.value) })} />
          {editable && <div className="flex flex-wrap gap-2 sm:col-span-2"><Button type="submit">Simpan data penerima</Button><Button type="button" variant="outline" onClick={() => { setReplaceMode((value) => !value); setNik(''); setCandidate(null); }}>Ganti penerima</Button></div>}
        </form>
        {replaceMode && editable && candidatePicker(true)}
      </>}
    </section>

    <section aria-labelledby={`equipment-heading-${currentSlot.id}`} className="space-y-3 rounded-lg border p-4"><h4 id={`equipment-heading-${currentSlot.id}`} className="font-medium">Data peralatan</h4><form className="grid gap-3 sm:grid-cols-2" onSubmit={(event) => { event.preventDefault(); equipmentUpdate.mutate(); }}>
      <div className="grid gap-2"><Label id={`machine-label-${currentSlot.id}`}>Merk/Tipe Mesin</Label><Select disabled={!editable} value={equipment.machine_option_code} onValueChange={(value) => setEquipment({ ...equipment, machine_option_code: value ?? '' })}><SelectTrigger aria-labelledby={`machine-label-${currentSlot.id}`}><SelectValue placeholder="Pilih mesin" /></SelectTrigger><SelectContent>{machineItems.map((option) => <SelectItem key={option.code} value={option.code}>{option.brand} {option.type}</SelectItem>)}</SelectContent></Select></div>
      <FormField label="Serial Number Mesin" name="machine_serial_number" value={equipment.machine_serial_number} disabled={!editable} onChange={(event) => setEquipment({ ...equipment, machine_serial_number: uppercaseBusinessText(event.target.value) })} />
      <div className="grid gap-2"><Label id={`converter-label-${currentSlot.id}`}>Merk Konkit/Reducer</Label><Select disabled={!editable} value={equipment.converter_option_code} onValueChange={(value) => setEquipment({ ...equipment, converter_option_code: value ?? '' })}><SelectTrigger aria-labelledby={`converter-label-${currentSlot.id}`}><SelectValue placeholder="Pilih konkit/reducer" /></SelectTrigger><SelectContent>{converterItems.map((option) => <SelectItem key={option.code} value={option.code}>{option.brand}</SelectItem>)}</SelectContent></Select></div>
      <FormField label="Serial Number Konkit/Reducer" name="converter_serial_number" value={equipment.converter_serial_number} disabled={!editable} onChange={(event) => setEquipment({ ...equipment, converter_serial_number: uppercaseBusinessText(event.target.value) })} />
      <div className="grid gap-2"><Label id={`hose-label-${currentSlot.id}`}>Merk/Spesifikasi Selang</Label><Select disabled={!editable} value={equipment.hose_option_code} onValueChange={(value) => setEquipment({ ...equipment, hose_option_code: value ?? '' })}><SelectTrigger aria-labelledby={`hose-label-${currentSlot.id}`}><SelectValue placeholder="Pilih selang" /></SelectTrigger><SelectContent>{hoseItems.map((option) => <SelectItem key={option.code} value={option.code}>{option.brand} {option.spec}</SelectItem>)}</SelectContent></Select></div>
      <FormField label="Serial Number Selang" name="hose_serial_number" value={equipment.hose_serial_number} disabled={!editable} onChange={(event) => setEquipment({ ...equipment, hose_serial_number: uppercaseBusinessText(event.target.value) })} />
      {editable && <Button className="sm:col-span-2" type="submit">Simpan data peralatan</Button>}
    </form></section>

    {(movingCount > 0 || failedCount > 0) && <div className="space-y-2">{movingCount > 0 && <div role="status" className="rounded-lg border bg-muted/30 px-3 py-2 text-sm text-muted-foreground">{movingCount} media sedang dipindahkan ke folder final.</div>}{failedCount > 0 && <Alert variant="destructive"><AlertDescription>{failedCount} pemindahan media gagal. Gunakan tombol coba lagi pada media terkait.</AlertDescription></Alert>}</div>}
    {documentation.length > 0 && <section aria-label="Dokumentasi" className="grid gap-4 sm:grid-cols-2">{documentation.map((item) => <DocumentationSlot key={item.code} slot={item} canManage={editable} onChanged={updateDocumentation} />)}</section>}
    <RevisionDialog slot={currentSlot} stage="dokumen" open={revisionOpen} onOpenChange={setRevisionOpen} onReopened={publish} />
  </PosSectionShell>;
}
