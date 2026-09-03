import type { Schedule } from '../programs/types';

export type SlotSummary = { id?: string; code: string; label: string; status: string; required?: boolean; min_files?: number; max_files?: number };
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
  district?: string; phone_number?: string; eligibility: string; eligibility_reasons: string[];
  source_snapshot: Record<string, unknown>; receipt_history: ReceiptHistory[]; documentation: SlotSummary[];
};
export type DraftInput = { nik: string; sector_identifier: string; address: string; village: string; district: string; phone_number: string; identity_change_reason: string };
export type DataResponse<T> = { data: T };
export type ScheduleResponse = DataResponse<Schedule[]>;
