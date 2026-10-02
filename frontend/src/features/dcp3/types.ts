import type { MachineOption, ProgramType, Schedule } from '../programs/types';

export type DCP3Mapping = {
  source_sequence: string;
  full_name: string;
  nik: string;
  farmer_card_number: string;
  kusuka_number: string;
  address: string;
  village: string;
  district: string;
  phone_number: string;
  machine_option: string;
};

export type PreviewRow = { source_row_number: number; values: Record<string, string> };
export type DCP3Preview = {
  id: string;
  schedule_id: string;
  program_type: ProgramType;
  original_filename: string;
  sheet_name: string;
  headers: string[];
  rows: PreviewRow[];
  status: string;
  machine_options: MachineOption[];
};
export type ImportResult = { batch_id: string; total_rows: number; valid_rows: number; warning_rows: number; invalid_rows: number };
export type ScheduleResponse = { data: Schedule[] };
export type DataResponse<T> = { data: T };

export const emptyMapping: DCP3Mapping = {
  source_sequence: '', full_name: '', nik: '', farmer_card_number: '', kusuka_number: '',
  address: '', village: '', district: '', phone_number: '', machine_option: '',
};
