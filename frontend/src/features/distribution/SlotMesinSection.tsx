import type { DistributionSlot } from './types';

export function SlotMesinSection({ slot, onChanged }: { slot: DistributionSlot; onChanged: (slot: DistributionSlot) => void }) {
  return <section aria-label="POS Mesin"><p>Slot #{slot.slot_number} — {slot.status}</p></section>;
}
