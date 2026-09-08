import { render, screen } from '@testing-library/react';
import { expect, test } from 'vitest';
import { StatusBadge } from './StatusBadge';

test('uses visible text alongside a status cue', () => {
  render(<StatusBadge active />);

  expect(screen.getByText('Aktif')).toBeInTheDocument();
  expect(screen.getByText('Aktif').closest('[data-slot="badge"]')?.querySelector('[aria-hidden="true"]')).toBeInTheDocument();
});

test('supports custom inactive status text', () => {
  render(<StatusBadge active={false} inactiveText="Tidak aktif" />);

  expect(screen.getByText('Tidak aktif')).toBeInTheDocument();
});
