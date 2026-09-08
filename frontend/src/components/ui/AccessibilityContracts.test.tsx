import { render, screen } from '@testing-library/react';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { expect, test } from 'vitest';
import { Button } from './button';
import { Dialog, DialogContent, DialogTitle } from './dialog';
import { Input } from './input';
import { Select, SelectTrigger, SelectValue } from './select';

const globalStyles = readFileSync(resolve(process.cwd(), 'src/styles/global.css'), 'utf8');

test('provides 44px touch targets for primary controls', () => {
  const { container } = render(
    <>
      <Button>Save</Button>
      <Input aria-label="Nama" />
      <Select>
        <SelectTrigger>
          <SelectValue placeholder="Pilih wilayah" />
        </SelectTrigger>
      </Select>
      <Dialog open>
        <DialogContent>
          <DialogTitle>Ubah data</DialogTitle>
        </DialogContent>
      </Dialog>
    </>,
  );

  expect(container.querySelector('[data-slot="button"]')).toHaveClass('h-11');
  expect(container.querySelector('[data-slot="input"]')).toHaveClass('h-11');
  expect(container.querySelector('[data-slot="select-trigger"]')).toHaveClass('h-11');
  expect(screen.getByRole('button', { name: 'Close' })).toHaveClass('size-11');
});

test('disables non-essential motion when reduced motion is preferred', () => {
  expect(globalStyles).toContain('@media (prefers-reduced-motion: reduce)');
  expect(globalStyles).toMatch(/animation:\s*none !important/);
  expect(globalStyles).toMatch(/transition:\s*none !important/);
});
