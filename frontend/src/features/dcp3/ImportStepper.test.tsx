import { render, screen, within } from '@testing-library/react';
import { expect, test } from 'vitest';
import { ImportStepper } from './ImportStepper';

test('announces active and completed import steps', () => {
  render(<ImportStepper currentStep={2} steps={[
    { label: 'Jadwal', description: 'Pilih jadwal aktif' },
    { label: 'Workbook', description: 'Unggah file DCP3' },
    { label: 'Pemetaan', description: 'Cocokkan kolom' },
    { label: 'Tinjau', description: 'Periksa dan import' },
  ]} />);

  expect(screen.getByRole('list', { name: 'Tahapan import DCP3' })).toBeInTheDocument();
  expect(screen.getByText('Workbook').closest('li')).toHaveAttribute('aria-current', 'step');
  const completedStep = screen.getByText('Jadwal').closest('li')!;
  expect(completedStep).toHaveAttribute('data-state', 'complete');
  expect(within(completedStep).getByText('1')).toBeVisible();
});
