import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { test, expect, vi } from 'vitest';
import { SlotCatalogGrid } from './SlotCatalogGrid';
import type { SlotCatalogEntry } from './types';

test('renders one cell per quota slot and marks completed/pending slots', () => {
  const entries: SlotCatalogEntry[] = [
    { slot_number: 1, status: 'completed', documentation_complete: true },
    { slot_number: 2, status: 'linked', documentation_complete: false },
  ];
  render(<SlotCatalogGrid quota={4} entries={entries} onSelect={vi.fn()} onCreateNext={vi.fn()} canCreate />);
  expect(screen.getByRole('button', { name: /Nomor 1.*Selesai/ })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: /Nomor 2.*Proses/ })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: /Buat Nomor 3/ })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Nomor 4 belum tersedia' })).toBeDisabled();
});

test('clicking an existing slot calls onSelect with its number', async () => {
  const onSelect = vi.fn();
  const entries: SlotCatalogEntry[] = [{ slot_number: 1, status: 'open', documentation_complete: false }];
  render(<SlotCatalogGrid quota={2} entries={entries} onSelect={onSelect} onCreateNext={vi.fn()} canCreate />);
  await userEvent.click(screen.getByRole('button', { name: /Nomor 1/ }));
  expect(onSelect).toHaveBeenCalledWith(1);
});

test('clicking the next empty slot calls onCreateNext', async () => {
  const onCreateNext = vi.fn();
  render(<SlotCatalogGrid quota={2} entries={[]} onSelect={vi.fn()} onCreateNext={onCreateNext} canCreate />);
  await userEvent.click(screen.getByRole('button', { name: /Buat Nomor 1/ }));
  expect(onCreateNext).toHaveBeenCalled();
});

test('without a quota, exactly one create-next box is shown after existing slots', () => {
  const entries: SlotCatalogEntry[] = [{ slot_number: 1, status: 'open', documentation_complete: false }];
  render(<SlotCatalogGrid entries={entries} onSelect={vi.fn()} onCreateNext={vi.fn()} canCreate />);
  expect(screen.getByRole('button', { name: /Buat Nomor 2/ })).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /Nomor 3/ })).not.toBeInTheDocument();
});

test('canCreate=false disables the create-next box', () => {
  render(<SlotCatalogGrid entries={[]} onSelect={vi.fn()} onCreateNext={vi.fn()} canCreate={false} />);
  expect(screen.getByRole('button', { name: /Buat Nomor 1/ })).toBeDisabled();
});
