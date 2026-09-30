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
