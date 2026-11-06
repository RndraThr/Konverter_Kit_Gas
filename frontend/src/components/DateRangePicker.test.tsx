import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { DateRangePicker } from './DateRangePicker';

async function openCalendar(onChange = vi.fn()) {
  render(<><DateRangePicker aria-label="Rentang tanggal" from="2026-11-06" to="2026-11-06" onChange={onChange} /><button type="button">Di luar</button></>);
  await userEvent.click(screen.getByRole('button', { name: 'Rentang tanggal' }));
  await screen.findAllByRole('grid');
  return { onChange, day: (date: number) => screen.getByRole('button', { name: new RegExp(`, ${date} November 2026`) }) };
}

describe('DateRangePicker', () => {
  it('starts a new range on the first click and stays open after the second', async () => {
    const { onChange, day } = await openCalendar();
    await userEvent.click(day(10));
    expect(onChange).not.toHaveBeenCalled();
    await userEvent.click(day(12));
    expect(onChange).toHaveBeenCalledWith('2026-11-10', '2026-11-12');
    expect(screen.getAllByRole('grid')).toHaveLength(2);
  });

  it('takes the same day twice as one day', async () => {
    const { onChange, day } = await openCalendar();
    await userEvent.click(day(10));
    await userEvent.click(day(10));
    expect(onChange).toHaveBeenCalledWith('2026-11-10', '2026-11-10');
    expect(screen.getAllByRole('grid')).toHaveLength(2);
  });

  it('closes when clicking outside', async () => {
    await openCalendar();
    await userEvent.click(screen.getByRole('button', { name: 'Di luar', hidden: true }));
    await vi.waitFor(() => expect(screen.queryAllByRole('grid')).toHaveLength(0));
  });
});
