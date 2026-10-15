export type FinalReportSettings = {
  program_id: string;
  regency_id: string;
  leader_count: number;
  admin_count: number;
  technician_count: number;
  helper_count: number;
  machine_manufacturer_name: string;
  delivery_hose_date: string;
  delivery_machine_group_date: string;
  delivery_gas_cylinder_date: string;
};

export type FinalReportPhotoPick = {
  id: string;
  activity_media_id: string;
  slide_key: string;
  sort_order: number;
  display_name: string;
  content_url: string;
};

export type FinalReportPreview = {
  regency_label: string;
  fiscal_year: string;
  agriculture_office_name: string;
  consultant_company_name: string;
  machine_manufacturer_name: string;
  delivery_hose_date: string;
  delivery_machine_group_date: string;
  delivery_gas_cylinder_date: string;
  delivery_range_start: string;
  delivery_range_end: string;
  rakorda_date: string;
  sosialisasi_date: string;
  leader_count: string;
  admin_count: string;
  technician_count: string;
  helper_count: string;
  handover_location: string;
  distribution_start_date: string;
  distribution_end_date: string;
  distribution_total: string;
  photo_picks: Record<string, FinalReportPhotoPick[]>;
};

export type FinalReportDocument = {
  id: string;
  version: number;
  filename: string;
  status: string;
  finalized_at: string;
};

export const SLIDE_KEYS = ['pengiriman_penerimaan', 'rapat_koordinasi', 'sosialisasi'] as const;
export type SlideKey = (typeof SLIDE_KEYS)[number];
export const SLIDE_KEY_LABELS: Record<SlideKey, string> = {
  pengiriman_penerimaan: 'Pengiriman & Penerimaan Barang',
  rapat_koordinasi: 'Rapat Koordinasi',
  sosialisasi: 'Seremoni & Sosialisasi',
};
export const SLIDE_KEY_ACTIVITY_TYPES: Record<SlideKey, string[]> = {
  pengiriman_penerimaan: ['unloading_konkit', 'unloading_mesin_pompa', 'unloading_oli', 'unloading_selang', 'unloading_tabung_gas'],
  rapat_koordinasi: ['rakor'],
  sosialisasi: ['ceremony_sosialisasi'],
};
