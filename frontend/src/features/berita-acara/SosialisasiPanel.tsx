import type { ProgramType } from '../programs/types';
import { RakordaPanel, type RakordaConfig } from './RakordaPanel';

const sosialisasiConfig: RakordaConfig = {
  apiPath: 'sosialisasi',
  name: 'Sosialisasi',
  documentTitle: 'BA Sosialisasi',
  locationKey: 'sosialisasi_location',
  rowCountKey: 'sosialisasi_row_count',
  defaultRowCount: 51,
  folder: '7. SOSIALISASI',
  filenamePrefix: 'BA-SOSIALISASI',
  locationOptional: true,
  note: 'Nama penandatangan (Dinas Pertanian, Pelaksana, Konsultan Pengawas, Pertamina) diambil dari Konfigurasi Berita Acara jadwal. Bila belum diisi, kolom nama dicetak kosong untuk ditulis tangan.',
};

type Props = { scheduleID: string; regencyName: string; programType: ProgramType; defaultDate: string };

export function SosialisasiPanel(props: Props) {
  return <RakordaPanel config={sosialisasiConfig} {...props} />;
}
