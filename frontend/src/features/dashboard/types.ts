export type EvidenceSlot = {
  slot_code: string;
  label: string;
  is_required: boolean;
  min_files: number;
  accepted_files: number;
  complete: boolean;
};

export type ReplacementSummary = {
  full_name: string;
  nik?: string;
  reason: string;
  replaced_at: string;
};

export type Recipient = {
  allocation_id: string;
  distribution_number: number | null;
  allocation_status: 'candidate' | 'ready' | 'needs_review' | 'distributed' | 'replaced' | 'cancelled';
  distribution_status: 'draft' | 'completed' | 'cancelled' | null;
  full_name: string;
  nik: string;
  sector_identifier_type: string;
  sector_identifier: string;
  address: string;
  village: string;
  district: string;
  phone_number: string;
  program_id: string;
  program_name: string;
  program_type: 'farmer' | 'fisherman';
  zone_id: string;
  zone_code: string;
  zone_name: string;
  regency_id: string;
  regency_name: string;
  regency_document_code: string;
  schedule_id: string;
  schedule_name: string;
  evidence_slots: EvidenceSlot[];
  replaced_by?: ReplacementSummary;
  replaces?: ReplacementSummary;
};

export type RecipientPage = { items: Recipient[]; page: number; page_size: number; all?: boolean; total: number };

export type RecipientStats = {
  total: number;
  by_allocation_status: Record<string, number>;
  by_evidence_status: Record<string, number>;
};

// Per-kabupaten aggregate behind the distribution map. Recipient and evidence buckets come from the
// same query the Data Penerima list uses, so the map and the list always agree for one filter.
export type MapRegion = {
  regency_id: string;
  regency_name: string;
  province_name: string;
  document_code: string;
  recipients: number;
  candidate: number;
  ready: number;
  distributed: number;
  needs_review: number;
  replaced: number;
  evidence_complete: number;
  evidence_partial: number;
  evidence_empty: number;
  evidence_not_configured: number;
  slot_quota: number;
  slots_open: number;
  slots_linked: number;
  slots_completed: number;
  slots_cancelled: number;
};

export type MapData = { regions: MapRegion[]; totals: MapRegion };

export type RecipientInput = {
  schedule_id?: string;
  full_name: string;
  nik: string;
  sector_identifier: string;
  address: string;
  village: string;
  district: string;
  phone_number: string;
};

export const allocationStatusLabel: Record<string, string> = {
  candidate: 'Kandidat', ready: 'Siap', needs_review: 'Perlu ditinjau', distributed: 'Sudah distribusi', replaced: 'Digantikan', cancelled: 'Dibatalkan',
};

export const distributionStatusLabel: Record<string, string> = {
  draft: 'Draft', completed: 'Selesai', cancelled: 'Dibatalkan',
};
