import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { SlotMesinCreate } from './SlotMesinCreate';

function renderCreate() {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}>
    <SlotMesinCreate
      scheduleID="schedule-1"
      slotNumber={3}
      machineOptions={[{ code: 'm1', brand: 'SHARK', type: 'SPWP 80-30' }]}
      hoseOptions={[{ code: 'h1', brand: 'TRILIUNHOSE', spec: '6m' }]}
      converterOptions={[{ code: 'ergas', brand: 'ERGAS' }]}
      onCreated={vi.fn()}
    />
  </QueryClientProvider>);
}

test('shows the target slot number and a save action', () => {
	renderCreate();
	expect(screen.getByText('Nomor bagi #3')).toBeVisible();
	expect(screen.getByLabelText('Tanggal distribusi')).toBeRequired();
	expect(screen.getByRole('button', { name: 'Simpan Nomor Bagi' })).toBeVisible();
});

test('offers barcode scanning only for serial numbers that exist', () => {
  renderCreate();
  expect(screen.getByRole('button', { name: 'Scan Serial Number Mesin' })).toBeVisible();
  expect(screen.getByRole('button', { name: 'Scan Serial Number Konkit/Reducer' })).toBeVisible();
  expect(screen.queryByRole('button', { name: 'Scan Serial Number Selang' })).not.toBeInTheDocument();
  expect(screen.getByLabelText('Serial Number Selang')).toBeDisabled();
  expect(screen.getByLabelText('Serial Number Selang')).toHaveValue('-');
});

test('offers a konkit/reducer brand selector from the package options', async () => {
  renderCreate();
  await userEvent.click(screen.getByRole('combobox', { name: 'Merk Konkit/Reducer' }));
  expect(await screen.findByRole('option', { name: 'ERGAS' })).toBeVisible();
});

test('uppercases serial numbers while typing', async () => {
  renderCreate();
  await userEvent.type(screen.getByLabelText('Serial Number Mesin'), 'ms-a1');
  await userEvent.type(screen.getByLabelText('Serial Number Konkit/Reducer'), 'cv-c3');

  expect(screen.getByLabelText('Serial Number Mesin')).toHaveValue('MS-A1');
  expect(screen.getByLabelText('Serial Number Selang')).toHaveValue('-');
  expect(screen.getByLabelText('Serial Number Konkit/Reducer')).toHaveValue('CV-C3');
});

test('places the hose fields after the converter fields', () => {
  const { container } = renderCreate();
  const labels = Array.from(container.querySelectorAll('label')).map((label) => label.textContent);

  expect(labels.indexOf('Merk/Spesifikasi Selang')).toBeGreaterThan(labels.indexOf('Serial Number Konkit/Reducer'));
  expect(labels.indexOf('Serial Number Selang')).toBeGreaterThan(labels.indexOf('Merk/Spesifikasi Selang'));
});
