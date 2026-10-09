import type { Schedule } from '../programs/types';

export type MediaStorageState = 'staging' | 'moving' | 'final' | 'move_failed';

export type MediaFile = {
  id: string; slot_id: string; original_filename: string; mime_type: string;
  byte_size: number; source: string; status: string; content_url: string;
  captured_at?: string; storage_state: MediaStorageState; storage_last_error?: string;
};

export type SlotSummary = {
  id?: string; code: string; label: string; stage: 'mesin' | 'dokumen' | 'penyerahan'; status: string;
  required?: boolean; min_files?: number; max_files?: number;
  media_kind?: 'image' | 'video' | 'image_video';
  input_source?: 'camera' | 'gallery' | 'both'; require_location?: boolean; require_captured_at?: boolean; files?: MediaFile[];
};

export type DistributionSlot = {
	id: string; schedule_id: string; slot_number: number; status: 'open' | 'linked' | 'completed' | 'cancelled';
	distribution_date: string | null;
  allocation_id?: string; full_name?: string; nik?: string;
  sector_identifier?: string; address?: string; village?: string; district?: string; phone_number?: string;
  machine_option_code?: string; machine_serial_number?: string;
  hose_option_code?: string; hose_serial_number?: string;
  converter_option_code?: string; converter_serial_number?: string;
  documentation: SlotSummary[]; distributed_at?: string;
  needs_recompletion: boolean; reopened_at?: string; reopened_by?: string; reopened_stage?: RevisionStage; revision_reason?: string;
  created_at: string; updated_at: string;
};

export type CreateSlotInput = {
	schedule_id: string;
	slot_number?: number;
};

export type RevisionStage = 'mesin' | 'dokumen' | 'penyerahan';
export type ReopenSlotInput = { schedule_id: string; slot_number: number; stage: RevisionStage; reason: string };
export type UpdateRecipientInput = { schedule_id: string; slot_number: number; address: string; village: string; district: string; phone_number: string; sector_identifier: string };
export type ReplaceRecipientInput = UpdateRecipientInput & { nik: string; full_name: string; reason: string };

// Mirrors distribution.CandidateLookup. "not_registered" is the only state that invites creating
// recipient data; every other state must be explained instead, otherwise the officer would try to
// add a NIK that already exists and hit the unique-NIK rejection.
export type CandidateLookupState = 'receivable' | 'needs_review' | 'not_available' | 'already_assigned' | 'previously_received' | 'not_registered';
export type CandidateLookup = {
  state: CandidateLookupState; full_name?: string; allocation_status?: string; distribution_number?: number;
};

export type RecipientReplacement = {
  id: string; slot_number: number;
  old_person_id: string; old_full_name: string;
  new_person_id: string; new_full_name: string;
  origin: 'existing_allocation' | 'new_allocation';
  reason: string; replaced_by_name?: string; replaced_at: string;
};

export type UpdateEquipmentInput = {
  machine_option_code: string; machine_serial_number: string;
  hose_option_code: string; hose_serial_number: string;
  converter_option_code: string; converter_serial_number: string;
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

export type SlotCatalogEntry = { slot_number: number; status: 'open' | 'linked' | 'completed' | 'cancelled'; documentation_complete: boolean; needs_recompletion?: boolean };

export type DataResponse<T> = { data: T };
export type ScheduleResponse = DataResponse<Schedule[]>;
