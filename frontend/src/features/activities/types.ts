export type ActivityType =
  | 'ceremony_sosialisasi'
  | 'pelatihan_teknis'
  | 'rakor'
  | 'training_10'
  | 'training_100'
  | 'unloading_konkit'
  | 'unloading_mesin_pompa'
  | 'unloading_oli'
  | 'unloading_selang'
  | 'unloading_tabung_gas';

export type ActivityMedia = {
  id: string;
  regency_id: string;
  regency_name: string;
  regency_document_code: string;
  activity_type: ActivityType;
  display_name: string;
  original_filename: string;
  media_type: 'image' | 'video';
  mime_type: string;
  byte_size: number;
  checksum: string;
  source: 'camera' | 'gallery';
  status: 'active' | 'deleted';
  uploaded_by?: string;
  uploaded_at: string;
  created_at: string;
  updated_at: string;
  content_url: string;
};

export type ActivityMediaPage = {
  items: ActivityMedia[];
  page: number;
  page_size: number;
  total: number;
};

export type RegencyOption = { id: string; name: string; document_code: string };

export const activityTypeLabels: Record<ActivityType, string> = {
  ceremony_sosialisasi: 'Ceremony & Sosialisasi',
  pelatihan_teknis: 'Pelatihan Teknis',
  rakor: 'Rakor',
  training_10: 'Training 10%',
  training_100: 'Training 100%',
  unloading_konkit: 'Unloading Konkit',
  unloading_mesin_pompa: 'Unloading Mesin Pompa',
  unloading_oli: 'Unloading Oli',
  unloading_selang: 'Unloading Selang Hisap & Buang',
  unloading_tabung_gas: 'Unloading Tabung Gas',
};
