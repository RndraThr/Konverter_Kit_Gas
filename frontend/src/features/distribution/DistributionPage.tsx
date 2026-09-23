import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { FormEvent, useState } from 'react';
import { Search } from 'lucide-react';
import { apiRequest, ApiError } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import type { CreateSlotInput, DataResponse, DistributionSlot, EquipmentOption, ScheduleResponse } from './types';
import { SlotMesinSection } from './SlotMesinSection';
import { SlotDokumenSection } from './SlotDokumenSection';
import { SlotPenyerahanSection } from './SlotPenyerahanSection';
import styles from './Distribution.module.css';
import { DataState } from '@/components/DataState';
import { PageHeader } from '@/components/PageHeader';
import { FormField } from '@/components/FormField';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';

const emptyCreateInput = (scheduleID: string): CreateSlotInput => ({ schedule_id: scheduleID, machine_option_code: '', machine_serial_number: '', hose_option_code: '', hose_serial_number: '', converter_serial_number: '' });

export function DistributionPage() {
  const queryClient = useQueryClient();
  const canCreateSlot = useCan('distribution.pos_mesin');
  const [scheduleID, setScheduleID] = useState('');
  const [query, setQuery] = useState('');
  const [slot, setSlot] = useState<DistributionSlot | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [createInput, setCreateInput] = useState<CreateSlotInput>(() => emptyCreateInput(''));

  const schedules = useQuery({ queryKey: ['program-setup', 'schedules'], queryFn: () => apiRequest<ScheduleResponse>('/api/v1/program-setup/schedules') });
  const selectedSchedule = schedules.data?.data.find((schedule) => schedule.id === scheduleID);
  const machineOptions = ((selectedSchedule?.package_template?.values as { machine_options?: EquipmentOption[] } | undefined)?.machine_options) ?? [];
  const hoseOptions = ((selectedSchedule?.package_template?.values as { hose_options?: EquipmentOption[] } | undefined)?.hose_options) ?? [];

  const search = useMutation({
    mutationFn: () => apiRequest<DataResponse<DistributionSlot>>(`/api/v1/distribution/slots/search?schedule_id=${encodeURIComponent(scheduleID)}&q=${encodeURIComponent(query)}`),
    onSuccess: ({ data }) => setSlot(data),
  });

  const create = useMutation({
    mutationFn: () => apiRequest<DataResponse<DistributionSlot>>('/api/v1/distribution/slots', { method: 'POST', body: JSON.stringify(createInput) }),
    onSuccess: ({ data }) => { setSlot(data); setCreateOpen(false); void queryClient.invalidateQueries({ queryKey: ['distribution'] }); },
  });

  const changeSchedule = (value: string) => { setScheduleID(value); setQuery(''); setSlot(null); setCreateInput(emptyCreateInput(value)); };
  const submitSearch = (event: FormEvent) => { event.preventDefault(); setSlot(null); search.mutate(); };
  const openCreate = () => { setCreateInput(emptyCreateInput(scheduleID)); setCreateOpen(true); };
  const submitCreate = (event: FormEvent) => { event.preventDefault(); create.mutate(); };
  const onSlotChanged = (next: DistributionSlot) => setSlot(next);

  return <div className={`page ${styles.page}`}>
    <PageHeader title="Pendistribusian" description="Tandai mesin, hubungkan penerima, dan selesaikan serah terima dalam satu halaman." context={selectedSchedule ? <span className={styles.context}><strong>{selectedSchedule.regency?.document_code}</strong>{selectedSchedule.name}</span> : undefined} />
    <section className={styles.lookup} aria-label="Cari atau buat nomor bagi">
      <div className={styles.scheduleField}>
        <Label id="distribution-schedule-label">Jadwal distribusi</Label>
        <Select value={scheduleID} onValueChange={(value) => changeSchedule(value ?? '')}>
          <SelectTrigger className="w-full" aria-labelledby="distribution-schedule-label"><SelectValue placeholder="Pilih kabupaten dan jadwal" /></SelectTrigger>
          <SelectContent>{schedules.data?.data.filter((schedule) => schedule.status === 'active').map((schedule) => <SelectItem key={schedule.id} value={schedule.id}>{schedule.regency?.name} / {schedule.name}</SelectItem>)}</SelectContent>
        </Select>
      </div>
      <form className={styles.searchArea} onSubmit={submitSearch}>
        <div className={styles.searchField}>
          <Search />
          <input aria-label="Nomor bagi atau NIK" placeholder="Nomor bagi atau NIK" disabled={!scheduleID} value={query} onChange={(event) => setQuery(event.target.value)} />
        </div>
        <div className={styles.lookupActions}>
          <Button type="submit" disabled={!scheduleID || !query.trim() || search.isPending}>{search.isPending ? 'Mencari...' : 'Cari'}</Button>
          {canCreateSlot && <Button type="button" variant="outline" disabled={!scheduleID} onClick={openCreate}>Buat Slot Mesin Baru</Button>}
        </div>
      </form>
    </section>
    {search.isError && <DataState kind="error" title="Nomor bagi tidak ditemukan" description={search.error instanceof ApiError ? search.error.message : 'Periksa nomor bagi atau NIK, lalu coba lagi.'} />}
    {slot && <div className={styles.slotSections}>
      <SlotMesinSection slot={slot} onChanged={onSlotChanged} />
      <SlotDokumenSection slot={slot} onChanged={onSlotChanged} />
      <SlotPenyerahanSection slot={slot} onChanged={onSlotChanged} />
    </div>}

    <Dialog open={createOpen} onOpenChange={setCreateOpen}><DialogContent aria-label="Buat Slot Mesin Baru" className="max-h-[calc(100dvh-2rem)] max-w-2xl overflow-y-auto p-0">
      <form onSubmit={submitCreate}>
        <DialogHeader className="border-b p-5"><DialogTitle>Buat Slot Mesin Baru</DialogTitle><DialogDescription>Catat perlengkapan yang dipasang pada unit mesin sebelum penerima diketahui.</DialogDescription></DialogHeader>
        <div className="grid gap-4 p-5 sm:grid-cols-2">
          <div className="grid min-w-0 gap-2"><Label id="create-machine-label">Merk/Tipe Mesin</Label><Select value={createInput.machine_option_code} onValueChange={(value) => setCreateInput({ ...createInput, machine_option_code: value ?? '' })}><SelectTrigger className="w-full" aria-labelledby="create-machine-label"><SelectValue placeholder="Pilih mesin" /></SelectTrigger><SelectContent>{machineOptions.map((option) => <SelectItem key={option.code} value={option.code}>{option.brand} {option.type}</SelectItem>)}</SelectContent></Select></div>
          <FormField label="Serial Number Mesin" name="machine_serial_number" value={createInput.machine_serial_number} onChange={(event) => setCreateInput({ ...createInput, machine_serial_number: event.target.value })} />
          <div className="grid min-w-0 gap-2"><Label id="create-hose-label">Merk/Spesifikasi Selang</Label><Select value={createInput.hose_option_code} onValueChange={(value) => setCreateInput({ ...createInput, hose_option_code: value ?? '' })}><SelectTrigger className="w-full" aria-labelledby="create-hose-label"><SelectValue placeholder="Pilih selang" /></SelectTrigger><SelectContent>{hoseOptions.map((option) => <SelectItem key={option.code} value={option.code}>{option.brand} {option.spec}</SelectItem>)}</SelectContent></Select></div>
          <FormField label="Serial Number Selang" name="hose_serial_number" value={createInput.hose_serial_number} onChange={(event) => setCreateInput({ ...createInput, hose_serial_number: event.target.value })} />
          <FormField className="sm:col-span-2" label="Serial Number Konkit/Reducer" name="converter_serial_number" value={createInput.converter_serial_number} onChange={(event) => setCreateInput({ ...createInput, converter_serial_number: event.target.value })} />
          {create.isError && <Alert className="sm:col-span-2" variant="destructive"><AlertDescription>{create.error instanceof ApiError ? create.error.message : 'Slot belum dapat dibuat.'}</AlertDescription></Alert>}
        </div>
        <DialogFooter className="mx-0 mb-0"><DialogClose render={<Button variant="outline" type="button" />}>Batal</DialogClose><Button disabled={create.isPending} type="submit">{create.isPending ? 'Membuat...' : 'Buat Slot'}</Button></DialogFooter>
      </form>
    </DialogContent></Dialog>
  </div>;
}
