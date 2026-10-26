import { apiRequest } from '@/lib/api';
import type { DataResponse } from '../programs/types';
import type { BaLogo } from './types';

type BrandingLogoResponse = Omit<BaLogo, 'content_url'>;

export function brandingLogosQueryKey(programID: string) {
  return ['bast', 'branding', programID];
}

function contentURL(programID: string, logoID: string) {
  return `/api/v1/bast/branding/logos/${logoID}/content?program_id=${encodeURIComponent(programID)}`;
}

/** Tender logos of a program in display order, each with its (same-origin, cookie-authenticated) content URL. */
export async function fetchBrandingLogos(programID: string): Promise<BaLogo[]> {
  const response = await apiRequest<DataResponse<BrandingLogoResponse[]>>(`/api/v1/bast/branding?program_id=${encodeURIComponent(programID)}`);
  return (response.data ?? [])
    .map((logo): BaLogo => ({ ...logo, content_url: contentURL(programID, logo.id) }))
    .sort((left, right) => left.sort_order - right.sort_order || left.id.localeCompare(right.id));
}
