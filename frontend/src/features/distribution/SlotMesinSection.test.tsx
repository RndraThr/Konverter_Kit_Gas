import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { expect, test, vi } from 'vitest';
import { PermissionsProvider } from '../../lib/permissions';
import { SlotMesinSection } from './SlotMesinSection';
import type { DistributionSlot } from './types';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));

const slot: DistributionSlot = {
  id: 'slot-1', schedule_id: 'schedule-1', slot_number: 7, status: 'linked',
  machine_option_code: 'MSN-001', machine_serial_number: 'SN-MSN-1',
  hose_option_code: 'HSE-001', hose_serial_number: 'SN-HSE-1', converter_serial_number: 'SN-CNV-1',
  documentation: [], created_at: '2026-09-20T00:00:00Z', updated_at: '2026-09-20T00:00:00Z',
};

function renderSection(overrides: Partial<DistributionSlot> = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={['*']}><SlotMesinSection slot={{ ...slot, ...overrides }} onChanged={vi.fn()} /></PermissionsProvider></QueryClientProvider>);
}

test('renders equipment summary fields from the slot fixture', () => {
  renderSection();
  const section = screen.getByLabelText('POS Mesin');
  expect(section).toBeVisible();
  expect(screen.getByText('Nomor bagi #7')).toBeVisible();
  expect(screen.getByText('MSN-001')).toBeVisible();
  expect(screen.getByText('SN-MSN-1')).toBeVisible();
  expect(screen.getByText('HSE-001')).toBeVisible();
  expect(screen.getByText('SN-HSE-1')).toBeVisible();
  expect(screen.getByText('SN-CNV-1')).toBeVisible();
});

test('shows a placeholder dash when equipment fields are empty', () => {
  renderSection({ machine_option_code: '', machine_serial_number: '', hose_option_code: '', hose_serial_number: '', converter_serial_number: '' });
  expect(screen.getAllByText('-')).toHaveLength(5);
});
