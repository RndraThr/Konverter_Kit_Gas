import { FormEvent, lazy, Suspense, useState } from 'react';
import { Cog, ScanBarcode } from 'lucide-react';
import { useMutation } from '@tanstack/react-query';
import { apiRequest, ApiError } from '../../lib/api';
import type { CreateSlotInput, DataResponse, DistributionSlot, EquipmentOption } from './types';
import { PosSectionShell } from './PosSectionShell';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { uppercaseBusinessText } from '@/lib/text';

const BarcodeScanner = lazy(() => import('./BarcodeScanner'));

type SerialField = 'machine_serial_number' | 'hose_serial_number' | 'converter_serial_number';

const emptyInput = (scheduleID: string, slotNumber: number): CreateSlotInput => ({ schedule_id: scheduleID, slot_number: slotNumber, machine_option_code: '', machine_serial_number: '', hose_option_code: '', hose_serial_number: '', converter_option_code: '', converter_serial_number: '' });

// POS Mesin in "create mode": clicking an empty catalog number opens this inline form (no modal) so
// the officer records the machine on the spot. Each serial field can be filled by camera barcode
// scan. On success the parent swaps in the full slot workspace.
export function SlotMesinCreate({ scheduleID, slotNumber, machineOptions, hoseOptions, converterOptions, onCreated }: {
  scheduleID: string;
  slotNumber: number;
  machineOptions: EquipmentOption[];
  hoseOptions: EquipmentOption[];
  converterOptions: EquipmentOption[];
  onCreated: (slot: DistributionSlot) => void;
}) {
  const [input, setInput] = useState<CreateSlotInput>(() => emptyInput(scheduleID, slotNumber));
  const [scanning, setScanning] = useState<SerialField | null>(null);

  const create = useMutation({
    mutationFn: () => apiRequest<DataResponse<DistributionSlot>>('/api/v1/distribution/slots', { method: 'POST', body: JSON.stringify(input) }),
    onSuccess: ({ data }) => onCreated(data),
  });

  const submit = (event: FormEvent) => { event.preventDefault(); create.mutate(); };

  const serialField = (label: string, field: SerialField, className?: string) => <div className={`grid min-w-0 gap-2 ${className ?? ''}`}>
    <Label htmlFor={field}>{label}</Label>
    <div className="flex gap-2">
      <Input id={field} name={field} className="flex-1" value={input[field]} onChange={(event) => setInput({ ...input, [field]: uppercaseBusinessText(event.target.value) })} />
      <Button type="button" variant="outline" size="icon" aria-label={`Scan ${label}`} title="Scan barcode" onClick={() => setScanning(field)}><ScanBarcode aria-hidden="true" /></Button>
    </div>
  </div>;

  return <PosSectionShell label="POS Mesin" badge="POS Mesin" icon={<Cog aria-hidden="true" />} title={`Nomor bagi #${slotNumber}`} state="active">
    <form className="grid gap-4 sm:grid-cols-2" onSubmit={submit}>
      <div className="grid min-w-0 gap-2">
        <Label id="create-machine-label">Merk/Tipe Mesin</Label>
        <Select value={input.machine_option_code} onValueChange={(value) => setInput({ ...input, machine_option_code: value ?? '' })}>
          <SelectTrigger className="w-full" aria-labelledby="create-machine-label"><SelectValue placeholder="Pilih mesin" /></SelectTrigger>
          <SelectContent>{machineOptions.map((option) => <SelectItem key={option.code} value={option.code}>{option.brand} {option.type}</SelectItem>)}</SelectContent>
        </Select>
      </div>
      {serialField('Serial Number Mesin', 'machine_serial_number')}
      <div className="grid min-w-0 gap-2">
        <Label id="create-hose-label">Merk/Spesifikasi Selang</Label>
        <Select value={input.hose_option_code} onValueChange={(value) => setInput({ ...input, hose_option_code: value ?? '' })}>
          <SelectTrigger className="w-full" aria-labelledby="create-hose-label"><SelectValue placeholder="Pilih selang" /></SelectTrigger>
          <SelectContent>{hoseOptions.map((option) => <SelectItem key={option.code} value={option.code}>{option.brand} {option.spec}</SelectItem>)}</SelectContent>
        </Select>
      </div>
      {serialField('Serial Number Selang', 'hose_serial_number')}
      <div className="grid min-w-0 gap-2">
        <Label id="create-converter-label">Merk Konkit/Reducer</Label>
        <Select value={input.converter_option_code} onValueChange={(value) => setInput({ ...input, converter_option_code: value ?? '' })}>
          <SelectTrigger className="w-full" aria-labelledby="create-converter-label"><SelectValue placeholder="Pilih konkit/reducer" /></SelectTrigger>
          <SelectContent>{converterOptions.map((option) => <SelectItem key={option.code} value={option.code}>{option.brand}</SelectItem>)}</SelectContent>
        </Select>
      </div>
      {serialField('Serial Number Konkit/Reducer', 'converter_serial_number')}
      {create.isError && <Alert className="sm:col-span-2" variant="destructive"><AlertDescription>{create.error instanceof ApiError ? create.error.message : 'Nomor bagi belum dapat dibuat.'}{create.error instanceof ApiError && create.error.code === 'slot_quota_exceeded' && ' Hubungi admin Program Setup untuk menambah kuota atau membuat jadwal tambahan.'}</AlertDescription></Alert>}
      <Button className="sm:col-span-2" type="submit" disabled={create.isPending}>{create.isPending ? 'Menyimpan...' : 'Simpan Nomor Bagi'}</Button>
    </form>
    {scanning && <Suspense fallback={null}>
      <BarcodeScanner onResult={(text) => { setInput((prev) => ({ ...prev, [scanning]: uppercaseBusinessText(text) })); setScanning(null); }} onClose={() => setScanning(null)} />
    </Suspense>}
  </PosSectionShell>;
}
