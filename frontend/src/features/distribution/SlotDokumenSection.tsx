import type { DistributionSlot } from './types';

export function SlotDokumenSection({ slot, onChanged }: { slot: DistributionSlot; onChanged: (slot: DistributionSlot) => void }) {
  return <section aria-label="POS Dokumen"><p>Slot #{slot.slot_number} — {slot.status}</p></section>;
}
