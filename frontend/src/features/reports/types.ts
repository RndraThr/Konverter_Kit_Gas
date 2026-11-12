import type { Schedule } from '../programs/types';

// ReportRow: satu penerima Laporan DP3 (distribusi selesai, tanpa status).
export type ReportRow = {
  row_number: number;
  full_name: string;
  nik: string;
  card_number: string;
  phone_number: string;
  address: string;
  village: string;
  district: string;
  distribution_date: string;
  handover_location: string;
  distribution_number: number;
  machine_brand: string;
  machine_type: string;
  machine_power: string;
  machine_fuel_type: string;
  machine_serial: string;
  converter_brand: string;
  converter_serial: string;
  hose_serial: string;
};
export type Report = {
  schedule_id: string;
  program_name: string;
  program_type: 'farmer' | 'fisherman';
  fiscal_year: number;
  regency_name: string;
  province_name: string;
  rows: ReportRow[];
};
// DriveStatus: file Laporan DP3 terakhir di folder kabupaten di Drive.
export type DriveStatus = {
  available: boolean;
  filename: string;
  recipient_count: number;
  synced_at: string | null;
  last_error: string;
  last_attempt_at: string | null;
};
export type DataResponse<T> = { data: T };
export type ScheduleResponse = DataResponse<Schedule[]>;
