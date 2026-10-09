import { lazy, Suspense, useEffect, useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { Cog, Pencil, ScanBarcode } from 'lucide-react';
import { apiRequest, ApiError } from '../../lib/api';
import type { DataResponse, DistributionSlot, EquipmentOption } from './types';
import { DocumentationSlot } from './DocumentationSlot';
import { PosSectionShell } from './PosSectionShell';
import { RevisionDialog } from './RevisionDialog';
import { useCan } from '../../lib/permissions';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { uppercaseBusinessText } from '@/lib/text';

const BarcodeScanner = lazy(() => import('./BarcodeScanner'));
type SerialField = 'machine_serial_number' | 'converter_serial_number';

type SlotMesinSectionProps = {
  slot: DistributionSlot;
  machineOptions: EquipmentOption[];
  converterOptions: EquipmentOption[];
  hoseOptions: EquipmentOption[];
  onChanged: (slot: DistributionSlot) => void;
};

export function SlotMesinSection({ slot, onChanged }: SlotMesinSectionProps) {
  const canManage = useCan('distribution.pos_mesin');
  const [currentSlot, setCurrentSlot] = useState(slot);
  const [revisionOpen, setRevisionOpen] = useState(false);
  const [serials, setSerials] = useState({ machine_serial_number: slot.machine_serial_number ?? '', converter_serial_number: slot.converter_serial_number ?? '' });
  const [scanning, setScanning] = useState<SerialField | null>(null);
  useEffect(() => {
    setCurrentSlot(slot);
    setSerials({ machine_serial_number: slot.machine_serial_number ?? '', converter_serial_number: slot.converter_serial_number ?? '' });
  }, [slot]);

  const documentation = currentSlot.documentation.filter((item) => item.stage === 'mesin');
  const editable = canManage && currentSlot.status !== 'completed';
  const updateDocumentation = (next: DistributionSlot['documentation'][number]) => {
    const updated = { ...currentSlot, documentation: currentSlot.documentation.map((item) => item.code === next.code ? next : item) };
    setCurrentSlot(updated);
    onChanged(updated);
  };
  const handleReopened = (reopened: DistributionSlot) => {
    setCurrentSlot(reopened);
    onChanged(reopened);
  };
  const serialUpdate = useMutation({
    mutationFn: () => apiRequest<DataResponse<DistributionSlot>>(`/api/v1/distribution/slots/${currentSlot.slot_number}/equipment-serials?schedule_id=${encodeURIComponent(currentSlot.schedule_id)}`, { method: 'PATCH', body: JSON.stringify(serials) }),
    onSuccess: ({ data }) => {
      setCurrentSlot(data);
      setSerials({ machine_serial_number: data.machine_serial_number ?? '', converter_serial_number: data.converter_serial_number ?? '' });
      onChanged(data);
    },
  });
  const serialField = (label: string, field: SerialField) => {
    const id = `mesin-${field}-${currentSlot.id}`;
    return <div className="grid min-w-0 gap-2">
      <Label htmlFor={id}>{label}</Label>
      <div className="flex gap-2">
        <Input id={id} name={field} className="flex-1" disabled={!editable} value={serials[field]} onChange={(event) => setSerials((current) => ({ ...current, [field]: uppercaseBusinessText(event.target.value) }))} />
        {editable && <Button type="button" variant="outline" size="icon" className="size-11 shrink-0" aria-label={`Scan ${label}`} title="Scan barcode" onClick={() => setScanning(field)}><ScanBarcode aria-hidden="true" /></Button>}
      </div>
    </div>;
  };

  return <PosSectionShell
    label="POS Mesin"
    badge="POS Mesin"
    icon={<Cog aria-hidden="true" />}
    title={`Nomor bagi #${currentSlot.slot_number}`}
    state={currentSlot.status === 'completed' ? 'done' : 'active'}
    status={currentSlot.status === 'completed' ? 'Selesai' : 'Dokumentasi'}
  >
    {canManage && currentSlot.status === 'completed' && <div className="flex justify-end">
      <Button type="button" variant="outline" onClick={() => setRevisionOpen(true)}><Pencil aria-hidden="true" />Buka revisi POS Mesin</Button>
    </div>}

    <section aria-labelledby={`machine-serials-${currentSlot.id}`} className="space-y-3 rounded-lg border p-4">
      <div className="space-y-1">
        <h4 id={`machine-serials-${currentSlot.id}`} className="font-medium">Nomor seri dari box</h4>
        <p className="text-sm text-muted-foreground">Scan kartu barcode di dalam box. Jika belum tersedia, nomor seri dapat dilengkapi di POS Dokumen.</p>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        {serialField('Serial Number Mesin', 'machine_serial_number')}
        {serialField('Serial Number Konkit/Reducer', 'converter_serial_number')}
      </div>
      {editable && <Button type="button" disabled={serialUpdate.isPending} onClick={() => serialUpdate.mutate()}>{serialUpdate.isPending ? 'Menyimpan...' : 'Simpan nomor seri'}</Button>}
      {serialUpdate.isSuccess && <p role="status" className="text-sm text-primary">Nomor seri tersimpan.</p>}
      {serialUpdate.isError && <p role="alert" className="text-sm text-destructive">{serialUpdate.error instanceof ApiError ? serialUpdate.error.message : 'Nomor seri belum dapat disimpan.'}</p>}
    </section>

    {documentation.length > 0
      ? <div className="grid gap-4 sm:grid-cols-2">{documentation.map((item) => <DocumentationSlot key={item.code} slot={item} canManage={editable} onChanged={updateDocumentation} />)}</div>
      : <p className="text-sm text-muted-foreground">Tidak ada slot dokumentasi mesin pada template jadwal ini.</p>}

    <RevisionDialog slot={currentSlot} stage="mesin" open={revisionOpen} onOpenChange={setRevisionOpen} onReopened={handleReopened} />
    {scanning && <Suspense fallback={null}><BarcodeScanner onResult={(text) => { setSerials((current) => ({ ...current, [scanning]: uppercaseBusinessText(text) })); setScanning(null); }} onClose={() => setScanning(null)} /></Suspense>}
  </PosSectionShell>;
}
