import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { describe, expect, it } from 'vitest';
import { MoneyInput } from './MoneyInput';

function Field({ start = 0 }: { start?: number }) {
  const [value, setValue] = useState(start);
  return <><MoneyInput value={value} onChange={setValue} /><output>{value}</output></>;
}

describe('MoneyInput', () => {
  it('groups thousands while typing and keeps the plain number', async () => {
    render(<Field />);
    await userEvent.type(screen.getByRole('textbox'), '200000');
    expect(screen.getByRole('textbox')).toHaveValue('200.000');
    expect(screen.getByRole('status')).toHaveTextContent('200000');
  });

  it('ignores letters and clears to empty', async () => {
    render(<Field start={1500} />);
    await userEvent.type(screen.getByRole('textbox'), 'x');
    expect(screen.getByRole('textbox')).toHaveValue('1.500');
    await userEvent.clear(screen.getByRole('textbox'));
    expect(screen.getByRole('textbox')).toHaveValue('');
  });
});
