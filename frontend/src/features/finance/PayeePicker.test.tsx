import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fetchPayees, savePayee } from './api';
import { PayeePicker } from './PayeePicker';

vi.mock('./api', async (original) => ({ ...await original<typeof import('./api')>(), fetchPayees: vi.fn(), savePayee: vi.fn(), deletePayee: vi.fn() }));

const empty = { account_name: '', bank_name: '', account_number: '' };
const saved = { id: 'p1', account_name: 'CV Maju', bank_name: 'BRI', account_number: '1234567890' };

function renderPicker(onChange = vi.fn()) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><PayeePicker teamID="t1" value={empty} onChange={onChange} /></QueryClientProvider>);
  return onChange;
}

describe('PayeePicker', () => {
  beforeEach(() => vi.clearAllMocks());

  it('starts empty and opens the form from the button', async () => {
    vi.mocked(fetchPayees).mockResolvedValue({ data: [] });
    renderPicker();
    expect(screen.queryByLabelText(/Nomor rekening/)).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Tambah rekening baru' }));
    expect(screen.getByRole('button', { name: 'Simpan rekening' })).toBeDisabled();
    await userEvent.type(screen.getByLabelText(/Nama pemilik rekening/), 'CV Maju');
    await userEvent.type(screen.getByLabelText(/Bank/), 'BRI');
    await userEvent.type(screen.getByLabelText(/Nomor rekening/), '1234567890');
    vi.mocked(savePayee).mockResolvedValue({ data: saved });
    await userEvent.click(screen.getByRole('button', { name: 'Simpan rekening' }));
    await waitFor(() => expect(savePayee).toHaveBeenCalledWith('t1', { account_name: 'CV Maju', bank_name: 'BRI', account_number: '1234567890' }));
  });

  it('reuses a saved account with one click', async () => {
    vi.mocked(fetchPayees).mockResolvedValue({ data: [saved] });
    const onChange = renderPicker();
    await userEvent.click(await screen.findByRole('button', { name: /CV Maju/, pressed: false }));
    expect(onChange).toHaveBeenCalledWith(saved);
  });
});
