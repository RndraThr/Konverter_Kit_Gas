import styles from './Distribution.module.css';
import type { SlotCatalogEntry } from './types';

type Props = {
  quota?: number;
  entries: SlotCatalogEntry[];
  onSelect: (slotNumber: number) => void;
  onCreateNext: () => void;
  canCreate: boolean;
};

const statusLabel: Record<SlotCatalogEntry['status'], string> = { open: 'Terbuka', linked: 'Proses', completed: 'Selesai', cancelled: 'Batal' };
const statusClass: Record<SlotCatalogEntry['status'], string> = { open: styles.catalogOpen, linked: styles.catalogPending, completed: styles.catalogCompleted, cancelled: styles.catalogPending };

export function SlotCatalogGrid({ quota, entries, onSelect, onCreateNext, canCreate }: Props) {
  const byNumber = new Map(entries.map((entry) => [entry.slot_number, entry]));
  // Slot numbers are allocated contiguously from 1 (COALESCE(max(slot_number),0)+1) and slots are
  // only ever cancelled, never hard-deleted, so entries.length + 1 is safe as "the next free number".
  const nextNumber = entries.length + 1;
  // Never render fewer boxes than there are real entries, even if quota was lowered below that count.
  const total = Math.max(quota ?? nextNumber, entries.length);
  const numbers = Array.from({ length: total }, (_, index) => index + 1);

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
      if (number === nextNumber) {
        return <button key={number} type="button" className={`${styles.catalogCell} ${styles.catalogNext}`}
          disabled={!canCreate} onClick={onCreateNext}>{`Buat Nomor ${number}`}</button>;
      }
      return <button key={number} type="button" className={styles.catalogCell} disabled aria-label={`Nomor ${number} belum tersedia`}>{number}</button>;
    })}
  </div>;
}
