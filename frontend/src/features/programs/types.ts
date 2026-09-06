export type ProgramType = 'farmer' | 'fisherman';
export type Regency = { id: string; province_name: string; name: string; document_code: string; is_active: boolean; notes?: string };
export type Program = { id: string; code: string; name: string; program_type: ProgramType; fiscal_year: number; status: string; notes?: string };
export type PackageTemplate = { id: string; template_code: string; version: number; name: string; program_type: ProgramType; values: Record<string, unknown>; status: string };
export type DocumentationSlot = { slot_code: string; label: string; stage: string; is_required: boolean; min_files: number; max_files: number; input_source: 'camera' | 'gallery' | 'both'; require_location: boolean; require_captured_at: boolean; instructions?: string; sort_order: number };
export type DocumentationTemplate = { id: string; template_code: string; version: number; name: string; program_type: ProgramType; status: string; slots: DocumentationSlot[] };
export type Schedule = { id: string; program_id: string; regency_id: string; package_template_version_id: string; documentation_template_version_id: string; name: string; start_date: string; end_date: string; status: string; distribution_number_padding: number; supervisor_name?: string; notes?: string; program?: Program; regency?: Regency; package_template?: PackageTemplate; documentation_template?: DocumentationTemplate };

export type DataResponse<T> = { data: T };
export const programTypeLabel = (value: ProgramType) => value === 'farmer' ? 'Petani' : 'Nelayan';
export const formatDate = (value: string) => new Intl.DateTimeFormat('id-ID', { day: '2-digit', month: 'short', year: 'numeric' }).format(new Date(value));
export const dateInputValue = (value?: string) => value ? value.slice(0, 10) : '';
export const datePayload = (value: string) => `${value}T00:00:00Z`;
