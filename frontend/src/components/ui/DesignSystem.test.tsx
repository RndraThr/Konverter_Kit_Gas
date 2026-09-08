import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test } from 'vitest';
import { Button } from './button';
import { Sheet, SheetContent, SheetTitle, SheetTrigger } from './sheet';

test('provides accessible Konkit controls', async () => {
  render(
    <Sheet>
      <SheetTrigger render={<Button variant="outline" />}>Buka filter</SheetTrigger>
      <SheetContent>
        <SheetTitle>Filter data</SheetTitle>
      </SheetContent>
    </Sheet>,
  );

  await userEvent.click(screen.getByRole('button', { name: 'Buka filter' }));
  expect(screen.getByRole('dialog', { name: 'Filter data' })).toBeInTheDocument();
});
