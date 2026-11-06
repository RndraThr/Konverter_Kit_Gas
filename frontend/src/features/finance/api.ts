import { apiBlobRequest, apiRequest } from '@/lib/api';

export type DataResponse<T> = { data: T };
export type RequestType = 'cashless' | 'kasbon';
export type Option = { id: string; name: string };
export type Item = { key: string; entry: 'debit' | 'kredit'; description: string; quantity: number; unit_price: number; note: string; project_code: string; amount?: number };
export type Attachment = { id: string; kind: string; item_key: string; filename: string; content_type: string; size_bytes: number; created_at: string };
export type FinanceEvent = { action: string; from_status: string; to_status: string; note: string; actor_name: string; created_at: string };

export type FinanceRequest = {
  id: string; number: string; type: RequestType; status: string;
  team_id: string; team_name: string; requester_id: string; requester_name: string; regency_id: string; regency_name: string;
  needed_date: string; category: string; description: string; total_amount: number;
  payee_name: string; bank_name: string; account_number: string; account_name: string; invoice_number: string;
  settlement_due: string; settlement_amount: number | null; settlement_note: string; review_note: string;
  reviewed_by_name: string; paid_by_name: string; submitted_at: string | null; reviewed_at: string | null; paid_at: string | null; done_at: string | null;
  created_at: string; updated_at: string;
  items?: Item[]; settlement_items?: Item[]; attachments?: Attachment[]; events?: FinanceEvent[]; actions?: string[];
  editable?: boolean; upload_kinds?: string[];
  vouchers?: VoucherRef[];
};

export type AccountKind = 'bank' | 'kas' | 'lainnya';
export type Account = { id: string; kind: AccountKind; name: string; account_number: string; is_active: boolean };
export type VoucherLine = { item_key: string; description: string; debit: number; kredit: number; note: string; project_code: string; is_bank: boolean };
export type VoucherRef = { id: string; number: string; date: string; created_at: string };
export type Voucher = { id: string; number: string; date: string; account_id: string; pay_to: string; description: string; total_amount: number; page_count: number; created_by_name: string; created_at: string; lines?: VoucherLine[]; request_ids?: string[] };
export type VoucherDraft = { pay_to: string; description: string; lines: VoucherLine[]; request_ids: string[] };
export type SheetLine = VoucherLine & { rowId: string; manual?: boolean };
export type SheetForm = { id: string; number: string; date: string; account_id: string; pay_to: string; description: string; lines: SheetLine[]; request_ids: string[] };
export type Payee = { id: string; account_name: string; bank_name: string; account_number: string };
export type VoucherSettings = { company_name: string; address_line1: string; address_line2: string; address_line3: string; has_logo: boolean; accounts: Account[] };

export type DraftInput = Pick<FinanceRequest, 'type' | 'team_id' | 'regency_id' | 'needed_date' | 'category' | 'description' | 'payee_name' | 'bank_name' | 'account_number' | 'account_name' | 'invoice_number' | 'settlement_due'> & { items: Item[] };

export type Team = { id: string; name: string; is_active: boolean; leaders: Option[]; finance: Option[]; regencies: Option[] };
export type TeamOptions = { leaders: Option[]; finance: Option[]; regencies: Option[] };
export type Profile = { can_request: boolean; can_review: boolean; can_monitor: boolean; can_manage: boolean; categories: string[]; leader_teams: Team[]; finance_teams: Team[] };
export type Page = { rows: FinanceRequest[]; total: number };
export type Summary = {
  statuses: { status: string; count: number; amount: number }[];
  teams: { team_id: string; name: string; count: number; paid: number; open: number }[];
  kasbon_open: number; kasbon_open_count: number; kasbon_overdue: number; waiting_review: number; waiting_payment: number; total_paid: number;
};

export const financeKey = ['finance'];

export const statusLabels: Record<string, string> = {
  draft: 'Draf', submitted: 'Menunggu review', revision: 'Perlu revisi', rejected: 'Ditolak', cancelled: 'Dibatalkan', approved: 'Disetujui',
  paid: 'Menunggu laporan', settlement_submitted: 'Laporan direview', settlement_revision: 'Laporan perlu revisi', done: 'Selesai',
};

export const actionLabels: Record<string, string> = {
  submit: 'Kirim pengajuan', cancel: 'Batalkan', approve: 'Setujui', reject: 'Tolak', revision: 'Minta revisi', pay: 'Tandai dibayar',
  settle: 'Kirim laporan', settle_approve: 'Terima laporan', settle_revision: 'Minta revisi laporan',
};

export const noteActions = new Set(['reject', 'revision', 'settle_revision']);

export const typeLabels: Record<RequestType, string> = { cashless: 'Cashless', kasbon: 'Kasbon' };

export const kindLabels: Record<string, string> = { evidence: 'Dokumen pendukung', payment: 'Bukti bayar', settlement: 'Nota realisasi', item_transaction: 'Nota / bukti', item_payment: 'Nota / bukti' };

export const formatRupiah = (value: number) => `Rp${new Intl.NumberFormat('id-ID').format(value)}`;
export const formatDate = (value: string) => value ? new Intl.DateTimeFormat('id-ID', { dateStyle: 'medium' }).format(new Date(`${value}T00:00:00`)) : '-';
export const formatDateTime = (value: string | null) => value ? new Intl.DateTimeFormat('id-ID', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) : '-';

export function statusTone(status: string) {
  if (status === 'done') return 'bg-emerald-600 text-white';
  if (status === 'rejected' || status === 'cancelled') return 'bg-destructive/10 text-destructive';
  if (status === 'revision' || status === 'settlement_revision') return 'bg-amber-100 text-amber-900 dark:bg-amber-950/40 dark:text-amber-200';
  if (status === 'draft') return 'bg-muted text-muted-foreground';
  return 'bg-sky-100 text-sky-900 dark:bg-sky-950/40 dark:text-sky-200';
}

export const fetchProfile = () => apiRequest<DataResponse<Profile>>('/api/v1/finance/profile');
export const fetchRequests = (params: URLSearchParams) => apiRequest<DataResponse<Page>>(`/api/v1/finance/requests?${params}`);
export const fetchRequest = (id: string) => apiRequest<DataResponse<FinanceRequest>>(`/api/v1/finance/requests/${id}`);
export const saveDraft = (id: string, input: DraftInput) => apiRequest<DataResponse<FinanceRequest>>(`/api/v1/finance/requests/${id}`, { method: 'PUT', body: JSON.stringify(input) });
export const runAction = (id: string, action: string, note = '') => apiRequest<DataResponse<FinanceRequest>>(`/api/v1/finance/requests/${id}/actions/${action}`, { method: 'POST', body: JSON.stringify({ note }) });
export const saveSettlement = (id: string, note: string, items: Item[]) => apiRequest<DataResponse<FinanceRequest>>(`/api/v1/finance/requests/${id}/settlement`, { method: 'PUT', body: JSON.stringify({ note, items }) });

export function uploadAttachment(id: string, kind: string, file: File, itemKey = '') {
  const form = new FormData();
  form.append('kind', kind);
  if (itemKey) form.append('item_key', itemKey);
  form.append('file', file, file.name);
  return apiRequest<DataResponse<Attachment>>(`/api/v1/finance/requests/${id}/attachments`, { method: 'POST', body: form });
}

export const deleteAttachment = (id: string, attachmentID: string) => apiRequest(`/api/v1/finance/requests/${id}/attachments/${attachmentID}`, { method: 'DELETE' });

export const fetchPayees = (teamID: string) => apiRequest<DataResponse<Payee[]>>(`/api/v1/finance/payees?team_id=${teamID}`);
export const savePayee = (teamID: string, input: Omit<Payee, 'id'>) => apiRequest<DataResponse<Payee>>('/api/v1/finance/payees', { method: 'POST', body: JSON.stringify({ team_id: teamID, ...input }) });
export const deletePayee = (id: string) => apiRequest(`/api/v1/finance/payees/${id}`, { method: 'DELETE' });

export const fetchVoucherSettings = () => apiRequest<DataResponse<VoucherSettings>>('/api/v1/finance/voucher-settings');
export const fetchVouchers = () => apiRequest<DataResponse<Voucher[]>>('/api/v1/finance/vouchers');
export const fetchVoucher = (id: string) => apiRequest<DataResponse<Voucher>>(`/api/v1/finance/vouchers/${id}`);
export const fetchVoucherCandidates = (from: string, to: string) => apiRequest<DataResponse<FinanceRequest[]>>(`/api/v1/finance/vouchers/candidates?from=${from}&to=${to}`);
export const composeVoucher = (requestIDs: string[]) => apiRequest<DataResponse<VoucherDraft>>('/api/v1/finance/vouchers/compose', { method: 'POST', body: JSON.stringify({ request_ids: requestIDs }) });
const voucherBody = (form: Omit<SheetForm, 'id'>) => JSON.stringify({ ...form, lines: form.lines.map(({ rowId: _rowId, manual: _manual, ...line }) => line) });
export const previewVoucher = ({ id: _id, ...form }: SheetForm) => apiBlobRequest('/api/v1/finance/vouchers/preview', { method: 'POST', body: voucherBody(form) });
export function saveVoucher({ id, ...form }: SheetForm) {
  const body = voucherBody(form);
  return apiRequest<DataResponse<Voucher>>(id ? `/api/v1/finance/vouchers/${id}` : '/api/v1/finance/vouchers', { method: id ? 'PUT' : 'POST', body });
}

export const attachmentPath = (id: string, attachmentID: string) => `/api/v1/finance/requests/${id}/attachments/${attachmentID}`;
export const voucherFilePath = (id: string) => `/api/v1/finance/vouchers/${id}/file`;
export const fetchFile = (path: string) => apiBlobRequest(path);

export async function downloadExcel(path: string, filename: string) {
  const blob = await apiBlobRequest(path);
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  link.click();
  window.setTimeout(() => URL.revokeObjectURL(url), 60_000);
}

export function requestLine(item: Pick<Item, 'quantity' | 'unit_price'>) {
  return item.quantity * item.unit_price;
}

/** Potongan (entry kredit, misal PPh) mengurangi total. */
export function itemsTotal(items: Item[]) {
  return items.reduce((sum, item) => sum + (item.entry === 'kredit' ? -requestLine(item) : requestLine(item)), 0);
}

/** Cara rekening terbaca di baris jurnal. */
export function accountLabel(account: Account) {
  if (account.kind === 'bank') return `Bank ${account.name} - ${account.account_number}`;
  return account.kind === 'kas' ? `Kas ${account.name}` : account.name;
}

/** Cara rekening terbaca di kotak Bank, Kas, Lainnya. */
export function accountBoxLabel(account: Account) {
  return account.kind === 'bank' ? `Bank ${account.name} — ${account.account_number}` : accountLabel(account);
}

export const sheetLines = (lines: VoucherLine[]): SheetLine[] => lines.map((line) => ({ ...line, rowId: crypto.randomUUID() }));

export function blankSheet(): SheetForm {
  return { id: '', number: '', date: today(), account_id: '', pay_to: '', description: '', request_ids: [], lines: sheetLines([{ item_key: '', description: '', debit: 0, kredit: 0, note: '', project_code: '', is_bank: true }]) };
}

/** Baris bank menutup selisih baris lain, kecuali nominalnya sudah diisi sendiri. */
export function rebalance(lines: SheetLine[]) {
  const net = lines.reduce((sum, line) => line.is_bank ? sum : sum + line.debit - line.kredit, 0);
  return lines.map((line) => line.is_bank && !line.manual ? { ...line, debit: Math.max(-net, 0), kredit: Math.max(net, 0) } : line);
}

export function shiftDate(value: string, days: number) {
  const date = new Date(`${value}T12:00:00`);
  date.setDate(date.getDate() + days);
  return new Intl.DateTimeFormat('en-CA').format(date);
}

/** Batas lapor kasbon paling cepat H+3 dari tanggal dana dibutuhkan. */
export const earliestSettlement = (needed: string) => shiftDate(needed, 3);

export function blankItem(): Item {
  return { key: crypto.randomUUID(), entry: 'debit', description: '', quantity: 1, unit_price: 0, note: '', project_code: '' };
}

/** Tahap pengajuan untuk indikator; null bila ditolak atau dibatalkan. */
export const stageLabels: Record<RequestType, string[]> = {
  cashless: ['Draf', 'Diajukan', 'Disetujui', 'Dibayar'],
  kasbon: ['Draf', 'Diajukan', 'Disetujui', 'Dibayar', 'Laporan', 'Selesai'],
};

export function stageIndex(type: RequestType, status: string): number | null {
  const cashless: Record<string, number> = { draft: 0, submitted: 1, revision: 1, approved: 2, done: 3 };
  const kasbon: Record<string, number> = { draft: 0, submitted: 1, revision: 1, approved: 2, paid: 3, settlement_submitted: 4, settlement_revision: 4, done: 5 };
  return (type === 'cashless' ? cashless : kasbon)[status] ?? null;
}

/** Kasbon yang belum dilaporkan melewati batas lapor. */
export function isOverdue(request: Pick<FinanceRequest, 'status' | 'settlement_due'>, now = today()) {
  return (request.status === 'paid' || request.status === 'settlement_revision') && request.settlement_due !== '' && request.settlement_due < now;
}

export function timeAgo(value: string | null, now = Date.now()) {
  if (!value) return '';
  const minutes = Math.floor((now - new Date(value).getTime()) / 60_000);
  if (minutes < 1) return 'baru saja';
  if (minutes < 60) return `${minutes} menit lalu`;
  if (minutes < 1440) return `${Math.floor(minutes / 60)} jam lalu`;
  return `${Math.floor(minutes / 1440)} hari lalu`;
}

export function today() {
  return new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Jakarta' }).format(new Date());
}
