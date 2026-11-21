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
