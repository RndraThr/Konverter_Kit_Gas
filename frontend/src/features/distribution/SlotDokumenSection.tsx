import { FormEvent, KeyboardEvent, useEffect, useState } from 'react';
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
import { uppercaseBusinessText } from '@/lib/text';

const emptyLinkInput = (scheduleID: string, slotNumber: number): LinkSlotInput => ({ schedule_id: scheduleID, slot_number: slotNumber, nik: '', address: '', village: '', district: '', phone_number: '', sector_identifier: '' });

export function SlotDokumenSection({ slot, onChanged }: { slot: DistributionSlot; onChanged: (slot: DistributionSlot) => void }) {
  const canLink = useCan('distribution.pos_dokumen');
  const documentation = slot.documentation.filter((item) => item.stage === 'dokumen');
  const updateDocumentation = (next: DistributionSlot['documentation'][number]) => onChanged({ ...slot, documentation: slot.documentation.map((item) => item.code === next.code ? next : item) });

  const [nik, setNik] = useState('');
  const [candidate, setCandidate] = useState<CandidateMatch | null>(null);
  const [suggestions, setSuggestions] = useState<CandidateMatch[]>([]);
  const [activeSuggestion, setActiveSuggestion] = useState(-1);
  const [isSearching, setIsSearching] = useState(false);
  const [hasSearched, setHasSearched] = useState(false);
  const [suggestionError, setSuggestionError] = useState<Error | null>(null);
  const [linkInput, setLinkInput] = useState<LinkSlotInput>(() => emptyLinkInput(slot.schedule_id, slot.slot_number));

  useEffect(() => {
    if (nik.length < 4 || candidate?.nik === nik) {
      setSuggestions([]);
      setActiveSuggestion(-1);
      setIsSearching(false);
      setHasSearched(false);
      setSuggestionError(null);
      return;
    }

    let cancelled = false;
    setIsSearching(true);
    setSuggestionError(null);
    const timer = window.setTimeout(() => {
      void apiRequest<DataResponse<CandidateMatch[]>>(`/api/v1/distribution/candidate-suggestions?schedule_id=${encodeURIComponent(slot.schedule_id)}&nik_prefix=${encodeURIComponent(nik)}`)
        .then(({ data }) => {
          if (cancelled) return;
          setSuggestions(data);
          setActiveSuggestion(-1);
          setHasSearched(true);
        })
        .catch((error: unknown) => {
          if (cancelled) return;
          setSuggestions([]);
          setSuggestionError(error instanceof Error ? error : new Error('Pencarian penerima gagal.'));
          setHasSearched(true);
        })
        .finally(() => {
          if (!cancelled) setIsSearching(false);
        });
    }, 300);

    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [candidate?.nik, nik, slot.schedule_id]);

  const link = useMutation({
    mutationFn: () => apiRequest<DataResponse<DistributionSlot>>(`/api/v1/distribution/slots/${slot.slot_number}/link?schedule_id=${encodeURIComponent(slot.schedule_id)}`, { method: 'POST', body: JSON.stringify(linkInput) }),
    onSuccess: ({ data }) => onChanged(data),
  });

  const submitLink = (event: FormEvent) => { event.preventDefault(); link.mutate(); };
  const selectCandidate = (next: CandidateMatch) => {
    setNik(next.nik);
    setCandidate(next);
    setSuggestions([]);
    setActiveSuggestion(-1);
    setLinkInput({ schedule_id: slot.schedule_id, slot_number: slot.slot_number, nik: next.nik, address: next.address, village: next.village, district: next.district, phone_number: next.phone_number, sector_identifier: next.sector_identifier });
  };
  const handleNIKKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Escape') {
      setSuggestions([]);
      setActiveSuggestion(-1);
      return;
    }
    if (suggestions.length === 0) return;
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      setActiveSuggestion((current) => (current + 1) % suggestions.length);
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      setActiveSuggestion((current) => current <= 0 ? suggestions.length - 1 : current - 1);
    } else if (event.key === 'Enter' && activeSuggestion >= 0) {
      event.preventDefault();
      selectCandidate(suggestions[activeSuggestion]);
    }
  };

  if (slot.status === 'open') {
    if (!canLink) {
      return <PosSectionShell label="POS Dokumen" badge="POS Dokumen" icon={<Lock aria-hidden="true" />} title="Menunggu penerima" state="locked" />;
    }
    return <PosSectionShell label="POS Dokumen" badge="POS Dokumen" icon={<UserCheck aria-hidden="true" />} title="Hubungkan penerima" state="active">
      <div className="relative">
        <FormField
          label="NIK Penerima"
          name="nik"
          maxLength={16}
          inputMode="numeric"
          autoComplete="off"
          role="combobox"
          aria-autocomplete="list"
          aria-expanded={suggestions.length > 0}
          aria-controls={`nik-suggestions-${slot.id}`}
          aria-activedescendant={activeSuggestion >= 0 ? `nik-suggestion-${suggestions[activeSuggestion]?.allocation_id}` : undefined}
          value={nik}
          hint={nik.length < 4 ? 'Ketik minimal 4 digit NIK.' : isSearching ? 'Mencari penerima...' : undefined}
          onKeyDown={handleNIKKeyDown}
          onChange={(event) => {
            setNik(event.target.value.replace(/\D/g, ''));
            setCandidate(null);
            setLinkInput(emptyLinkInput(slot.schedule_id, slot.slot_number));
          }}
        />
        {suggestions.length > 0 && <div id={`nik-suggestions-${slot.id}`} role="listbox" aria-label="Pilihan penerima" className="absolute z-20 mt-1 max-h-64 w-full overflow-y-auto rounded-lg border bg-popover p-1 text-popover-foreground shadow-md">
          {suggestions.map((item, index) => <button
            id={`nik-suggestion-${item.allocation_id}`}
            key={item.allocation_id}
            type="button"
            role="option"
            aria-label={`${item.nik} ${item.full_name}`}
            aria-selected={index === activeSuggestion}
            className="flex min-h-11 w-full flex-col items-start rounded-md px-3 py-2 text-left hover:bg-muted focus:bg-muted focus:outline-none aria-selected:bg-muted"
            onMouseEnter={() => setActiveSuggestion(index)}
            onClick={() => selectCandidate(item)}
          >
            <span className="text-sm font-medium">{item.nik}</span>
            <span className="text-xs text-muted-foreground">{item.full_name}</span>
          </button>)}
        </div>}
      </div>
      {nik.length >= 4 && hasSearched && !isSearching && suggestions.length === 0 && !candidate && !suggestionError && <p className="text-sm text-muted-foreground" role="status">Penerima dengan awalan NIK tersebut tidak ditemukan.</p>}
      {suggestionError && <Alert variant="destructive"><AlertDescription>{suggestionError instanceof ApiError ? suggestionError.message : 'Pencarian penerima belum dapat dilakukan.'}</AlertDescription></Alert>}
      {candidate && <form className="grid gap-3 sm:grid-cols-2" onSubmit={submitLink}>
        <FormField className="sm:col-span-2" label="Nama" name="candidate_full_name" value={candidate.full_name} disabled onChange={() => {}} />
        <FormField label={candidate.program_type === 'farmer' ? 'Nomor kartu petani' : 'Nomor KUSUKA'} name="sector_identifier" value={linkInput.sector_identifier} onChange={(event) => setLinkInput({ ...linkInput, sector_identifier: uppercaseBusinessText(event.target.value) })} />
        <FormField label="Nomor telepon" name="phone_number" value={linkInput.phone_number} onChange={(event) => setLinkInput({ ...linkInput, phone_number: event.target.value })} />
        <FormField className="sm:col-span-2" label="Alamat" name="address" value={linkInput.address} onChange={(event) => setLinkInput({ ...linkInput, address: uppercaseBusinessText(event.target.value) })} />
        <FormField label="Desa/kelurahan" name="village" value={linkInput.village} onChange={(event) => setLinkInput({ ...linkInput, village: uppercaseBusinessText(event.target.value) })} />
        <FormField label="Kecamatan" name="district" value={linkInput.district} onChange={(event) => setLinkInput({ ...linkInput, district: uppercaseBusinessText(event.target.value) })} />
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
