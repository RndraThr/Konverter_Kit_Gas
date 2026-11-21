import type { Phase } from './types';

export type Days = Record<string, Phase>;
/** A run of consecutive days of one phase, like a colored bar of the sheet. */
export type Block = { phase: Phase; start: string; end: string };

export const addDays = (day: string, n: number) => {
  const date = new Date(`${day}T00:00:00Z`);
  date.setUTCDate(date.getUTCDate() + n);
  return date.toISOString().slice(0, 10);
};
export const daysBetween = (from: string, to: string) =>
  Math.round((Date.parse(`${to}T00:00:00Z`) - Date.parse(`${from}T00:00:00Z`)) / 86_400_000);

/** Day number of each cell: its position among the row's days of the same phase (1, 2, 3…). */
export function dayNumbers(days: Days): Record<string, number> {
  const counters: Record<string, number> = {};
  const out: Record<string, number> = {};
  Object.keys(days).sort().forEach((day) => {
    counters[days[day]] = (counters[days[day]] ?? 0) + 1;
    out[day] = counters[days[day]];
  });
  return out;
}

export function blocksOf(days: Days): Block[] {
  const blocks: Block[] = [];
  Object.keys(days).sort().forEach((day) => {
    const last = blocks[blocks.length - 1];
    if (last && last.phase === days[day] && addDays(last.end, 1) === day) last.end = day;
    else blocks.push({ phase: days[day], start: day, end: day });
  });
  return blocks;
}

export const blockAt = (days: Days, day: string) => blocksOf(days).find((b) => b.start <= day && day <= b.end);

const spanOf = (start: string, end: string) => Array.from({ length: daysBetween(start, end) + 1 }, (_, i) => addDays(start, i));

/** Moves one block by delta days; it covers whatever lies where it lands. */
export function moveBlock(days: Days, block: Block, delta: number): Days {
  const next = { ...days };
  spanOf(block.start, block.end).forEach((day) => { delete next[day]; });
  spanOf(addDays(block.start, delta), addDays(block.end, delta)).forEach((day) => { next[day] = block.phase; });
  return next;
}

/** Drags one end of a block to day; the block keeps at least one day. */
export function resizeBlock(days: Days, block: Block, edge: 'start' | 'end', day: string): Days {
  const start = edge === 'start' ? (day > block.end ? block.end : day) : block.start;
  const end = edge === 'end' ? (day < block.start ? block.start : day) : block.end;
  const next = { ...days };
  spanOf(block.start, block.end).forEach((d) => { delete next[d]; });
  spanOf(start, end).forEach((d) => { next[d] = block.phase; });
  return next;
}

/** Moves the whole kabupaten schedule by delta days. */
export function shiftAll(days: Days, delta: number): Days {
  return Object.fromEntries(Object.entries(days).map(([day, phase]) => [addDays(day, delta), phase]));
}

/** Moves every day between from and to (inclusive) by delta days; what lies where they land is covered. */
export function moveRange(days: Days, from: string, to: string, delta: number): Days {
  if (delta === 0) return days;
  const moving = Object.entries(days).filter(([day]) => day >= from && day <= to);
  const next = { ...days };
  moving.forEach(([day]) => { delete next[day]; });
  moving.forEach(([day, phase]) => { next[addDays(day, delta)] = phase; });
  return next;
}

/** Clears every day between from and to (inclusive). */
export function clearRange(days: Days, from: string, to: string): Days {
  return Object.fromEntries(Object.entries(days).filter(([day]) => day < from || day > to));
}
