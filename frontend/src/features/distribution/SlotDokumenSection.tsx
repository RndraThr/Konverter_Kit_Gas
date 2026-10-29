import { type FormEvent, type KeyboardEvent, lazy, Suspense, useEffect, useMemo, useState } from 'react';
import { FileText, Pencil, ScanBarcode } from 'lucide-react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { apiRequest, ApiError } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import type { CandidateLookup, CandidateMatch, DataResponse, DistributionSlot, EquipmentOption, LinkSlotInput, MediaFile, RecipientReplacement, ReplaceRecipientInput, UpdateEquipmentInput, UpdateRecipientInput } from './types';
import { DocumentationSlot } from './DocumentationSlot';
import { HandoverLocationField } from './HandoverLocationField';
import { RecipientSignature } from './RecipientSignature';
import { PosSectionShell } from './PosSectionShell';
import { RevisionDialog } from './RevisionDialog';
import { Button } from '@/components/ui/button';
import { FormField, RequirementMark } from '@/components/FormField';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { uppercaseBusinessText } from '@/lib/text';

const emptyLinkInput = (scheduleID: string, slotNumber: number): LinkSlotInput => ({ schedule_id: scheduleID, slot_number: slotNumber, nik: '', address: '', village: '', district: '', phone_number: '', sector_identifier: '' });

const PHONE_PATTERN = /^08\d{8,11}$/;
const PHONE_HINT = 'Diawali 08, 10–13 digit.';

// Keeps digits only, rewrites an international +62 prefix to the local 0, and caps the length at
// the longest valid Indonesian mobile number.
export function normalizePhoneInput(value: string): string {
  let digits = value.replace(/\D/g, '');
  if (digits.startsWith('628')) digits = `0${digits.slice(2)}`;
  return digits.slice(0, 13);
}

type RecipientFields = Pick<LinkSlotInput, 'address' | 'village' | 'district' | 'phone_number' | 'sector_identifier'>;
type RecipientErrors = Partial<Record<keyof RecipientFields, string>>;

// Address, village, district and phone number are mandatory for every recipient the web form
// saves. The farmer card number is mandatory for farmer programs; the KUSUKA number of fisherman
// programs stays optional.
export function recipientErrors(values: RecipientFields, programType?: 'farmer' | 'fisherman'): RecipientErrors {
  const errors: RecipientErrors = {};
  if (programType === 'farmer' && !values.sector_identifier.trim()) errors.sector_identifier = 'Nomor kartu tani wajib diisi.';
  if (!values.phone_number) errors.phone_number = 'Nomor HP wajib diisi.';
  else if (!PHONE_PATTERN.test(values.phone_number)) errors.phone_number = 'Nomor HP harus diawali 08 dan terdiri dari 10–13 digit.';
  if (!values.address.trim()) errors.address = 'Alamat wajib diisi.';
  if (!values.village.trim()) errors.village = 'Desa/kelurahan wajib diisi.';
  if (!values.district.trim()) errors.district = 'Kecamatan wajib diisi.';
  return errors;
}
const hasErrors = (errors: RecipientErrors) => Object.keys(errors).length > 0;

// Camera barcode scanning is lazy-loaded so @zxing only downloads when an officer actually starts
// a scan. POS Dokumen remains the fallback for the two serial numbers that can also be captured
// earlier at POS Mesin.
const BarcodeScanner = lazy(() => import('./BarcodeScanner'));

type SerialField = 'machine_serial_number' | 'converter_serial_number';

type Props = {
  slot: DistributionSlot;
  machineOptions?: EquipmentOption[];
  converterOptions?: EquipmentOption[];
  hoseOptions?: EquipmentOption[];
  onChanged: (slot: DistributionSlot) => void;
  // Program type of the selected schedule; decides whether the farmer card number is mandatory.
  programType?: 'farmer' | 'fisherman';
};

function withLegacyOption(options: EquipmentOption[], code?: string): EquipmentOption[] {
  if (!code || options.some((option) => option.code === code)) return options;
  return [...options, { code, brand: code }];
}

export function SlotDokumenSection({ slot, machineOptions = [], converterOptions = [], hoseOptions = [], onChanged, programType }: Props) {
  const canManage = useCan('distribution.pos_dokumen');
  const [currentSlot, setCurrentSlot] = useState(slot);
  const { schedule_id: scheduleID, slot_number: slotNumber, distribution_date: distributionDate, address, village, district, phone_number: phoneNumber, sector_identifier: sectorIdentifier,
    machine_option_code: machineOptionCode, machine_serial_number: machineSerial, hose_option_code: hoseOptionCode, converter_option_code: converterOptionCode, converter_serial_number: converterSerial } = slot;
  const [date, setDate] = useState(slot.distribution_date ?? '');
  const [revisionOpen, setRevisionOpen] = useState(false);
  const [replaceMode, setReplaceMode] = useState(false);
  const editable = canManage && currentSlot.status !== 'completed';
  const canCorrectHandover = useCan('bast.manage');

  const [nik, setNik] = useState('');
  const [candidate, setCandidate] = useState<CandidateMatch | null>(null);
  const [suggestions, setSuggestions] = useState<CandidateMatch[]>([]);
  const [activeSuggestion, setActiveSuggestion] = useState(-1);
  const [isSearching, setIsSearching] = useState(false);
  const [hasSearched, setHasSearched] = useState(false);
  const [suggestionError, setSuggestionError] = useState<Error | null>(null);
  const [linkInput, setLinkInput] = useState<LinkSlotInput>(() => emptyLinkInput(slot.schedule_id, slot.slot_number));
  const [recipient, setRecipient] = useState<UpdateRecipientInput>({ schedule_id: slot.schedule_id, slot_number: slot.slot_number, address: slot.address ?? '', village: slot.village ?? '', district: slot.district ?? '', phone_number: slot.phone_number ?? '', sector_identifier: slot.sector_identifier ?? '' });
  const [equipment, setEquipment] = useState<UpdateEquipmentInput>({ machine_option_code: slot.machine_option_code ?? '', machine_serial_number: slot.machine_serial_number ?? '', hose_option_code: slot.hose_option_code ?? '', hose_serial_number: '', converter_option_code: slot.converter_option_code ?? '', converter_serial_number: slot.converter_serial_number ?? '' });
  const [scanning, setScanning] = useState<SerialField | null>(null);
  const [replacementReason, setReplacementReason] = useState('');
  const [replacementName, setReplacementName] = useState('');
  const [lookup, setLookup] = useState<CandidateLookup | null>(null);
  const [lookupNIK, setLookupNIK] = useState('');
  const [pickerOpen, setPickerOpen] = useState(false);
  const [linkAttempted, setLinkAttempted] = useState(false);
  const [recipientAttempted, setRecipientAttempted] = useState(false);

  useEffect(() => setCurrentSlot(slot), [slot]);
  // The form fields only reset when their saved values change, so a background refresh of the
  // documentation (media moving to the final folder) does not wipe what the officer is typing.
  useEffect(() => setDate(distributionDate ?? ''), [distributionDate]);
  useEffect(() => {
    setRecipient({ schedule_id: scheduleID, slot_number: slotNumber, address: address ?? '', village: village ?? '', district: district ?? '', phone_number: phoneNumber ?? '', sector_identifier: sectorIdentifier ?? '' });
    setRecipientAttempted(false);
  }, [address, district, phoneNumber, scheduleID, sectorIdentifier, slotNumber, village]);
  useEffect(() => {
    setEquipment({ machine_option_code: machineOptionCode ?? '', machine_serial_number: machineSerial ?? '', hose_option_code: hoseOptionCode ?? '', hose_serial_number: '', converter_option_code: converterOptionCode ?? '', converter_serial_number: converterSerial ?? '' });
  }, [converterOptionCode, converterSerial, hoseOptionCode, machineOptionCode, machineSerial]);

  const publish = (next: DistributionSlot) => {
    setCurrentSlot(next);
    setDate(next.distribution_date ?? '');
    onChanged(next);
  };
  const documentation = currentSlot.documentation.filter((item) => item.stage === 'dokumen');
  const mediaStates = useMemo(() => currentSlot.documentation.flatMap((item) => item.files ?? []).map((file) => file.storage_state), [currentSlot.documentation]);
  const movingCount = mediaStates.filter((state) => state === 'moving' || state === 'staging').length;
  const failedCount = mediaStates.filter((state) => state === 'move_failed').length;
  const failedMediaOutsideDocument = useMemo(() => currentSlot.documentation
    .filter((item) => item.stage !== 'dokumen')
    .flatMap((item) => item.files ?? [])
    .filter((file) => file.storage_state === 'move_failed'), [currentSlot.documentation]);

  useEffect(() => {
    // An empty field still lists candidates while it has focus, so the officer can pick without
    // typing; once digits are typed the list narrows by NIK prefix.
    if (!editable || candidate?.nik === nik || (nik === '' && !pickerOpen)) {
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
  }, [candidate?.nik, currentSlot.schedule_id, editable, nik, pickerOpen]);

  // A replacement may target someone who is not in DCP3 at all. The suggestion list is empty both
  // for "not registered" and for "registered but not receivable", so ask the backend which one it
  // is before offering to create recipient data.
  const needsLookup = replaceMode && editable && nik.length === 16 && hasSearched && !isSearching && !suggestions.length && !candidate;
  useEffect(() => {
    if (!needsLookup || lookupNIK === nik) return;
    let cancelled = false;
    void apiRequest<DataResponse<CandidateLookup>>(`/api/v1/distribution/candidate-lookup?schedule_id=${encodeURIComponent(currentSlot.schedule_id)}&nik=${encodeURIComponent(nik)}`)
      .then(({ data }) => { if (!cancelled) { setLookup(data); setLookupNIK(nik); } })
      .catch(() => { if (!cancelled) { setLookup(null); setLookupNIK(nik); } });
    return () => { cancelled = true; };
  }, [currentSlot.schedule_id, lookupNIK, needsLookup, nik]);

  const replacements = useQuery({
    queryKey: ['distribution', 'replacements', currentSlot.schedule_id, currentSlot.slot_number],
    queryFn: () => apiRequest<DataResponse<RecipientReplacement[]>>(`/api/v1/distribution/slots/${currentSlot.slot_number}/replacements?schedule_id=${encodeURIComponent(currentSlot.schedule_id)}`),
    enabled: currentSlot.status !== 'open',
  });
  const replacementHistory = replacements.data?.data ?? [];

  const closeReplacement = () => {
    setReplaceMode(false); setCandidate(null); setNik(''); setLinkAttempted(false);
    setReplacementName(''); setReplacementReason(''); setLookup(null); setLookupNIK('');
  };
  const unregisteredSubstitute = replaceMode && lookup?.state === 'not_registered';
  // A replacement form is offered for a matched candidate, for a substitute who is not registered
  // yet, and for a NIK the backend confirms is receivable but that the suggestion list missed.
  const substituteFormVisible = candidate !== null || unregisteredSubstitute || (replaceMode && lookup?.state === 'receivable');
  const replacementReady = replacementReason.trim() !== '' && (!unregisteredSubstitute || replacementName.trim() !== '');

  const dateUpdate = useMutation({ mutationFn: () => apiRequest<DataResponse<DistributionSlot>>(`/api/v1/distribution/slots/${currentSlot.slot_number}/date?schedule_id=${encodeURIComponent(currentSlot.schedule_id)}`, { method: 'PATCH', body: JSON.stringify({ distribution_date: date }) }), onSuccess: ({ data }) => publish(data) });
  const link = useMutation({ mutationFn: () => apiRequest<DataResponse<DistributionSlot>>(`/api/v1/distribution/slots/${currentSlot.slot_number}/link?schedule_id=${encodeURIComponent(currentSlot.schedule_id)}`, { method: 'POST', body: JSON.stringify(linkInput) }), onSuccess: ({ data }) => publish(data) });
  const recipientUpdate = useMutation({ mutationFn: () => apiRequest<DataResponse<DistributionSlot>>(`/api/v1/distribution/slots/${currentSlot.slot_number}/recipient?schedule_id=${encodeURIComponent(currentSlot.schedule_id)}`, { method: 'PATCH', body: JSON.stringify(recipient) }), onSuccess: ({ data }) => publish(data) });
  const recipientReplace = useMutation({
    mutationFn: () => apiRequest<DataResponse<DistributionSlot>>(`/api/v1/distribution/slots/${currentSlot.slot_number}/replace-recipient?schedule_id=${encodeURIComponent(currentSlot.schedule_id)}`, {
      method: 'POST',
      body: JSON.stringify({
        ...linkInput,
        slot_number: currentSlot.slot_number,
        nik,
        full_name: candidate?.full_name ?? replacementName,
        reason: replacementReason.trim(),
      } satisfies ReplaceRecipientInput),
    }),
    onSuccess: ({ data }) => { publish(data); closeReplacement(); void replacements.refetch(); },
  });
  const equipmentUpdate = useMutation({ mutationFn: () => apiRequest<DataResponse<DistributionSlot>>(`/api/v1/distribution/slots/${currentSlot.slot_number}/equipment?schedule_id=${encodeURIComponent(currentSlot.schedule_id)}`, { method: 'PATCH', body: JSON.stringify(equipment) }), onSuccess: ({ data }) => publish(data) });
  const retryMove = useMutation({
    mutationFn: (id: string) => apiRequest<DataResponse<MediaFile>>(`/api/v1/distribution/media/${id}/retry-move`, { method: 'POST' }),
    onSuccess: ({ data }) => publish({
      ...currentSlot,
      documentation: currentSlot.documentation.map((item) => ({ ...item, files: (item.files ?? []).map((file) => file.id === data.id ? data : file) })),
    }),
  });

  const selectCandidate = (next: CandidateMatch) => {
    setNik(next.nik); setCandidate(next); setSuggestions([]); setActiveSuggestion(-1); setPickerOpen(false); setLinkAttempted(false);
    setLinkInput({ schedule_id: currentSlot.schedule_id, slot_number: currentSlot.slot_number, nik: next.nik, address: next.address, village: next.village, district: next.district, phone_number: normalizePhoneInput(next.phone_number), sector_identifier: next.sector_identifier });
  };
  const handleNIKKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Escape') { setPickerOpen(false); setActiveSuggestion(-1); return; }
    if (!suggestions.length) return;
    if (event.key === 'ArrowDown') { event.preventDefault(); setActiveSuggestion((current) => (current + 1) % suggestions.length); }
    else if (event.key === 'ArrowUp') { event.preventDefault(); setActiveSuggestion((current) => current <= 0 ? suggestions.length - 1 : current - 1); }
    else if (event.key === 'Enter' && activeSuggestion >= 0) { event.preventDefault(); selectCandidate(suggestions[activeSuggestion]); }
  };
  const linkProgramType = candidate?.program_type ?? programType;
  const linkErrors = recipientErrors(linkInput, linkProgramType);
  const recipientFormErrors = recipientErrors(recipient, programType);
  const cardLabel = (type?: 'farmer' | 'fisherman') => type === 'fisherman' ? 'Nomor KUSUKA' : type === 'farmer' ? 'Nomor kartu tani' : 'Nomor kartu/KUSUKA';
  const updateDocumentation = (next: DistributionSlot['documentation'][number]) => publish({ ...currentSlot, documentation: currentSlot.documentation.map((item) => item.code === next.code ? next : item) });

  // Explains an empty suggestion list. Only "not_registered" invites creating recipient data;
  // every other state must steer the officer to fix the existing record instead, because a NIK that
  // already exists cannot be added a second time.
  const lookupPanel = () => {
    if (!replaceMode || !lookup || candidate) return null;
    switch (lookup.state) {
      case 'not_registered':
        return <Alert><AlertDescription className="space-y-1"><p className="font-medium">Pengganti belum terdaftar</p><p>NIK ini belum terdaftar pada jadwal ini. Lengkapi nama dan alamat di bawah, lalu data penerima beserta alokasi baru dibuat saat penggantian disimpan.</p></AlertDescription></Alert>;
      case 'receivable':
        return <Alert><AlertDescription>Penerima ini siap dipasang sebagai pengganti.</AlertDescription></Alert>;
      case 'needs_review':
        return <Alert variant="destructive"><AlertDescription>NIK ini masih berstatus <strong>perlu ditinjau</strong> pada data DCP3 sehingga belum dapat menerima paket. Selesaikan peninjauannya terlebih dahulu.</AlertDescription></Alert>;
      case 'not_available':
        return <Alert variant="destructive"><AlertDescription>Alokasi NIK ini sudah tidak dapat menerima paket{lookup.allocation_status ? ` (status ${lookup.allocation_status})` : ''}. Pulihkan datanya di Data Penerima bila memang akan dipakai kembali.</AlertDescription></Alert>;
      case 'already_assigned':
        return <Alert variant="destructive"><AlertDescription>NIK ini sudah terpasang pada nomor bagi {lookup.distribution_number ?? '-'}, sehingga tidak dapat dipasang di nomor ini.</AlertDescription></Alert>;
      case 'previously_received':
        return <Alert variant="destructive"><AlertDescription>NIK ini sudah pernah menerima paket sebelumnya sehingga tidak dapat dipasang sebagai pengganti.</AlertDescription></Alert>;
      default:
        return null;
    }
  };

  const candidatePicker = (replacement: boolean) => <div className="space-y-3">
    <div className="relative">
      <FormField label={replacement ? 'NIK pengganti' : 'NIK Penerima'} name={replacement ? 'replacement_nik' : 'nik'} maxLength={16} inputMode="numeric" autoComplete="off" role="combobox" aria-autocomplete="list" aria-expanded={pickerOpen && suggestions.length > 0} aria-controls={`nik-suggestions-${currentSlot.id}`} aria-activedescendant={activeSuggestion >= 0 ? `nik-suggestion-${suggestions[activeSuggestion]?.allocation_id}` : undefined} value={nik} hint={isSearching ? 'Mencari penerima...' : candidate ? undefined : 'Ketik NIK atau pilih dari daftar penerima.'} onKeyDown={handleNIKKeyDown} onFocus={() => setPickerOpen(true)} onBlur={() => setPickerOpen(false)} onChange={(event) => { setPickerOpen(true); setNik(event.target.value.replace(/\D/g, '')); setCandidate(null); setLinkAttempted(false); setLookup(null); setLookupNIK(''); setLinkInput(emptyLinkInput(currentSlot.schedule_id, currentSlot.slot_number)); }} />
      {pickerOpen && suggestions.length > 0 && <div id={`nik-suggestions-${currentSlot.id}`} role="listbox" aria-label="Pilihan penerima" className="absolute z-20 mt-1 max-h-64 w-full overflow-y-auto rounded-lg border bg-popover p-1 text-popover-foreground shadow-md">{suggestions.map((item, index) => <button id={`nik-suggestion-${item.allocation_id}`} key={item.allocation_id} type="button" role="option" aria-label={`${item.nik} ${item.full_name}`} aria-selected={index === activeSuggestion} className="flex min-h-11 w-full flex-col items-start rounded-md px-3 py-2 text-left hover:bg-muted focus:bg-muted focus:outline-none aria-selected:bg-muted" onMouseEnter={() => setActiveSuggestion(index)} onMouseDown={(event) => event.preventDefault()} onClick={() => selectCandidate(item)}><span className="text-sm font-medium">{item.nik}</span><span className="text-xs text-muted-foreground">{item.full_name}</span></button>)}</div>}
    </div>
    {nik.length > 0 && hasSearched && !isSearching && !suggestions.length && !candidate && !suggestionError && !replaceMode && <p className="text-sm text-muted-foreground" role="status">Penerima dengan awalan NIK tersebut tidak ditemukan.</p>}
    {suggestionError && <Alert variant="destructive"><AlertDescription>{suggestionError instanceof ApiError ? suggestionError.message : 'Pencarian penerima belum dapat dilakukan.'}</AlertDescription></Alert>}
    {replacement && lookupPanel()}
    {substituteFormVisible && <form noValidate className="grid gap-3 sm:grid-cols-2" onSubmit={(event: FormEvent) => { event.preventDefault(); setLinkAttempted(true); if (hasErrors(linkErrors) || (replacement && !replacementReady)) return; replacement ? recipientReplace.mutate() : link.mutate(); }}>
      <p className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground sm:col-span-2">Lengkapi kolom bertanda <RequirementMark requirement="required" /> sebelum menyimpan.</p>
      {candidate
        ? <FormField className="sm:col-span-2" label="Nama" name="candidate_full_name" value={candidate.full_name} disabled onChange={() => {}} />
        : <FormField className="sm:col-span-2" label="Nama lengkap pengganti" name="replacement_full_name" requirement="required" value={replacementName} onChange={(event) => setReplacementName(uppercaseBusinessText(event.target.value))} />}
      <FormField label={cardLabel(linkProgramType)} name="sector_identifier" requirement={linkProgramType === 'farmer' ? 'required' : 'optional'} error={linkAttempted ? linkErrors.sector_identifier : undefined} value={linkInput.sector_identifier} onChange={(event) => setLinkInput({ ...linkInput, sector_identifier: uppercaseBusinessText(event.target.value) })} />
      <FormField label="Nomor HP" name="phone_number" requirement="required" type="tel" inputMode="numeric" autoComplete="off" maxLength={13} placeholder="08xxxxxxxxxx" hint={PHONE_HINT} error={linkAttempted ? linkErrors.phone_number : undefined} value={linkInput.phone_number} onChange={(event) => setLinkInput({ ...linkInput, phone_number: normalizePhoneInput(event.target.value) })} />
      <FormField className="sm:col-span-2" label="Alamat" name="address" requirement="required" error={linkAttempted ? linkErrors.address : undefined} value={linkInput.address} onChange={(event) => setLinkInput({ ...linkInput, address: uppercaseBusinessText(event.target.value) })} />
      <FormField label="Desa/kelurahan" name="village" requirement="required" error={linkAttempted ? linkErrors.village : undefined} value={linkInput.village} onChange={(event) => setLinkInput({ ...linkInput, village: uppercaseBusinessText(event.target.value) })} />
      <FormField label="Kecamatan" name="district" requirement="required" error={linkAttempted ? linkErrors.district : undefined} value={linkInput.district} onChange={(event) => setLinkInput({ ...linkInput, district: uppercaseBusinessText(event.target.value) })} />
      {replacement && <div className="grid gap-2 sm:col-span-2">
        <div className="flex items-center gap-2"><label className="text-sm font-medium" htmlFor={`replacement-reason-${currentSlot.id}`}>Alasan penggantian</label><RequirementMark requirement="required" /></div>
        <textarea id={`replacement-reason-${currentSlot.id}`} className="min-h-20 rounded-lg border border-input bg-transparent px-3 py-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50" value={replacementReason} onChange={(event) => setReplacementReason(event.target.value)} />
        <p className="text-xs text-muted-foreground">Alasan disimpan pada riwayat penggantian dan tidak dapat dikosongkan.</p>
      </div>}
      <Button className="sm:col-span-2" disabled={link.isPending || recipientReplace.isPending || (replacement && !replacementReady)} type="submit">{replacement ? 'Pasang penerima pengganti' : 'Hubungkan ke Nomor Bagi Ini'}</Button>
      {linkAttempted && hasErrors(linkErrors) && <p role="alert" className="text-sm text-destructive sm:col-span-2">Data penerima belum lengkap. Periksa kolom bertanda merah.</p>}
      {link.isError && <p role="alert" className="text-sm text-destructive sm:col-span-2">{link.error instanceof ApiError ? link.error.message : 'Penerima belum dapat dihubungkan.'}</p>}
      {recipientReplace.isError && <p role="alert" className="text-sm text-destructive sm:col-span-2">{recipientReplace.error instanceof ApiError ? recipientReplace.error.message : 'Penggantian penerima belum dapat disimpan.'}</p>}
    </form>}
  </div>;

  const machineItems = withLegacyOption(machineOptions, equipment.machine_option_code);
  const converterItems = withLegacyOption(converterOptions, equipment.converter_option_code);
  const hoseItems = withLegacyOption(hoseOptions, equipment.hose_option_code);

  const serialField = (label: string, field: SerialField) => {
    const fieldID = `${field}-${currentSlot.id}`;
    return <div className="grid min-w-0 gap-2">
      <Label htmlFor={fieldID}>{label}</Label>
      <div className="flex gap-2">
        <Input id={fieldID} name={field} className="flex-1" disabled={!editable} value={equipment[field]} onChange={(event) => setEquipment({ ...equipment, [field]: uppercaseBusinessText(event.target.value) })} />
        {editable && <Button type="button" variant="outline" size="icon" aria-label={`Scan ${label}`} title="Scan barcode" onClick={() => setScanning(field)}><ScanBarcode aria-hidden="true" /></Button>}
      </div>
    </div>;
  };

  return <PosSectionShell label="POS Dokumen" badge="POS Dokumen" icon={<FileText aria-hidden="true" />} title={currentSlot.status === 'open' ? 'Hubungkan penerima' : currentSlot.full_name || 'Lengkapi data distribusi'} state={currentSlot.status === 'completed' ? 'done' : 'active'} status={currentSlot.status === 'completed' ? 'Selesai' : currentSlot.status === 'linked' ? 'Terhubung' : undefined}>
    {canManage && currentSlot.status === 'completed' && <div className="flex justify-end"><Button type="button" variant="outline" onClick={() => setRevisionOpen(true)}><Pencil aria-hidden="true" />Buka revisi POS Dokumen</Button></div>}

    <section aria-label="Pengaturan tanggal" className="space-y-3 rounded-lg border p-4"><h4 className="font-medium">Tanggal distribusi</h4><div className="flex flex-wrap items-end gap-3"><div className="grid min-w-52 flex-1 gap-2"><Label htmlFor={`distribution-date-${currentSlot.id}`}>Tanggal distribusi</Label><Input id={`distribution-date-${currentSlot.id}`} type="date" value={date} disabled={!editable} onChange={(event) => setDate(event.target.value)} /></div>{editable && <Button type="button" variant="outline" disabled={!date || dateUpdate.isPending} onClick={() => dateUpdate.mutate()}>Simpan tanggal distribusi</Button>}</div></section>
    <HandoverLocationField slot={currentSlot} editable={editable} correctable={canCorrectHandover} onSaved={publish} />

    <section aria-labelledby={`recipient-heading-${currentSlot.id}`} className="space-y-3 rounded-lg border p-4"><h4 id={`recipient-heading-${currentSlot.id}`} className="font-medium">Penerima</h4>
      {currentSlot.status === 'open' ? (editable ? candidatePicker(false) : <p className="text-sm text-muted-foreground">Menunggu penerima</p>) : <>
        <form noValidate className="grid gap-3 sm:grid-cols-2" onSubmit={(event) => { event.preventDefault(); setRecipientAttempted(true); if (hasErrors(recipientFormErrors)) return; recipientUpdate.mutate(); }}>
          <FormField className="sm:col-span-2" label="NIK terpasang" name="mounted_nik" value={currentSlot.nik ?? ''} disabled onChange={() => {}} />
          <p className="sr-only">{currentSlot.nik}</p>
          <FormField label={cardLabel(programType)} name="mounted_sector_identifier" requirement={programType === 'farmer' ? 'required' : 'optional'} error={editable && recipientAttempted ? recipientFormErrors.sector_identifier : undefined} value={recipient.sector_identifier} disabled={!editable} onChange={(event) => setRecipient({ ...recipient, sector_identifier: uppercaseBusinessText(event.target.value) })} />
          <FormField label="Nomor HP" name="mounted_phone_number" requirement="required" type="tel" inputMode="numeric" autoComplete="off" maxLength={13} placeholder="08xxxxxxxxxx" hint={editable ? PHONE_HINT : undefined} error={editable && recipientAttempted ? recipientFormErrors.phone_number : undefined} value={recipient.phone_number} disabled={!editable} onChange={(event) => setRecipient({ ...recipient, phone_number: normalizePhoneInput(event.target.value) })} />
          <FormField className="sm:col-span-2" label="Alamat" name="mounted_address" requirement="required" error={editable && recipientAttempted ? recipientFormErrors.address : undefined} value={recipient.address} disabled={!editable} onChange={(event) => setRecipient({ ...recipient, address: uppercaseBusinessText(event.target.value) })} />
          <FormField label="Desa/kelurahan" name="mounted_village" requirement="required" error={editable && recipientAttempted ? recipientFormErrors.village : undefined} value={recipient.village} disabled={!editable} onChange={(event) => setRecipient({ ...recipient, village: uppercaseBusinessText(event.target.value) })} />
          <FormField label="Kecamatan" name="mounted_district" requirement="required" error={editable && recipientAttempted ? recipientFormErrors.district : undefined} value={recipient.district} disabled={!editable} onChange={(event) => setRecipient({ ...recipient, district: uppercaseBusinessText(event.target.value) })} />
          {editable && <div className="flex flex-wrap gap-2 sm:col-span-2"><Button type="submit" disabled={recipientUpdate.isPending}>Simpan data penerima</Button><Button type="button" variant="outline" onClick={() => { if (replaceMode) { closeReplacement(); return; } setReplaceMode(true); setNik(''); setCandidate(null); setLookup(null); setLookupNIK(''); setReplacementName(''); setReplacementReason(''); }}>Ganti penerima</Button></div>}
          {editable && recipientAttempted && hasErrors(recipientFormErrors) && <p role="alert" className="text-sm text-destructive sm:col-span-2">Data penerima belum lengkap. Periksa kolom bertanda merah.</p>}
          {recipientUpdate.isError && <p role="alert" className="text-sm text-destructive sm:col-span-2">{recipientUpdate.error instanceof ApiError ? recipientUpdate.error.message : 'Data penerima belum dapat disimpan.'}</p>}
        </form>
        {replaceMode && editable && candidatePicker(true)}
      </>}
    </section>

    {currentSlot.status !== 'open' && <RecipientSignature key={currentSlot.nik ?? ''} slot={currentSlot} editable={editable} onChanged={publish} />}

    {replacementHistory.length > 0 && <section aria-label="Riwayat penggantian penerima" className="space-y-3 rounded-lg border p-4">
      <h4 className="font-medium">Riwayat penggantian penerima</h4>
      <p className="text-sm text-muted-foreground">Nomor bagi ini sudah {replacementHistory.length} kali berganti penerima. Penerima sebelumnya tetap tercatat pada Data Penerima.</p>
      <ol className="space-y-2">
        {replacementHistory.map((entry) => <li key={entry.id} className="rounded-md border bg-muted/30 px-3 py-2 text-sm">
          <p><strong>{entry.old_full_name}</strong> digantikan oleh <strong>{entry.new_full_name}</strong></p>
          <p className="text-muted-foreground">Alasan: {entry.reason}</p>
          <p className="text-xs text-muted-foreground">{new Date(entry.replaced_at).toLocaleString('id-ID')}{entry.replaced_by_name ? ` · dicatat oleh ${entry.replaced_by_name}` : ''}{entry.origin === 'new_allocation' ? ' · data penerima baru dibuat' : ''}</p>
        </li>)}
      </ol>
    </section>}

    <section aria-labelledby={`equipment-heading-${currentSlot.id}`} className="space-y-3 rounded-lg border p-4"><h4 id={`equipment-heading-${currentSlot.id}`} className="font-medium">Data peralatan</h4><form className="grid gap-3 sm:grid-cols-2" onSubmit={(event) => { event.preventDefault(); equipmentUpdate.mutate(); }}>
      <div className="grid gap-2"><Label id={`machine-label-${currentSlot.id}`}>Merk/Tipe Mesin</Label><Select disabled={!editable} value={equipment.machine_option_code} onValueChange={(value) => setEquipment({ ...equipment, machine_option_code: value ?? '' })}><SelectTrigger aria-labelledby={`machine-label-${currentSlot.id}`}><SelectValue placeholder="Pilih mesin" /></SelectTrigger><SelectContent>{machineItems.map((option) => <SelectItem key={option.code} value={option.code}>{option.brand} {option.type}</SelectItem>)}</SelectContent></Select></div>
      {serialField('Serial Number Mesin', 'machine_serial_number')}
      <div className="grid gap-2"><Label id={`converter-label-${currentSlot.id}`}>Merk Konkit/Reducer</Label><Select disabled={!editable} value={equipment.converter_option_code} onValueChange={(value) => setEquipment({ ...equipment, converter_option_code: value ?? '' })}><SelectTrigger aria-labelledby={`converter-label-${currentSlot.id}`}><SelectValue placeholder="Pilih konkit/reducer" /></SelectTrigger><SelectContent>{converterItems.map((option) => <SelectItem key={option.code} value={option.code}>{option.brand}</SelectItem>)}</SelectContent></Select></div>
      {serialField('Serial Number Konkit/Reducer', 'converter_serial_number')}
      <div className="grid gap-2"><Label id={`hose-label-${currentSlot.id}`}>Selang Hisap &amp; Buang</Label><Select disabled={!editable} value={equipment.hose_option_code} onValueChange={(value) => setEquipment({ ...equipment, hose_option_code: value ?? '' })}><SelectTrigger aria-labelledby={`hose-label-${currentSlot.id}`}><SelectValue placeholder="Pilih selang" /></SelectTrigger><SelectContent>{hoseItems.map((option) => <SelectItem key={option.code} value={option.code}>{option.brand} {option.spec}</SelectItem>)}</SelectContent></Select></div>
      {editable && <Button className="sm:col-span-2" type="submit">Simpan data peralatan</Button>}
    </form></section>

    {(movingCount > 0 || failedCount > 0) && <div className="space-y-2">{movingCount > 0 && <div role="status" className="rounded-lg border bg-muted/30 px-3 py-2 text-sm text-muted-foreground">{movingCount} media sedang dipindahkan ke folder final.</div>}{failedCount > 0 && <Alert variant="destructive"><AlertDescription className="space-y-2"><p>{failedCount} pemindahan media gagal. Gunakan tombol coba lagi pada media terkait.</p>{editable && failedMediaOutsideDocument.map((file) => <Button key={file.id} type="button" size="sm" variant="outline" aria-label={`Coba pindahkan lagi ${file.original_filename}`} disabled={retryMove.isPending} onClick={() => retryMove.mutate(file.id)}>Coba lagi: {file.original_filename}</Button>)}{retryMove.isError && <p>{retryMove.error instanceof ApiError ? retryMove.error.message : 'Pemindahan media belum dapat dicoba kembali.'}</p>}</AlertDescription></Alert>}</div>}
    {documentation.length > 0 && <section aria-label="Dokumentasi" className="grid gap-4 sm:grid-cols-2">{documentation.map((item) => <DocumentationSlot key={item.code} slot={item} canManage={editable} canRetryMove={editable} onChanged={updateDocumentation} />)}</section>}
    {scanning && <Suspense fallback={null}>
      <BarcodeScanner onResult={(text) => { setEquipment((prev) => ({ ...prev, [scanning]: uppercaseBusinessText(text) })); setScanning(null); }} onClose={() => setScanning(null)} />
    </Suspense>}
    <RevisionDialog slot={currentSlot} stage="dokumen" open={revisionOpen} onOpenChange={setRevisionOpen} onReopened={publish} />
  </PosSectionShell>;
}
