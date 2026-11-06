import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import { blankItem, earliestSettlement, formatRupiah, itemsTotal, requestLine, type FinanceRequest } from './api';
import { ItemsEditor, RequestRow } from './shared';

describe('finance shared', () => {
  it('sums the lines and adds a new one', async () => {
    const onChange = vi.fn();
    render(<ItemsEditor items={[{ ...blankItem(), description: 'Sewa', quantity: 2, unit_price: 1500 }]} onChange={onChange} />);
    expect(screen.getByText('Total pengajuan').parentElement).toHaveTextContent('Rp3.000');
    await userEvent.click(screen.getByRole('button', { name: 'Tambah biaya' }));
    expect(onChange).toHaveBeenCalledWith([expect.anything(), expect.objectContaining({ entry: 'debit', description: '', quantity: 1 })]);
  });

  it('edits one line at a time and says which are unfinished', async () => {
    const lines = [{ ...blankItem(), description: 'Sewa', quantity: 2, unit_price: 1500 }, { ...blankItem(), description: 'Solar' }];
    render(<ItemsEditor items={lines} onChange={vi.fn()} />);
    expect(screen.getAllByLabelText(/Nama barang/)).toHaveLength(1);
    expect(screen.getByLabelText(/Nama barang/)).toHaveValue('Solar');
    expect(screen.getByText('2 × Rp1.500')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: /Sewa/ }));
    expect(screen.getByLabelText(/Nama barang/)).toHaveValue('Sewa');
    expect(screen.getByText('Belum lengkap')).toBeInTheDocument();
  });

  it('shows the number, type and status of a request', () => {
    const request = { id: 'r1', number: 'PGJ-202611-0001', type: 'kasbon', status: 'paid', team_name: 'Tim 1', requester_name: 'Dwi', needed_date: '', description: 'Uang makan', total_amount: 250000 } as FinanceRequest;
    render(<MemoryRouter><ul><RequestRow request={request} /></ul></MemoryRouter>);
    expect(screen.getByRole('link')).toHaveAttribute('href', '/keuangan/r1');
    expect(screen.getByText('Kasbon')).toBeInTheDocument();
    expect(screen.getByText('Menunggu laporan')).toBeInTheDocument();
    expect(screen.getByText(formatRupiah(250000))).toBeInTheDocument();
  });

  it('puts the kasbon report at H+3, across a month end', () => {
    expect(earliestSettlement('2026-11-29')).toBe('2026-12-02');
  });

  it('computes a line from quantity and price', () => {
    expect(requestLine({ quantity: 3, unit_price: 2000 })).toBe(6000);
  });

  it('takes kredit rows off the total', () => {
    expect(itemsTotal([{ ...blankItem(), quantity: 1, unit_price: 2250 }, { ...blankItem(), entry: 'kredit', quantity: 1, unit_price: 45 }])).toBe(2205);
  });
});
