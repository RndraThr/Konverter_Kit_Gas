import styles from './Distribution.module.css';
import type { SlotCatalogEntry } from './types';

type Props = {
  quota?: number;
  entries: SlotCatalogEntry[];
  onSelect: (slotNumber: number) => void;
  onCreate: (slotNumber: number) => void;
  canCreate: boolean;
};

const statusLabel: Record<SlotCatalogEntry['status'], string> = { open: 'Terbuka', linked: 'Proses', completed: 'Selesai', cancelled: 'Batal' };
const statusClass: Record<SlotCatalogEntry['status'], string> = { open: styles.catalogOpen, linked: styles.catalogPending, completed: styles.catalogCompleted, cancelled: styles.catalogPending };

export function SlotCatalogGrid({ quota, entries, onSelect, onCreate, canCreate }: Props) {
  const byNumber = new Map(entries.map((entry) => [entry.slot_number, entry]));
  const hasQuota = quota != null && quota > 0;
  // Slot numbers are allocated contiguously from 1 and slots are only ever cancelled, never
  // hard-deleted, so entries.length + 1 is the next free number in sequential mode.
  const nextNumber = entries.length + 1;
  // With a quota, show every number up to it; without one, only up to the next free number.
  // Never render fewer boxes than there are real entries, even if quota was lowered below that count.
  const total = Math.max(hasQuota ? quota : nextNumber, entries.length);
  const numbers = Array.from({ length: total }, (_, index) => index + 1);
  // An empty number is a valid creation target when it is within quota (or the next free number
  // without a quota). Permission (canCreate) only controls whether the button is enabled, so a
  // read-only viewer still sees "Buat Nomor N" rather than a misleading "belum tersedia".
  const isCreatePosition = (number: number) => hasQuota ? number <= quota : number === nextNumber;

  return <div className={styles.catalogGrid} role="group" aria-label="Katalog nomor bagi">
    {numbers.map((number) => {
      const entry = byNumber.get(number);
      if (entry) {
        const isCompletedButIncomplete = entry.status === 'completed' && !entry.documentation_complete;
        const cellClass = isCompletedButIncomplete ? styles.catalogCompletedWarning : statusClass[entry.status];
        const label = isCompletedButIncomplete ? `${statusLabel[entry.status]} (dokumen berkurang)` : statusLabel[entry.status];
        return <button key={number} type="button" className={`${styles.catalogCell} ${cellClass}`}
          onClick={() => onSelect(number)}>{`Nomor ${number} - ${label}`}</button>;
      }
      if (isCreatePosition(number)) {
        return <button key={number} type="button" className={`${styles.catalogCell} ${styles.catalogNext}`}
          disabled={!canCreate} onClick={() => onCreate(number)}>{`Buat Nomor ${number}`}</button>;
      }
      return <button key={number} type="button" className={styles.catalogCell} disabled aria-label={`Nomor ${number} belum tersedia`}>{number}</button>;
    })}
  </div>;
}
