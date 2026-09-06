import type { Schedule } from '../programs/types';

export type MediaFile = { id: string; slot_id: string; original_filename: string; mime_type: string; byte_size: number; source: string; status: string; content_url: string; captured_at?: string };
export type SlotSummary = { id?: string; code: string; label: string; status: string; required?: boolean; min_files?: number; max_files?: number; input_source?: 'camera' | 'gallery' | 'both'; require_location?: boolean; require_captured_at?: boolean; files?: MediaFile[] };
export type SearchResult = {
  allocation_id: string; distribution_number: number; full_name: string; masked_nik: string;
  location: string; program_type: 'farmer' | 'fisherman'; eligibility: string;
  allocation_status: string; documentation: SlotSummary[];
};
export type ReceiptHistory = { completed_at: string; regency: string; program: string; bast_number?: string };
export type RecipientWorkspaceData = {
  allocation_id: string; distribution_id: string; schedule_id: string; distribution_number: number;
  allocation_status: string; distribution_status: string; program_type: 'farmer' | 'fisherman';
  program_name: string; regency_name: string; full_name: string; nik?: string;
  sector_identifier?: string; sector_identifier_type?: string; address?: string; village?: string;
  district?: string; phone_number?: string;
  machine_option_code?: string; machine_serial_number?: string; hose_option_code?: string; hose_serial_number?: string; converter_serial_number?: string;
  eligibility: string; eligibility_reasons: string[];
  source_snapshot: Record<string, unknown>; package_snapshot?: Record<string, unknown>; receipt_history: ReceiptHistory[]; documentation: SlotSummary[];
};
export type EquipmentOption = { code: string; brand: string; type?: string; spec?: string };
export type DistributionRecord = { id: string; allocation_id: string; status: 'completed'; completed_at: string };
export type DraftInput = {
  nik: string; sector_identifier: string; address: string; village: string; district: string; phone_number: string; identity_change_reason: string;
  machine_option_code: string; machine_serial_number: string; hose_option_code: string; hose_serial_number: string; converter_serial_number: string;
};
export type DataResponse<T> = { data: T };
export type ScheduleResponse = DataResponse<Schedule[]>;
