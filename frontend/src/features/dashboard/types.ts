export type EvidenceSlot = {
  slot_code: string;
  label: string;
  is_required: boolean;
  min_files: number;
  accepted_files: number;
  complete: boolean;
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
  regency_id: string;
  regency_name: string;
  regency_document_code: string;
  schedule_id: string;
  schedule_name: string;
  evidence_slots: EvidenceSlot[];
};

export type RecipientPage = { items: Recipient[]; page: number; page_size: number; total: number };

export type RecipientStats = {
  total: number;
  by_allocation_status: Record<string, number>;
  by_evidence_status: Record<string, number>;
};

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
  candidate: 'Kandidat', ready: 'Siap', needs_review: 'Perlu ditinjau', distributed: 'Sudah distribusi', replaced: 'Diganti', cancelled: 'Dibatalkan',
};

export const distributionStatusLabel: Record<string, string> = {
  draft: 'Draft', completed: 'Selesai', cancelled: 'Dibatalkan',
};
