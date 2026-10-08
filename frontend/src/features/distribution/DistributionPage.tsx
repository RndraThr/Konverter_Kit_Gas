import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { Check, Circle, Clock3, LayoutGrid, Lock, MousePointerClick } from 'lucide-react';
import { apiRequest, ApiError } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import type { DataResponse, DistributionSlot, EquipmentOption, ScheduleResponse, SlotCatalogEntry } from './types';
import { SlotMesinSection } from './SlotMesinSection';
import { SlotDokumenSection } from './SlotDokumenSection';
import { SlotPenyerahanSection } from './SlotPenyerahanSection';
import { SlotCatalogGrid } from './SlotCatalogGrid';
import { SlotMesinCreate } from './SlotMesinCreate';
import { PosSectionShell } from './PosSectionShell';
import styles from './Distribution.module.css';
import { DataState } from '@/components/DataState';
import { PageHeader } from '@/components/PageHeader';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import type { PackageTemplate } from '../programs/types';

export function DistributionPage() {
  const queryClient = useQueryClient();
  const canCreateSlot = useCan('distribution.pos_mesin');
  const [scheduleID, setScheduleID] = useState('');
  const [slot, setSlot] = useState<DistributionSlot | null>(null);
  const [creating, setCreating] = useState<number | null>(null);
  const [selectedNumber, setSelectedNumber] = useState<number | null>(null);

  const schedules = useQuery({ queryKey: ['program-setup', 'schedules'], queryFn: () => apiRequest<ScheduleResponse>('/api/v1/program-setup/schedules') });
  const packageTemplates = useQuery({ queryKey: ['program-setup', 'package-templates'], queryFn: () => apiRequest<DataResponse<PackageTemplate[]>>('/api/v1/program-setup/package-templates') });
  const catalog = useQuery({ queryKey: ['distribution', 'slot-catalog', scheduleID], queryFn: () => apiRequest<DataResponse<SlotCatalogEntry[]>>(`/api/v1/distribution/slots/catalog?schedule_id=${encodeURIComponent(scheduleID)}`), enabled: !!scheduleID });
  const selectedSchedule = schedules.data?.data.find((schedule) => schedule.id === scheduleID);
  const selectedPackageTemplate = selectedSchedule?.package_template ?? packageTemplates.data?.data.find((template) => template.id === selectedSchedule?.package_template_version_id);
  const machineOptions = ((selectedPackageTemplate?.values as { machine_options?: EquipmentOption[] } | undefined)?.machine_options) ?? [];
  const rawHoseOptions = ((selectedPackageTemplate?.values as { hose_options?: Array<EquipmentOption & { suction_brand?: string; suction_spec?: string; discharge_brand?: string; discharge_spec?: string }> } | undefined)?.hose_options) ?? [];
  const hoseOptions = rawHoseOptions.map((option) => ({
    code: option.code,
    brand: [option.suction_brand, option.discharge_brand].filter(Boolean).join(' / ') || option.brand,
    spec: [option.suction_spec, option.discharge_spec].filter(Boolean).join(' / ') || option.spec,
  }));
  const converterOptions = ((selectedPackageTemplate?.values as { converter_options?: EquipmentOption[] } | undefined)?.converter_options) ?? [];

  const search = useMutation({
    mutationFn: (slotNumber: number) => apiRequest<DataResponse<DistributionSlot>>(`/api/v1/distribution/slots/search?schedule_id=${encodeURIComponent(scheduleID)}&q=${slotNumber}`),
    onSuccess: ({ data }) => setSlot(data),
  });

  const changeSchedule = (value: string) => { setScheduleID(value); setSlot(null); setCreating(null); setSelectedNumber(null); };
  const selectExisting = (slotNumber: number) => { setSlot(null); setCreating(null); setSelectedNumber(slotNumber); search.mutate(slotNumber); };
  const startCreate = (slotNumber: number) => { setSlot(null); setCreating(slotNumber); setSelectedNumber(slotNumber); };
  const onSlotChanged = (next: DistributionSlot) => { setSlot(next); void queryClient.invalidateQueries({ queryKey: ['distribution', 'slot-catalog', scheduleID] }); };
  const onCreated = (next: DistributionSlot) => { setSlot(next); setCreating(null); void queryClient.invalidateQueries({ queryKey: ['distribution'] }); };

  return <div className={`page ${styles.page}`}>
    <PageHeader title="Pendistribusian" description="Pilih nomor bagi dari katalog, catat mesin, hubungkan penerima, dan selesaikan serah terima." context={selectedSchedule ? <span className={styles.context}><strong>{selectedSchedule.regency?.document_code}</strong>{selectedSchedule.name}</span> : undefined} />
    <section className="max-w-md space-y-1.5 py-5" aria-label="Pilih jadwal distribusi">
      <Label id="distribution-schedule-label">Jadwal distribusi</Label>
      <Select value={scheduleID} onValueChange={(value) => changeSchedule(value ?? '')}>
        <SelectTrigger className="w-full" aria-labelledby="distribution-schedule-label"><SelectValue placeholder="Pilih kabupaten dan jadwal" /></SelectTrigger>
        <SelectContent>{schedules.data?.data.filter((schedule) => schedule.status === 'active').map((schedule) => <SelectItem key={schedule.id} value={schedule.id}>{schedule.regency?.name} / {schedule.name}</SelectItem>)}</SelectContent>
      </Select>
    </section>
    {scheduleID && <div className={styles.workspace}>
      <aside className={styles.catalogPanel} aria-label="Katalog nomor bagi">
        <div className={styles.catalogHeader}>
          <div><span className={styles.catalogEyebrow}><LayoutGrid aria-hidden="true" />Katalog</span><strong>Nomor bagi</strong></div>
          <span className={styles.catalogCount}>{catalog.data?.data.length ?? 0}{selectedSchedule?.slot_quota ? ` / ${selectedSchedule.slot_quota}` : ''}</span>
        </div>
        <div className={styles.catalogLegend} aria-label="Keterangan status">
          <span><Circle aria-hidden="true" />Terbuka</span><span><Clock3 aria-hidden="true" />Proses</span><span><Check aria-hidden="true" />Selesai</span>
        </div>
        <div className={styles.catalogScroll}>
          <SlotCatalogGrid quota={selectedSchedule?.slot_quota} entries={catalog.data?.data ?? []} selectedNumber={selectedNumber} onSelect={selectExisting} onCreate={startCreate} canCreate={canCreateSlot} />
        </div>
      </aside>
      <section className={styles.workArea} aria-label="Area kerja pendistribusian">
        {search.isPending && <DataState kind="loading" title="Membuka nomor bagi" description={`Menyiapkan data nomor ${selectedNumber ?? ''}.`} />}
        {search.isError && <DataState kind="error" title="Nomor bagi tidak ditemukan" description={search.error instanceof ApiError ? search.error.message : 'Muat ulang halaman, lalu coba lagi.'} />}
        {slot && <div className={styles.slotSections}>
          <SlotMesinSection slot={slot} machineOptions={machineOptions} converterOptions={converterOptions} hoseOptions={hoseOptions} onChanged={onSlotChanged} />
          <SlotDokumenSection slot={slot} machineOptions={machineOptions} converterOptions={converterOptions} hoseOptions={hoseOptions} onChanged={onSlotChanged} />
          <SlotPenyerahanSection slot={slot} onChanged={onSlotChanged} />
        </div>}
        {!slot && creating !== null && <div className={styles.slotSections}>
          <SlotMesinCreate scheduleID={scheduleID} slotNumber={creating} machineOptions={machineOptions} hoseOptions={hoseOptions} converterOptions={converterOptions} onCreated={onCreated} />
          <PosSectionShell label="POS Dokumen" badge="POS Dokumen" icon={<Lock aria-hidden="true" />} title="Menunggu nomor bagi disimpan" state="locked" />
          <PosSectionShell label="POS Penyerahan" badge="POS Penyerahan" icon={<Lock aria-hidden="true" />} title="Menunggu dokumen selesai" state="locked" />
        </div>}
        {!slot && creating === null && !search.isPending && !search.isError && <div className={styles.emptyWorkspace}>
          <span><MousePointerClick aria-hidden="true" /></span>
          <strong>Pilih nomor bagi untuk mulai bekerja</strong>
          <p>Pilih nomor yang sudah tersedia atau buat nomor baru dari katalog di sebelah kiri.</p>
        </div>}
      </section>
    </div>}
  </div>;
}
