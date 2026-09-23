import type { DistributionSlot } from './types';

export function SlotPenyerahanSection({ slot, onChanged }: { slot: DistributionSlot; onChanged: (slot: DistributionSlot) => void }) {
  return <section aria-label="POS Penyerahan"><p>Slot #{slot.slot_number} — {slot.status}</p></section>;
}
