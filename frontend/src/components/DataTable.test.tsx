import { render, screen } from '@testing-library/react';
import { expect, test } from 'vitest';
import { DataTable } from './DataTable';

test('labels a scrollable data region', () => {
  render(<DataTable label="Daftar pengguna"><tbody><tr><td>Admin</td></tr></tbody></DataTable>);

  expect(screen.getByRole('region', { name: 'Daftar pengguna' })).toBeInTheDocument();
  expect(screen.getByRole('table', { name: 'Daftar pengguna' })).toBeInTheDocument();
});

test('applies the requested minimum table width', () => {
  render(<DataTable label="Daftar pengguna" minimumWidth={720}><tbody><tr><td>Admin</td></tr></tbody></DataTable>);

  expect(screen.getByRole('table')).toHaveStyle({ minWidth: '720px' });
});

test('keeps the established minimum width by default', () => {
  render(<DataTable label="Daftar pengguna"><tbody><tr><td>Admin</td></tr></tbody></DataTable>);

  expect(screen.getByRole('table')).toHaveStyle({ minWidth: '720px' });
});
