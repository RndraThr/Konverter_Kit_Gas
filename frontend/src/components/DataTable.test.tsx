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

test('makes the constrained scroll container the labelled focus region', () => {
  render(<DataTable label="Daftar pengguna" minimumWidth={720}><tbody><tr><td>Admin</td></tr></tbody></DataTable>);

  const region = screen.getByRole('region', { name: 'Daftar pengguna' });
  expect(region).toHaveAttribute('data-slot', 'table-container');
  expect(region).toHaveAttribute('tabindex', '0');
  expect(region).toHaveClass('overflow-x-auto', '[&::-webkit-scrollbar]:h-2', '[&::-webkit-scrollbar-thumb]:bg-muted-foreground/35');
  expect(screen.getByRole('table')).toHaveStyle({ minWidth: '720px' });
});

test('normalizes semantic table markup through the Shadcn table primitives', () => {
  render(<DataTable label="Daftar pengguna"><thead><tr><th>Nama</th><th className="text-right">Aksi</th></tr></thead><tbody><tr><td>Admin</td><td className="text-right">Edit</td></tr></tbody></DataTable>);

  const [nameHeader, actionHeader] = screen.getAllByRole('columnheader');
  const [nameCell, actionCell] = screen.getAllByRole('cell');

  expect(nameHeader).toHaveAttribute('data-slot', 'table-head');
  expect(nameHeader).toHaveClass('text-left', 'px-4', 'text-xs', 'font-bold', 'uppercase');
  expect(nameCell).toHaveAttribute('data-slot', 'table-cell');
  expect(nameCell).toHaveClass('px-4', 'py-3');
  expect(actionHeader).toHaveClass('text-right');
  expect(actionCell).toHaveClass('text-right');
  expect(nameCell.closest('tr')).toHaveAttribute('data-slot', 'table-row');
});

test('normalizes table sections nested in a React fragment', () => {
  render(<DataTable label="Daftar pengguna"><><thead><tr><th>Nama</th></tr></thead><tbody><tr><td>Admin</td></tr></tbody></></DataTable>);

  expect(screen.getByRole('columnheader', { name: 'Nama' })).toHaveAttribute('data-slot', 'table-head');
  expect(screen.getByRole('cell', { name: 'Admin' })).toHaveAttribute('data-slot', 'table-cell');
});
