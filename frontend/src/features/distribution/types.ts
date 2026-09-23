import type { Schedule } from '../programs/types';

export type MediaFile = { id: string; slot_id: string; original_filename: string; mime_type: string; byte_size: number; source: string; status: string; content_url: string; captured_at?: string };

export type SlotSummary = {
  id?: string; code: string; label: string; stage: 'mesin' | 'dokumen' | 'penyerahan'; status: string;
  required?: boolean; min_files?: number; max_files?: number;
  input_source?: 'camera' | 'gallery' | 'both'; require_location?: boolean; require_captured_at?: boolean; files?: MediaFile[];
};

export type DistributionSlot = {
  id: string; schedule_id: string; slot_number: number; status: 'open' | 'linked' | 'completed' | 'cancelled';
  allocation_id?: string; full_name?: string; nik?: string;
  machine_option_code?: string; machine_serial_number?: string;
  hose_option_code?: string; hose_serial_number?: string; converter_serial_number?: string;
  documentation: SlotSummary[]; distributed_at?: string; created_at: string; updated_at: string;
};

export type CreateSlotInput = {
  schedule_id: string;
  machine_option_code: string; machine_serial_number: string;
  hose_option_code: string; hose_serial_number: string; converter_serial_number: string;
};

export type CandidateMatch = {
  allocation_id: string; full_name: string; nik: string;
  sector_identifier: string; sector_identifier_type: string;
  address: string; village: string; district: string; phone_number: string;
  program_type: 'farmer' | 'fisherman';
};

export type LinkSlotInput = {
  schedule_id: string; slot_number: number; nik: string;
  address: string; village: string; district: string; phone_number: string; sector_identifier: string;
};

export type EquipmentOption = { code: string; brand: string; type?: string; spec?: string };

export type DataResponse<T> = { data: T };
export type ScheduleResponse = DataResponse<Schedule[]>;
