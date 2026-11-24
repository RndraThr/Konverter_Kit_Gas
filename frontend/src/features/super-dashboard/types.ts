import type { Program } from '../monitoring/types';

/** How far recipients have gone, against the quota. */
export type DashboardStages = {
  quota: number;
  linked: number;
  completed: number;
  approved: number;
  ba_final: number;
};

export type DashboardRegency = DashboardStages & {
  schedule_id: string;
  regency_id: string;
  regency_name: string;
  province_name: string;
  /** "" = no zone. */
  zone_name: string;
  zone_sort: number;
};

export type DashboardZone = DashboardStages & { zone_name: string; zone_sort: number };

export type DashboardDay = { date: string; completed: number };

export type DashboardActivity = { activity_type: string; media: number };

export type Dashboard = {
  programs: Program[];
  program_id: string;
  totals: DashboardStages;
  zones: DashboardZone[];
  regencies: DashboardRegency[];
  daily: DashboardDay[];
  activities: DashboardActivity[];
};
