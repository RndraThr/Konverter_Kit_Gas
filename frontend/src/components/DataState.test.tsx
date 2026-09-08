import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { DataState } from './DataState';

test('gives an error state a retry action', async () => {
  const retry = vi.fn();
  const user = userEvent.setup();

  render(<DataState kind="error" title="Data gagal dimuat" description="Periksa koneksi." action={{ label: 'Coba lagi', onClick: retry }} />);

  await user.click(screen.getByRole('button', { name: 'Coba lagi' }));
  expect(retry).toHaveBeenCalledOnce();
});

test('announces loading state to assistive technology', () => {
  render(<DataState kind="loading" title="Memuat data" description="Tunggu sebentar." />);

  expect(screen.getByRole('status')).toHaveTextContent('Memuat data');
});
