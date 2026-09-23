import { CheckCircle2, Cog } from 'lucide-react';
import type { DistributionSlot } from './types';
import { DocumentationSlot } from './DocumentationSlot';
import styles from './Distribution.module.css';
import { Badge } from '@/components/ui/badge';

export function SlotMesinSection({ slot, onChanged }: { slot: DistributionSlot; onChanged: (slot: DistributionSlot) => void }) {
  const documentation = slot.documentation.filter((item) => item.stage === 'mesin');
  const updateDocumentation = (next: DistributionSlot['documentation'][number]) => onChanged({ ...slot, documentation: slot.documentation.map((item) => item.code === next.code ? next : item) });

  return <section className={styles.slotSection} data-state="done" aria-label="POS Mesin">
    <header className={styles.slotSectionHeader}>
      <div><Badge variant="outline">POS Mesin</Badge><h3><Cog aria-hidden="true" />Nomor bagi #{slot.slot_number}</h3></div>
      <span className={styles.slotSectionStatus}><CheckCircle2 aria-hidden="true" />Tercatat</span>
    </header>
    <dl className={styles.slotSummaryList}>
      <div><dt>Merk/Tipe Mesin</dt><dd>{slot.machine_option_code || '-'}</dd></div>
      <div><dt>Serial Number Mesin</dt><dd>{slot.machine_serial_number || '-'}</dd></div>
      <div><dt>Merk/Spesifikasi Selang</dt><dd>{slot.hose_option_code || '-'}</dd></div>
      <div><dt>Serial Number Selang</dt><dd>{slot.hose_serial_number || '-'}</dd></div>
      <div><dt>Serial Number Konkit/Reducer</dt><dd>{slot.converter_serial_number || '-'}</dd></div>
    </dl>
    <div className={styles.sectionDocumentation}>{documentation.map((item) => <DocumentationSlot key={item.code} slot={item} onChanged={updateDocumentation} />)}</div>
  </section>;
}
