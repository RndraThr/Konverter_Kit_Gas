import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fetchRequest, fetchVoucherCandidates, fetchVouchers, fetchVoucherSettings, type FinanceRequest } from './api';
import { VoucherPage } from './VoucherPage';

vi.mock('./api', async (original) => ({
  ...await original<typeof import('./api')>(),
  fetchRequest: vi.fn(), fetchVouchers: vi.fn(), fetchVoucherSettings: vi.fn(), fetchVoucherCandidates: vi.fn(),
}));

const row = (id: string, type: 'cashless' | 'kasbon', total: number, vouchers: FinanceRequest['vouchers'] = []) => ({
  id, number: `PGJ-${id}`, type, total_amount: total, requester_name: 'Leader Tes', description: 'Sewa', regency_name: '', done_at: '2026-11-06T02:00:00Z', vouchers,
}) as FinanceRequest;

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><VoucherPage /></QueryClientProvider>);
}

describe('VoucherPage transaction picker', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(fetchVouchers).mockResolvedValue({ data: [] });
    vi.mocked(fetchVoucherSettings).mockResolvedValue({ data: { company_name: 'PT', address_line1: '', address_line2: '', address_line3: '', has_logo: true, accounts: [] } });
    vi.mocked(fetchVoucherCandidates).mockResolvedValue({ data: [row('1', 'cashless', 800000), row('2', 'kasbon', 5000000, [{ id: 'v', number: 'BSK1', date: '2026-11-05', created_at: '2026-11-05T02:00:00Z' }])] });
  });

  it('filters, selects what is visible and sums the choice', async () => {
    renderPage();
    await screen.findByText(/PGJ-1/);
    await userEvent.click(screen.getByRole('button', { name: 'Belum divoucher (1)' }));
    expect(screen.queryByText(/PGJ-2/)).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('checkbox', { name: 'Pilih semua' }));
    expect(screen.getByText('1 transaksi')).toBeInTheDocument();
    expect(screen.getAllByText('Rp800.000').length).toBeGreaterThan(1);
  });

  it('opens a date range calendar from Rentang', async () => {
    renderPage();
    await screen.findByText(/PGJ-1/);
    expect(screen.queryByRole('button', { name: 'Rentang tanggal' })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Rentang' }));
    await userEvent.click(screen.getByRole('button', { name: 'Rentang tanggal' }));
    expect(await screen.findAllByRole('grid')).toHaveLength(2);
  });

  it('loads attachments only when a row opens', async () => {
    vi.mocked(fetchRequest).mockResolvedValue({ data: { ...row('1', 'cashless', 800000), attachments: [{ id: 'a', kind: 'evidence', item_key: '', filename: 'nota.pdf', content_type: 'application/pdf', size_bytes: 1, created_at: '' }] } });
    renderPage();
    const list = await screen.findByRole('list', { name: 'Transaksi selesai' });
    expect(fetchRequest).not.toHaveBeenCalled();
    await userEvent.click(within(list).getAllByRole('button', { name: 'Lampiran' })[0]);
    expect(await screen.findByRole('button', { name: /Lihat .* nota\.pdf/ })).toBeInTheDocument();
    expect(fetchRequest).toHaveBeenCalledTimes(1);
  });
});
