import { useEffect, useState } from 'react';
import { Cog, Pencil } from 'lucide-react';
import type { DistributionSlot, EquipmentOption } from './types';
import { DocumentationSlot } from './DocumentationSlot';
import { PosSectionShell } from './PosSectionShell';
import { RevisionDialog } from './RevisionDialog';
import { useCan } from '../../lib/permissions';
import { Button } from '@/components/ui/button';

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
  useEffect(() => setCurrentSlot(slot), [slot]);

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

    {documentation.length > 0
      ? <div className="grid gap-4 sm:grid-cols-2">{documentation.map((item) => <DocumentationSlot key={item.code} slot={item} canManage={editable} onChanged={updateDocumentation} />)}</div>
      : <p className="text-sm text-muted-foreground">Tidak ada slot dokumentasi mesin pada template jadwal ini.</p>}

    <RevisionDialog slot={currentSlot} stage="mesin" open={revisionOpen} onOpenChange={setRevisionOpen} onReopened={handleReopened} />
  </PosSectionShell>;
}
