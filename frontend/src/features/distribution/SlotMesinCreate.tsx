import { type FormEvent } from 'react';
import { useMutation } from '@tanstack/react-query';
import { Cog } from 'lucide-react';
import { apiRequest, ApiError } from '../../lib/api';
import type { CreateSlotInput, DataResponse, DistributionSlot, EquipmentOption } from './types';
import { PosSectionShell } from './PosSectionShell';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';

export function SlotMesinCreate({ scheduleID, slotNumber, onCreated }: {
  scheduleID: string;
  slotNumber: number;
  machineOptions: EquipmentOption[];
  hoseOptions: EquipmentOption[];
  converterOptions: EquipmentOption[];
  onCreated: (slot: DistributionSlot) => void;
}) {
  const input: CreateSlotInput = { schedule_id: scheduleID, slot_number: slotNumber };
  const create = useMutation({
    mutationFn: () => apiRequest<DataResponse<DistributionSlot>>('/api/v1/distribution/slots', { method: 'POST', body: JSON.stringify(input) }),
    onSuccess: ({ data }) => onCreated(data),
  });

  const submit = (event: FormEvent) => {
    event.preventDefault();
    create.mutate();
  };

  return <PosSectionShell label="POS Mesin" badge="POS Mesin" icon={<Cog aria-hidden="true" />} title={`Nomor bagi #${slotNumber}`} state="active">
    <form className="grid gap-4" onSubmit={submit}>
      <p className="text-sm text-muted-foreground">Buat nomor bagi untuk mulai mengambil dokumentasi mesin. Tanggal, penerima, dan data peralatan dilengkapi di POS Dokumen.</p>
      {create.isError && <Alert variant="destructive"><AlertDescription>{create.error instanceof ApiError ? create.error.message : 'Nomor bagi belum dapat dibuat.'}{create.error instanceof ApiError && create.error.code === 'slot_quota_exceeded' && ' Hubungi admin Program Setup untuk menambah kuota atau membuat jadwal tambahan.'}</AlertDescription></Alert>}
      <Button type="submit" disabled={create.isPending}>{create.isPending ? 'Menyiapkan...' : 'Mulai dokumentasi'}</Button>
    </form>
  </PosSectionShell>;
}
