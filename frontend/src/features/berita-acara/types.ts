export type DailyBundle = {
  id: string;
  program_id: string;
  regency_id: string;
  local_date: string;
  filename: string;
  recipient_count: number;
  page_count: number;
  version: number;
  status: 'active' | 'superseded' | 'failed';
  checksum: string;
  last_error?: string;
  synced_at?: string;
};

export type DateSummary = {
  local_date: string;
  recipient_count: number;
  validation_status: 'ready' | 'configuration_required' | 'total_not_locked';
  bundle?: DailyBundle;
};

export type RecipientDocument = {
  distribution_slot_id: string;
  slot_number: number;
  final_total: number;
  document_number: string;
  local_date: string;
};

export type LockResult = { program_id: string; regency_id: string; final_total: number; locked_at: string };
export type BundleRequest = { program_id: string; regency_id: string; local_date: string };

export type BaLogo = {
  id: string;
  program_id: string;
  slot_code: string;
  original_filename: string;
  mime_type: 'image/png' | 'image/jpeg';
  byte_size: number;
  checksum: string;
  sort_order: number;
  max_width_mm: number;
  max_height_mm: number;
  is_visible: boolean;
  content_url: string;
};

export type ScheduleSettings = {
  schedule_id: string;
  handover_location: string;
  consultant_company_name: string;
  agriculture_office_name: string;
  agriculture_office_nip: string;
  installer_name: string;
  supervisor_name: string;
  pertamina_rep_name: string;
  rakorda_location: string;
  rakorda_row_count: number;
  sosialisasi_location: string;
  sosialisasi_row_count: number;
  training_10_location: string;
  training_10_row_count: number;
  training_100_location: string;
  training_100_row_count: number;
  servis_1_start: string;
  servis_1_end: string;
  servis_2_start: string;
  servis_2_end: string;
};

/** Di profil, ref menunjuk barang template; description/notes boleh memuat {merk}, {tipe}, {spek}. */
export type PemeriksaanRow = { ref: string; description: string; unit: string; quantity_per_package: number; notes: string };

/** Barang Template Paket jadwal yang dapat dirujuk susunan TKDN dan form Pemeriksaan. */
export type TemplateEntry = { ref: string; kind: string; label: string; unit: string; quantity_per_package: number; variants: { code: string; brand: string; type: string; spec: string; tkdn_percent: number }[] };

export type PemeriksaanChecklist = {
  packaging: string;
  quantity: string;
  specification: string;
  condition: string;
  documents: string[];
  other_document: string;
  function_test: string;
  conclusion: string;
};

export type PemeriksaanForm = { code: string; title: string; rows: PemeriksaanRow[]; checklist: PemeriksaanChecklist; note: string };

export type PemeriksaanProfile = { program_id: string; forms: PemeriksaanForm[]; is_default: boolean; updated_at?: string };

export type PemeriksaanFormSummary = { code: string; title: string; po_number: string; rows: PemeriksaanRow[] };

export type PemeriksaanSummary = { total_packages: number; profile: PemeriksaanProfile; forms: PemeriksaanFormSummary[]; entries: TemplateEntry[] };

export type TKDNItem = {
  group: string;
  name: string;
  brand: string;
  quantity_per_package: number;
  tkdn_percent: number;
};

export type TKDNRow = { name: string; group: string; ref: string };

export type TKDNProfile = {
  program_id: string;
  rows: TKDNRow[];
  total_tkdn: number;
  is_default: boolean;
  updated_at?: string;
};

export type TKDNSummary = { total_packages: number; profile: TKDNProfile; items: TKDNItem[]; entries: TemplateEntry[] };

export type ScheduleItemChoice = {
  ref: string;
  name: string;
  variants: { code: string; brand: string }[];
  selected: string;
  source: 'single' | 'manual' | 'machine' | 'default';
  po_number: string;
  inspected: boolean;
};

export type ZonePORow = { kind: string; code: string; name: string; brand: string; po_number: string };
export type ZonePO = { zone_id: string; zone_name: string; rows: ZonePORow[] };

export type ScheduleItems = { schedule_id: string; program_id: string; zone_name: string; machine_brand: string; items: ScheduleItemChoice[] };

export type ServisPeriod = { start: string; end: string };

export type ServisBerkalaSummary = {
  total_packages: number;
  services: ServisPeriod[];
  schedule_ready: boolean;
};

export type RakordaUpload = {
  id: string;
  schedule_id: string;
  document_kind: 'rakorda' | 'sosialisasi' | 'training_10' | 'training_100';
  event_date: string;
  original_name: string;
  mime_type: 'application/pdf' | 'image/jpeg' | 'image/png';
  byte_size: number;
  created_at: string;
};

export type AggregateDocument = {
  id: string;
  schedule_id: string;
  program_id: string;
  regency_id: string;
  // dp3, daily_recap, closing_titik_serah, closing_kabupaten, servis_berkala,
  // tkdn, atau pemeriksaan:<kode form>.
  document_type: string;
  document_date: string;
  filename: string;
  recipient_count: number;
  page_count: number;
  version: number;
  status: 'active' | 'superseded';
  checksum: string;
  last_error?: string;
  finalized_at: string;
};

export type DP3Summary = {
  total_recipients: number;
  numbered_recipients: number;
  unmounted_recipients: number;
  validation_status: string;
};

export type DP3Recipient = {
  full_name: string;
  nik: string;
  address: string;
  village: string;
  district: string;
  regency: string;
  distribution_number: number | null;
  machine_brand: string;
  machine_type: string;
  machine_power: string;
  machine_fuel_type: string;
  source: string;
};

export type ClosingRow = {
  local_date: string;
  machine_brand: string;
  machine_type: string;
  count: number;
};

export type ClosingKabupatenRow = {
  location: string;
  machine_brand: string;
  machine_type: string;
  count: number;
};

export type DailyRecapRecipient = {
  slot_number: number;
  full_name: string;
  farmer_card_number: string;
  machine_brand: string;
  machine_type: string;
  machine_serial: string;
  machine_power: string;
  machine_fuel_type: string;
};

export type DailyRecapDate = {
  local_date: string;
  recipient_count: number;
  validation_status: string;
  document?: AggregateDocument;
};

/** Tanggal distribusi untuk BA Training: jumlah penerima dan peserta (10% pertama untuk Training 10%). */
export type TrainingDate = { local_date: string; recipient_count: number; participant_count: number };
