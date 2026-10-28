import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { MilestoneFilter } from './MilestoneFilter';

const counts = { 1: 4, 2: 3, 3: 10, 4: 2, 5: 1 };

test('shows the count per milestone and marks the selected stage with aria-pressed', () => {
  render(<MilestoneFilter counts={counts} selected={3} onChange={() => {}} />);
  const m3 = screen.getByRole('button', { name: /M3 Diserahkan: 10 titik/ });
  expect(m3).toHaveAttribute('aria-pressed', 'true');
  expect(screen.getByRole('button', { name: /M1 Terjadwal: 4 titik/ })).toHaveAttribute('aria-pressed', 'false');
  expect(screen.getByRole('button', { name: /M5 BA ditandatangani: 1 titik/ })).toBeVisible();
  expect(screen.getByRole('button', { name: /Semua titik/ })).toHaveAttribute('aria-pressed', 'false');
});

test('toggles a stage on and off and resets with "Semua titik"', async () => {
  const onChange = vi.fn();
  const { rerender } = render(<MilestoneFilter counts={counts} onChange={onChange} />);
  expect(screen.getByRole('button', { name: /Semua titik: 20 titik/ })).toHaveAttribute('aria-pressed', 'true');
  await userEvent.click(screen.getByRole('button', { name: /M2 Penerima tertaut/ }));
  expect(onChange).toHaveBeenLastCalledWith(2);

  rerender(<MilestoneFilter counts={counts} selected={2} onChange={onChange} />);
  await userEvent.click(screen.getByRole('button', { name: /M2 Penerima tertaut/ }));
  expect(onChange).toHaveBeenLastCalledWith(undefined);
  await userEvent.click(screen.getByRole('button', { name: /Semua titik/ }));
  expect(onChange).toHaveBeenLastCalledWith(undefined);
});
