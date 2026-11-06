import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { describe, expect, it } from 'vitest';
import { accountBoxLabel, accountLabel, blankSheet, rebalance, sheetLines, type Account, type SheetForm } from './api';
import { VoucherSheet } from './VoucherSheet';

const bsi: Account = { id: 'a1', kind: 'bank', name: 'BSI Operasional', account_number: '7185595751', is_active: true };
const gopay: Account = { id: 'a2', kind: 'kas', name: 'GoPay', account_number: '', is_active: true };
const settings = { company_name: 'PT KIAN SANTANG MULIATAMA TBK', address_line1: 'Jalan Wibawa', address_line2: 'Blok A5', address_line3: 'Bekasi', has_logo: true, accounts: [bsi, gopay] };

function Sheet() {
  const [form, setForm] = useState<SheetForm>({ ...blankSheet(), account_id: 'a1' });
  return <VoucherSheet form={form} settings={settings} onChange={setForm} />;
}

describe('voucher sheet', () => {
  it('labels accounts as the printed voucher does', () => {
    expect(accountLabel(bsi)).toBe('Bank BSI Operasional - 7185595751');
    expect(accountBoxLabel(bsi)).toBe('Bank BSI Operasional — 7185595751');
    expect(accountLabel(gopay)).toBe('Kas GoPay');
  });

  it('keeps the bank row balancing the others', () => {
    const lines = sheetLines([
      { item_key: '', description: '', debit: 0, kredit: 0, note: '', project_code: '', is_bank: true },
      { item_key: '', description: 'Jasa', debit: 13187100, kredit: 0, note: '', project_code: '', is_bank: false },
      { item_key: '', description: 'Biaya Transfer', debit: 2900, kredit: 0, note: '', project_code: '', is_bank: false },
      { item_key: '', description: 'PPh', debit: 0, kredit: 263742, note: '', project_code: '', is_bank: false },
      { item_key: '', description: 'PPN', debit: 1450581, kredit: 0, note: '', project_code: '', is_bank: false },
    ]);
    expect(rebalance(lines)[0].kredit).toBe(14376839);
    expect(rebalance(lines.slice(0, 1).concat({ ...lines[3], kredit: 500 }))[0]).toMatchObject({ debit: 500, kredit: 0 });
  });

  it('adds a row, totals both sides and moves the mark between accounts', async () => {
    render(<Sheet />);
    await userEvent.click(screen.getByRole('button', { name: 'Tambah baris' }));
    await userEvent.type(screen.getByRole('textbox', { name: 'Debit baris 2' }), '200000');
    expect(screen.getByRole('textbox', { name: 'Debit baris 2' })).toHaveValue('200.000');
    expect(screen.getByText('Rp200.000')).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: 'Kredit baris 1' })).toHaveValue('200.000');
    await userEvent.click(screen.getByRole('button', { name: /GoPay/ }));
    expect(screen.getByRole('button', { name: '[ ] Bank BSI Operasional — 7185595751' })).toBeInTheDocument();
  });

  it('lets the bank row be typed, shows the gap and can follow again', async () => {
    render(<Sheet />);
    await userEvent.click(screen.getByRole('button', { name: 'Tambah baris' }));
    await userEvent.type(screen.getByRole('textbox', { name: 'Debit baris 2' }), '200000');
    await userEvent.clear(screen.getByRole('textbox', { name: 'Kredit baris 1' }));
    await userEvent.type(screen.getByRole('textbox', { name: 'Kredit baris 1' }), '150000');
    expect(screen.getByRole('status')).toHaveTextContent('Selisih Rp50.000');
    await userEvent.click(screen.getByRole('button', { name: 'Samakan otomatis' }));
    expect(screen.getByRole('textbox', { name: 'Kredit baris 1' })).toHaveValue('200.000');
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it('takes a typed row off the other side', async () => {
    render(<Sheet />);
    await userEvent.click(screen.getByRole('button', { name: 'Tambah baris' }));
    await userEvent.type(screen.getByRole('textbox', { name: 'Debit baris 2' }), '100');
    await userEvent.type(screen.getByRole('textbox', { name: 'Kredit baris 2' }), '50');
    expect(screen.getByRole('textbox', { name: 'Debit baris 2' })).toHaveValue('');
  });
});
