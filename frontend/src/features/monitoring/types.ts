import type { DistributionSlot } from '../distribution/types';

export type Phase = 'material' | 'rakor' | 'sosialisasi' | 'distribusi' | 'penutupan';
export type Person = { id: string; name: string };
export type PhaseDay = { day: string; phase: Phase };
export type Program = { id: string; name: string; fiscal_year: number; status: string };

export type ScheduleRow = {
  schedule_id: string; name: string; regency_id: string; regency_name: string; province_name: string;
  zone_name: string; zone_sort: number; start_date: string; end_date: string; quota: number; sheet_no: number;
  days: PhaseDay[]; reviewers: Person[]; leaders: Person[];
  today_phase: Phase | ''; distribution_day: number; distribution_of: number;
  completed: number; completed_today: number; approved: number; pending: number; revision: number; pending_photos: number;
  last_activity_at: string | null;
};

export type Overview = {
  programs: Program[]; program_id: string; date: string; rakornas: string[];
  rows: ScheduleRow[]; reviewers: Person[]; users: Person[];
};

export type ReviewState = 'pending' | 'revision' | 'partial' | 'approved';

export type QueueItem = {
  slot_number: number; full_name: string; nik: string; village: string; machine_serial: string;
  distribution_date: string; status: string; completed_at: string | null; completed_by: string;
  photos: number; approved_photos: number; rejected_photos: number; pending_photos: number;
  state: ReviewState; last_upload_at: string | null; reviewed_by: string; reviewed_at: string | null;
};

/** The check of one photo; a photo without one is not checked yet. */
export type MediaReview = { status: 'approved' | 'rejected'; reason: string; reviewed_by: string; reviewed_at: string };

export type SlotDetail = {
  slot: DistributionSlot; schedule_name: string; regency_name: string;
  completed_at: string | null; completed_by: string; state: ReviewState; reviews: Record<string, MediaReview>;
};

/** Phases in calendar order, with the colors of the "Jadwal Pendistribusian" sheet. */
export const phases: { phase: Phase; label: string; short: string; cell: string; dot: string }[] = [
  { phase: 'material', label: 'Pengiriman material', short: 'Material', cell: 'bg-[#FFC000] text-neutral-900', dot: 'bg-[#FFC000]' },
  { phase: 'rakor', label: 'Rapat koordinasi', short: 'Rakor', cell: 'bg-[#92D050] text-neutral-900', dot: 'bg-[#92D050]' },
  { phase: 'sosialisasi', label: 'Sosialisasi & training', short: 'Sosialisasi', cell: 'bg-[#00B050] text-white', dot: 'bg-[#00B050]' },
  { phase: 'distribusi', label: 'Pendistribusian', short: 'Distribusi', cell: 'bg-[#00B0F0] text-neutral-900', dot: 'bg-[#00B0F0]' },
  { phase: 'penutupan', label: 'Penutupan', short: 'Penutupan', cell: 'bg-[#0070C0] text-white', dot: 'bg-[#0070C0]' },
];
export const rakornasStyle = { label: 'Rakornas', cell: 'bg-[#7030A0] text-white', dot: 'bg-[#7030A0]' };
export const phaseInfo = (phase: string) => phases.find((p) => p.phase === phase);

export const stateLabels: Record<ReviewState, { label: string; tone: string }> = {
  pending: { label: 'Ada foto baru', tone: 'bg-amber-100 text-amber-900 dark:bg-amber-950/40 dark:text-amber-200' },
  revision: { label: 'Perlu revisi', tone: 'bg-destructive/10 text-destructive' },
  partial: { label: 'Menunggu foto', tone: 'bg-sky-100 text-sky-900 dark:bg-sky-950/40 dark:text-sky-200' },
  approved: { label: 'Disetujui', tone: 'bg-emerald-100 text-emerald-900 dark:bg-emerald-950/40 dark:text-emerald-200' },
};

export const stageLabels: Record<string, string> = { mesin: 'POS Mesin', dokumen: 'POS Dokumen', penyerahan: 'POS Penyerahan' };

/** Photo counts of one activity type of a kabupaten. */
export type ActivityCell = {
  schedule_id: string; regency_id: string; activity_type: string;
  photos: number; approved: number; rejected: number; pending: number; last_upload_at: string | null;
};

export type ActivityPhoto = {
  id: string; content_url: string; mime_type: string; display_name: string; source: string;
  uploaded_at: string; uploaded_by: string; review: MediaReview | null;
};

export type ActivityAlbum = { schedule_id: string; regency_id: string; regency_name: string; activity_type: string; photos: ActivityPhoto[] };

/**
 * Activity types checked by the central team, in the app's order, with the
 * phase whose last day they are due by. Optional ones never show as missing.
 */
export const activityTypes: { code: string; label: string; short: string; phase: Phase; required: boolean }[] = [
  { code: 'kunjungan_dinas', label: 'Kunjungan Kantor Dinas', short: 'Dinas', phase: 'material', required: false },
  { code: 'gudang_titik_serah', label: 'Gudang Titik Serah', short: 'Gudang', phase: 'material', required: false },
  { code: 'unloading_konkit', label: 'Unloading Konkit', short: 'Konkit', phase: 'material', required: true },
  { code: 'unloading_mesin_pompa', label: 'Unloading Mesin Pompa', short: 'Mesin', phase: 'material', required: true },
  { code: 'unloading_oli', label: 'Unloading Oli', short: 'Oli', phase: 'material', required: true },
  { code: 'unloading_selang', label: 'Unloading Selang', short: 'Selang', phase: 'material', required: true },
  { code: 'unloading_tabung_gas', label: 'Unloading Tabung Gas', short: 'Tabung', phase: 'material', required: true },
  { code: 'perakitan_mesin', label: 'Perakitan Mesin', short: 'Rakit', phase: 'material', required: false },
  { code: 'rakor', label: 'Rakor', short: 'Rakor', phase: 'rakor', required: true },
  { code: 'pemeriksaan_barang', label: 'Pemeriksaan Barang', short: 'Periksa', phase: 'rakor', required: false },
  { code: 'ceremony_sosialisasi', label: 'Ceremony & Sosialisasi', short: 'Seremoni', phase: 'sosialisasi', required: true },
  { code: 'training_10', label: 'Training 10%', short: 'Tr 10%', phase: 'sosialisasi', required: true },
  { code: 'pelatihan_teknis', label: 'Pelatihan Teknis', short: 'Pelatihan', phase: 'sosialisasi', required: true },
  { code: 'kegiatan_distribusi', label: 'Kegiatan Pendistribusian', short: 'Distribusi', phase: 'distribusi', required: true },
  { code: 'training_100', label: 'Training 100%', short: 'Tr 100%', phase: 'distribusi', required: true },
  { code: 'closing_kabupaten', label: 'Closing Kabupaten', short: 'Closing', phase: 'penutupan', required: true },
  { code: 'kegiatan', label: 'Kegiatan Lainnya', short: 'Lainnya', phase: 'penutupan', required: false },
];
export const activityLabel = (code: string) => activityTypes.find((t) => t.code === code)?.label ?? code;

export type ActivityCellState = 'revision' | 'pending' | 'approved' | 'missing' | 'empty';

/**
 * What a cell shows: photos waiting for a fix or a check first, then all
 * approved; without photos, missing once its phase is over (required types only).
 */
export function activityCellState(cell: ActivityCell | undefined, type: (typeof activityTypes)[number], days: PhaseDay[], today: string): ActivityCellState {
  if (cell && cell.rejected > 0) return 'revision';
  if (cell && cell.pending > 0) return 'pending';
  if (cell && cell.photos > 0) return 'approved';
  if (!type.required) return 'empty';
  const due = days.filter((d) => d.phase === type.phase).map((d) => d.day).sort().at(-1);
  return due && due < today ? 'missing' : 'empty';
}

/** One slot that recorded a duplicated serial. */
export type SerialEntry = { schedule_id: string; regency_name: string; slot_number: number; status: string; entered_by: string; entered_at: string | null };
/** A serial of one kind recorded on several slots. */
export type SerialDuplicate = { field: 'machine' | 'converter' | 'hose'; serial: string; entries: SerialEntry[] };
export const serialFieldLabels: Record<SerialDuplicate['field'], string> = { machine: 'Mesin', converter: 'Konkit/Reducer', hose: 'Selang' };

/** How many duplicated serials each schedule has a slot in. */
export function duplicatesBySchedule(list: SerialDuplicate[]): Record<string, number> {
  const out: Record<string, number> = {};
  list.forEach((d) => new Set(d.entries.map((e) => e.schedule_id)).forEach((id) => { out[id] = (out[id] ?? 0) + 1; }));
  return out;
}
