import { Cog } from 'lucide-react';
import type { DistributionSlot } from './types';
import { DocumentationSlot } from './DocumentationSlot';
import { PosSectionShell } from './PosSectionShell';

export function SlotMesinSection({ slot, onChanged }: { slot: DistributionSlot; onChanged: (slot: DistributionSlot) => void }) {
  const documentation = slot.documentation.filter((item) => item.stage === 'mesin');
  const updateDocumentation = (next: DistributionSlot['documentation'][number]) => onChanged({ ...slot, documentation: slot.documentation.map((item) => item.code === next.code ? next : item) });

  const summary: { label: string; value: string }[] = [
    { label: 'Merk/Tipe Mesin', value: slot.machine_option_code || '-' },
    { label: 'Serial Number Mesin', value: slot.machine_serial_number || '-' },
    { label: 'Merk/Spesifikasi Selang', value: slot.hose_option_code || '-' },
    { label: 'Serial Number Selang', value: slot.hose_serial_number || '-' },
    { label: 'Merk Konkit/Reducer', value: slot.converter_option_code || '-' },
    { label: 'Serial Number Konkit/Reducer', value: slot.converter_serial_number || '-' },
  ];

  return <PosSectionShell label="POS Mesin" badge="POS Mesin" icon={<Cog aria-hidden="true" />} title={`Nomor bagi #${slot.slot_number}`} state="done" status="Tercatat">
    <dl className="grid grid-cols-2 gap-x-4 gap-y-3 sm:grid-cols-3">
      {summary.map((item) => <div key={item.label} className="min-w-0">
        <dt className="text-xs text-muted-foreground">{item.label}</dt>
        <dd className="text-sm font-medium wrap-break-word">{item.value}</dd>
      </div>)}
    </dl>
    {documentation.length > 0 && <div className="grid gap-4 sm:grid-cols-2">{documentation.map((item) => <DocumentationSlot key={item.code} slot={item} onChanged={updateDocumentation} />)}</div>}
  </PosSectionShell>;
}
