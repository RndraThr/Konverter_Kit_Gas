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
  padding: number;
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
};

export type RakordaUpload = {
  id: string;
  schedule_id: string;
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
  document_type: 'dp3' | 'daily_recap';
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
