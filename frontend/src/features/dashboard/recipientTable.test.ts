import { describe, expect, test } from 'vitest';
import { buildPageItems, summarizeEvidence } from './recipientTable';
import type { EvidenceSlot } from './types';

describe('buildPageItems', () => {
  test('returns every page when the result fits in one compact window', () => {
    expect(buildPageItems(1, 5)).toEqual([1, 2, 3, 4, 5]);
  });

  test('keeps the first pages visible and collapses the distant tail', () => {
    expect(buildPageItems(2, 12)).toEqual([1, 2, 3, 4, 5, 'ellipsis-end', 12]);
  });

  test('keeps a centered window between both boundaries', () => {
    expect(buildPageItems(6, 12)).toEqual([1, 'ellipsis-start', 5, 6, 7, 'ellipsis-end', 12]);
  });

  test('keeps the final pages visible and collapses the distant head', () => {
    expect(buildPageItems(11, 12)).toEqual([1, 'ellipsis-start', 8, 9, 10, 11, 12]);
  });
});

describe('summarizeEvidence', () => {
  const slot = (overrides: Partial<EvidenceSlot>): EvidenceSlot => ({
    slot_code: 'portrait', label: 'Foto penerima', is_required: true,
    min_files: 1, accepted_files: 0, complete: false, ...overrides,
  });

  test('uses required slots for status while still counting optional slots', () => {
    expect(summarizeEvidence([
      slot({ slot_code: 'portrait', complete: true, accepted_files: 1 }),
      slot({ slot_code: 'handover', label: 'BAST' }),
      slot({ slot_code: 'detail', label: 'Detail paket', is_required: false, complete: true, accepted_files: 1 }),
    ])).toEqual({
      state: 'partial', requiredComplete: 1, requiredTotal: 2,
      optionalComplete: 1, optionalTotal: 1,
    });
  });

  test('reports empty when configured evidence has no accepted files', () => {
    expect(summarizeEvidence([slot({})])).toEqual({
      state: 'empty', requiredComplete: 0, requiredTotal: 1,
      optionalComplete: 0, optionalTotal: 0,
    });
  });

  test('reports not configured when there are no custom slots', () => {
    expect(summarizeEvidence([])).toEqual({
      state: 'not-configured', requiredComplete: 0, requiredTotal: 0,
      optionalComplete: 0, optionalTotal: 0,
    });
  });

  test('optional missing files do not block required completeness', () => {
    expect(summarizeEvidence([
      slot({ complete: true, accepted_files: 1 }),
      slot({ slot_code: 'detail', is_required: false }),
    ]).state).toBe('complete');
  });
});
