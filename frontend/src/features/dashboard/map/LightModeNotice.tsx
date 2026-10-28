import { Button } from '../../../components/ui/button';

export const LIGHT_MODE_FAILED_TEXT = 'Peta ringan dipakai karena peta dasar gagal dimuat';
export const LIGHT_MODE_MANUAL_TEXT = 'Mode ringan aktif: peta sederhana tanpa peta dasar, hemat kuota dan sinyal.';

/** Banner above the SVG fallback map, with the way back to the interactive map. */
export function LightModeNotice({ reason, onRetry }: { reason: 'failed' | 'manual'; onRetry: () => void }) {
  return <div role="status" className="flex flex-wrap items-center gap-2 border-b bg-muted px-4 py-2 text-sm">
    <span>{reason === 'failed' ? `${LIGHT_MODE_FAILED_TEXT}. Titik dan batas zona belum ditampilkan; tabel progres di bawah tetap lengkap.` : LIGHT_MODE_MANUAL_TEXT}</span>
    <Button type="button" size="sm" variant="outline" className="ml-auto" onClick={onRetry}>{reason === 'failed' ? 'Coba peta interaktif lagi' : 'Kembali ke peta interaktif'}</Button>
  </div>;
}
