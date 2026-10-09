import type { RegencyAsset } from './mapRegions';

/** Keep the large boundary file in a lazy chunk while giving the page a testable loading seam. */
export async function loadRegencyAsset(): Promise<RegencyAsset> {
  const module = await import('./data/indonesia-regencies.json');
  return (module as unknown as { default?: RegencyAsset }).default ?? (module as unknown as RegencyAsset);
}
