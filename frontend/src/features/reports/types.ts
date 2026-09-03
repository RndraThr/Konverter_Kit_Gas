import type { Schedule } from '../programs/types';

export type StatusCount = { status: string; count: number };
export type ReportSummary = {
  total_allocations: number;
  allocation_status_counts: StatusCount[];
  distribution_status_counts: StatusCount[];
  documentation_incomplete: number;
};
export type ReportRow = {
  distribution_number: number;
  full_name: string;
  nik: string;
  sector_identifier: string;
  village: string;
  district: string;
  allocation_status: string;
  distribution_status: string;
  documentation_complete: boolean;
  completed_at?: string;
};
export type DataResponse<T> = { data: T };
export type ScheduleResponse = DataResponse<Schedule[]>;
