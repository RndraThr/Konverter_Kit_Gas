import { expect, test } from 'vitest';
import { formatSettingsPreview } from './settingsPresentation';

test('formats slash dates in the selected timezone', () => {
  expect(formatSettingsPreview('2026-10-09T09:15:00Z', 'Asia/Jakarta', '02/01/2006', 'id-ID'))
    .toContain('09/10/2026');
});

test('formats Indonesian long dates in the selected timezone', () => {
  expect(formatSettingsPreview('2026-10-09T09:15:00Z', 'Asia/Makassar', '02 January 2006', 'id-ID'))
    .toContain('09 Oktober 2026');
});
