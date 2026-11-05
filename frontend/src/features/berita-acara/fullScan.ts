// Pindai lengkap: seluruh BA dicetak, ditandatangani dan diparaf basah, lalu
// semua halamannya dipindai (atau PDF hasil pindai diunggah). Selama ada
// pindaian, pindaian itulah preview dan PDF final BA; tanpa pindaian BA
// dibuat sistem (tanda tangan digital atau halaman tanda tangan basah).

export type FullScanStatus = {
  supported: boolean;
  code?: string;
  /** Jumlah halaman BA yang dicetak; pindaian harus sama. */
  page_count?: number;
  scan?: { uploaded_at: string; page_count: number; valid: boolean };
};

/** Kunci query memuat 'preview' agar ikut dimuat ulang setiap preview BA berubah. */
export const fullScanKey = (scheduleID: string, document: string, date: string) => ['bast', 'full-scan', 'preview', scheduleID, document, date];

/** document "closing-titik-serah@<id lokasi>" memilih titik serah BA Serah Terima. */
export function fullScanPath(scheduleID: string, document: string, date: string, suffix = '') {
  const [base, locationID] = document.split('@');
  const query = new URLSearchParams({ schedule_id: scheduleID, document: base, date });
  if (locationID) query.set('location_id', locationID);
  return `/api/v1/bast/full-scan${suffix ? `/${suffix}` : ''}?${query.toString()}`;
}

/** BA yang mendukung pindai lengkap (sama dengan server). */
export const fullScanDocuments = new Set(['dp3', 'rekap-harian', 'closing-titik-serah', 'closing-kabupaten', 'servis-berkala', 'tkdn', 'ba-perorangan', 'gas-elpiji']);
