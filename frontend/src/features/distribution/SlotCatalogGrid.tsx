import { Ban, Check, Circle, Clock3, Plus, SquareCheck, TriangleAlert } from 'lucide-react';
import styles from './Distribution.module.css';
import type { SlotCatalogEntry } from './types';

type Props = {
  quota?: number;
  entries: SlotCatalogEntry[];
  onSelect: (slotNumber: number) => void;
  onCreate: (slotNumber: number) => void;
  canCreate: boolean;
  selectedNumber?: number | null;
  /** Select mode (export): existing numbers toggle in `picked`, empty ones are off. */
  picked?: ReadonlySet<number>;
  onTogglePick?: (slotNumber: number) => void;
};

const statusLabel: Record<SlotCatalogEntry['status'], string> = { open: 'Terbuka', linked: 'Proses', completed: 'Selesai', cancelled: 'Batal' };
const statusClass: Record<SlotCatalogEntry['status'], string> = { open: styles.catalogOpen, linked: styles.catalogPending, completed: styles.catalogCompleted, cancelled: styles.catalogPending };

export function SlotCatalogGrid({ quota, entries, onSelect, onCreate, canCreate, selectedNumber, picked, onTogglePick }: Props) {
  const picking = picked != null && onTogglePick != null;
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
        const needsAttention = entry.needs_recompletion === true;
        const cellClass = needsAttention || isCompletedButIncomplete ? styles.catalogCompletedWarning : statusClass[entry.status];
        const label = needsAttention ? 'Perlu diselesaikan ulang' : isCompletedButIncomplete ? `${statusLabel[entry.status]} (dokumen berkurang)` : statusLabel[entry.status];
        const StatusIcon = needsAttention || isCompletedButIncomplete ? TriangleAlert : entry.status === 'completed' ? Check : entry.status === 'linked' ? Clock3 : entry.status === 'cancelled' ? Ban : Circle;
        if (picking) {
          const on = picked.has(number);
          return <button key={number} type="button" className={`${styles.catalogCell} ${cellClass} ${on ? styles.catalogPicked : ''}`}
            aria-label={`Pilih nomor ${number} - ${label}`} aria-pressed={on} title={`Nomor ${number} · ${label}`}
            onClick={() => onTogglePick(number)}><span>{number}</span>{on ? <SquareCheck aria-hidden="true" /> : <StatusIcon aria-hidden="true" />}</button>;
        }
        return <button key={number} type="button" className={`${styles.catalogCell} ${cellClass}`}
          aria-label={`Nomor ${number} - ${label}`} aria-pressed={selectedNumber === number} title={`Nomor ${number} · ${label}`}
          onClick={() => onSelect(number)}><span>{number}</span><StatusIcon aria-hidden="true" /></button>;
      }
      if (isCreatePosition(number) && !picking) {
        return <button key={number} type="button" className={`${styles.catalogCell} ${styles.catalogNext}`}
          aria-label={`Buat Nomor ${number}`} aria-pressed={selectedNumber === number} title={`Buat Nomor ${number}`}
          disabled={!canCreate} onClick={() => onCreate(number)}><span>{number}</span><Plus aria-hidden="true" /></button>;
      }
      return <button key={number} type="button" className={styles.catalogCell} disabled aria-label={`Nomor ${number} belum tersedia`}>{number}</button>;
    })}
  </div>;
}
