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
  const nextNumber = entries.length + 1;
  const total = quota ?? nextNumber;
  const numbers = Array.from({ length: total }, (_, index) => index + 1);

  return <div className={styles.catalogGrid} role="group" aria-label="Katalog nomor bagi">
    {numbers.map((number) => {
      const entry = byNumber.get(number);
      if (entry) {
        return <button key={number} type="button" className={`${styles.catalogCell} ${statusClass[entry.status]}`}
          onClick={() => onSelect(number)}>{`Nomor ${number} - ${statusLabel[entry.status]}`}</button>;
      }
      if (number === nextNumber) {
        return <button key={number} type="button" className={`${styles.catalogCell} ${styles.catalogNext}`}
          disabled={!canCreate} onClick={onCreateNext}>{`Buat Nomor ${number}`}</button>;
      }
      return <button key={number} type="button" className={styles.catalogCell} disabled aria-label={`Nomor ${number} belum tersedia`}>{number}</button>;
    })}
  </div>;
}
