import type { ReactNode } from 'react';
import type { GuideTask } from './guideContent';
import type { GuideShotSpec } from './GuideShot';
import { beritaAcara } from './topics/beritaAcara';
import { dataPenerima } from './topics/dataPenerima';
import { dcp3 } from './topics/dcp3';
import { dokumentasiKegiatan } from './topics/dokumentasiKegiatan';
import { laporan } from './topics/laporan';
import { pendistribusian } from './topics/pendistribusian';
import { penggunaAkses } from './topics/penggunaAkses';
import { persiapanProgram } from './topics/persiapanProgram';

// Tutorial rinci per fitur (/panduan/:slug). Setiap topik dikaitkan ke tahap
// alur di guideContent.ts lewat stepId.

export type FieldRow = { name: string; meaning: ReactNode; example?: string; required?: boolean };

export type TopicSection = {
  /** Dipakai sebagai anchor (#id) di URL. */
  id: string;
  title: string;
  summary: string;
  shots?: GuideShotSpec[];
  steps: GuideTask[];
  fields?: FieldRow[];
  problems?: { q: string; a: ReactNode }[];
  doneWhen?: string;
};

export type GuideTopic = {
  slug: string;
  stepId: string;
  title: string;
  intro: string;
  manage: string[];
  view: string[];
  route: string;
  sections: TopicSection[];
};

export const guideTopics: GuideTopic[] = [persiapanProgram, penggunaAkses, dcp3, dataPenerima, pendistribusian, dokumentasiKegiatan, beritaAcara, laporan];

export function topicBySlug(slug: string | undefined) {
  return guideTopics.find((topic) => topic.slug === slug);
}

export function topicForStep(stepId: string) {
  return guideTopics.find((topic) => topic.stepId === stepId);
}

/**
 * Bagian tutorial yang sesuai dengan tab halaman saat ini, mis.
 * /persiapan-program?tab=schedules → #jadwal.
 */
const tabSections: Record<string, Record<string, string>> = {
  'persiapan-program': { regencies: 'kabupaten', programs: 'program', zones: 'zona', templates: 'template-paket', logos: 'logo', schedules: 'jadwal' },
  'berita-acara': { dp3: 'isian-preview', 'ba-perorangan': 'per-tanggal', 'rekap-harian': 'per-tanggal', 'closing-titik-serah': 'isian-preview', 'closing-kabupaten': 'isian-preview', rakorda: 'daftar-hadir', sosialisasi: 'daftar-hadir', 'training-10': 'daftar-hadir', 'training-100': 'daftar-hadir', 'ba-pemeriksaan': 'pemeriksaan', 'servis-berkala': 'servis', tkdn: 'tkdn', 'gas-elpiji': 'gas-lpg' },
};

export function sectionForTab(slug: string, tab: string | null) {
  return tab ? tabSections[slug]?.[tab] : undefined;
}
