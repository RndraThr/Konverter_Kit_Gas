import { useState } from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test } from 'vitest';
import { SearchableSelect } from './SearchableSelect';

const options = [
  { value: '', label: 'Semua kabupaten' },
  { value: 'bka', label: 'BKA - KAB. BANGKA' },
  { value: 'tjt', label: 'TJT - KAB. TANJUNG JABUNG TIMUR' },
  { value: 'mjb', label: 'MJB - KAB. MUARO JAMBI' },
];

function Harness({ initial = '' }: { initial?: string }) {
  const [value, setValue] = useState(initial);
  return <>
    <SearchableSelect aria-label="Kabupaten" options={options} value={value} onValueChange={setValue} placeholder="Cari kabupaten" />
    <output aria-label="Nilai">{value}</output>
  </>;
}

test('filters options by typed text and selects one', async () => {
  render(<Harness />);
  const input = screen.getByRole('combobox', { name: 'Kabupaten' });
  await userEvent.type(input, 'jambi');

  expect(await screen.findByRole('option', { name: 'MJB - KAB. MUARO JAMBI' })).toBeVisible();
  expect(screen.queryByRole('option', { name: 'BKA - KAB. BANGKA' })).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole('option', { name: 'MJB - KAB. MUARO JAMBI' }));

  expect(screen.getByLabelText('Nilai')).toHaveTextContent('mjb');
  expect(input).toHaveValue('MJB - KAB. MUARO JAMBI');
});

test('shows the current value and can go back to the empty-value option', async () => {
  render(<Harness initial="bka" />);
  const input = screen.getByRole('combobox', { name: 'Kabupaten' });
  expect(input).toHaveValue('BKA - KAB. BANGKA');

  await userEvent.clear(input);
  await userEvent.type(input, 'semua');
  await userEvent.click(await screen.findByRole('option', { name: 'Semua kabupaten' }));
  expect(screen.getByLabelText('Nilai')).toBeEmptyDOMElement();
});

test('opens with every option listed even when one is already chosen', async () => {
  render(<Harness initial="bka" />);
  await userEvent.click(screen.getByRole('combobox', { name: 'Kabupaten' }));
  expect(await screen.findAllByRole('option')).toHaveLength(options.length);
});

test('says so when nothing matches', async () => {
  render(<Harness />);
  await userEvent.type(screen.getByRole('combobox', { name: 'Kabupaten' }), 'zzz');
  expect(await screen.findByText('Tidak ditemukan.')).toBeVisible();
});
