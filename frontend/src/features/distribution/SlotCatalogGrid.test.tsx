import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { test, expect, vi } from 'vitest';
import { SlotCatalogGrid } from './SlotCatalogGrid';
import type { SlotCatalogEntry } from './types';

test('with a quota, every empty number is directly creatable', () => {
  const entries: SlotCatalogEntry[] = [
    { slot_number: 1, status: 'completed', documentation_complete: true },
    { slot_number: 2, status: 'linked', documentation_complete: false },
  ];
  render(<SlotCatalogGrid quota={4} entries={entries} onSelect={vi.fn()} onCreate={vi.fn()} canCreate />);
  expect(screen.getByRole('button', { name: /Nomor 1.*Selesai/ })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: /Nomor 2.*Proses/ })).toBeInTheDocument();
  // Both remaining numbers within quota are fillable directly, not just the next sequential one.
  expect(screen.getByRole('button', { name: /Buat Nomor 3/ })).toBeEnabled();
  expect(screen.getByRole('button', { name: /Buat Nomor 4/ })).toBeEnabled();
});

test('clicking an existing slot calls onSelect with its number', async () => {
  const onSelect = vi.fn();
  const entries: SlotCatalogEntry[] = [{ slot_number: 1, status: 'open', documentation_complete: false }];
  render(<SlotCatalogGrid quota={2} entries={entries} onSelect={onSelect} onCreate={vi.fn()} canCreate />);
  await userEvent.click(screen.getByRole('button', { name: /Nomor 1/ }));
  expect(onSelect).toHaveBeenCalledWith(1);
});

test('clicking any empty quota number calls onCreate with that exact number', async () => {
  const onCreate = vi.fn();
  render(<SlotCatalogGrid quota={5} entries={[]} onSelect={vi.fn()} onCreate={onCreate} canCreate />);
  await userEvent.click(screen.getByRole('button', { name: /Buat Nomor 3/ }));
  expect(onCreate).toHaveBeenCalledWith(3);
});

test('without a quota, exactly one create box is shown after existing slots', () => {
  const entries: SlotCatalogEntry[] = [{ slot_number: 1, status: 'open', documentation_complete: false }];
  render(<SlotCatalogGrid entries={entries} onSelect={vi.fn()} onCreate={vi.fn()} canCreate />);
  expect(screen.getByRole('button', { name: /Buat Nomor 2/ })).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /Nomor 3/ })).not.toBeInTheDocument();
});

test('canCreate=false disables the create box but still labels it', () => {
  render(<SlotCatalogGrid entries={[]} onSelect={vi.fn()} onCreate={vi.fn()} canCreate={false} />);
  expect(screen.getByRole('button', { name: /Buat Nomor 1/ })).toBeDisabled();
});

test('a completed slot whose documentation is no longer complete is flagged distinctly from a normal completed slot', () => {
  const entries: SlotCatalogEntry[] = [
    { slot_number: 1, status: 'completed', documentation_complete: true },
    { slot_number: 2, status: 'completed', documentation_complete: false },
  ];
  render(<SlotCatalogGrid quota={2} entries={entries} onSelect={vi.fn()} onCreate={vi.fn()} canCreate />);
  const normalCompleted = screen.getByRole('button', { name: /Nomor 1 - Selesai$/ });
  const flaggedCompleted = screen.getByRole('button', { name: /Nomor 2 - Selesai \(dokumen berkurang\)/ });
  expect(normalCompleted.className).not.toBe(flaggedCompleted.className);
});

test('quota lower than the number of existing entries still renders every entry', () => {
  const entries: SlotCatalogEntry[] = [
    { slot_number: 1, status: 'open', documentation_complete: false },
    { slot_number: 2, status: 'open', documentation_complete: false },
    { slot_number: 3, status: 'open', documentation_complete: false },
  ];
  render(<SlotCatalogGrid quota={1} entries={entries} onSelect={vi.fn()} onCreate={vi.fn()} canCreate />);
  expect(screen.getByRole('button', { name: /Nomor 1/ })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: /Nomor 2/ })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: /Nomor 3/ })).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /Buat Nomor/ })).not.toBeInTheDocument();
});

test('announces the selected catalog number without changing its action', async () => {
  const onSelect = vi.fn();
  const entries: SlotCatalogEntry[] = [
    { slot_number: 1, status: 'open', documentation_complete: false },
    { slot_number: 2, status: 'linked', documentation_complete: false },
  ];
  render(<SlotCatalogGrid quota={2} entries={entries} selectedNumber={2} onSelect={onSelect} onCreate={vi.fn()} canCreate />);

  expect(screen.getByRole('button', { name: /Nomor 1/ })).toHaveAttribute('aria-pressed', 'false');
  expect(screen.getByRole('button', { name: /Nomor 2/ })).toHaveAttribute('aria-pressed', 'true');
  await userEvent.click(screen.getByRole('button', { name: /Nomor 2/ }));
  expect(onSelect).toHaveBeenCalledWith(2);
});
