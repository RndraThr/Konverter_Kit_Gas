import { render, screen } from '@testing-library/react';
import { expect, test } from 'vitest';
import { PageHeader } from './PageHeader';

test('presents page context and actions in a semantic header', () => {
  render(
    <PageHeader
      eyebrow="Pengaturan"
      title="Pengguna"
      description="Kelola akun dan akses pengguna."
      context={<p>12 pengguna aktif</p>}
      actions={<button type="button">Tambah pengguna</button>}
    />,
  );

  expect(screen.getByRole('banner')).toHaveTextContent('Pengaturan');
  expect(screen.getByRole('heading', { level: 1, name: 'Pengguna' })).toBeInTheDocument();
  expect(screen.getByText('12 pengguna aktif')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Tambah pengguna' })).toBeInTheDocument();
});
