import { act, renderHook } from '@testing-library/react';
import type { ReactNode } from 'react';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { expect, test } from 'vitest';
import { useMapUrlState } from './useMapUrlState';

function setup(initial: string) {
  const wrapper = ({ children }: { children: ReactNode }) => <MemoryRouter initialEntries={[initial]}>{children}</MemoryRouter>;
  return renderHook(() => ({ map: useMapUrlState(), loc: useLocation() }), { wrapper });
}

test('defaults when the URL has no map params', () => {
  const { result } = setup('/map');
  expect(result.current.map.state).toEqual({ base: 'map', color: 'zone' });
});

test('reads map params and ignores invalid ones', () => {
  const { result } = setup('/map?base=hyb&color=progress&slot=s1&site=hl:abc&stage=3&c=110.5,-7.2&z=9.5');
  expect(result.current.map.state).toEqual({ base: 'hyb', color: 'progress', slot: 's1', site: 'hl:abc', stage: 3, center: [110.5, -7.2], zoom: 9.5 });
  const bad = setup('/map?base=x&color=y&stage=9&c=a,b&z=nope&site=geo:1');
  expect(bad.result.current.map.state).toEqual({ base: 'map', color: 'zone' });
});

test('writes base, color and slot without dropping existing filters', () => {
  const { result } = setup('/map?program_id=p1&regency_id=r1&district=D');
  act(() => result.current.map.update({ base: 'hyb', color: 'zone', slot: 'slot-1' }));
  const p = new URLSearchParams(result.current.loc.search);
  expect(p.get('base')).toBe('hyb');
  expect(p.get('color')).toBe('zone');
  expect(p.get('slot')).toBe('slot-1');
  expect(p.get('program_id')).toBe('p1');
  expect(p.get('regency_id')).toBe('r1');
  expect(p.get('district')).toBe('D');
});

test('consecutive updates in one tick both apply', () => {
  const { result } = setup('/map?schedule_id=s1');
  act(() => {
    result.current.map.update({ base: 'sat' });
    result.current.map.update({ stage: 2 });
  });
  const p = new URLSearchParams(result.current.loc.search);
  expect(p.get('base')).toBe('sat');
  expect(p.get('stage')).toBe('2');
  expect(p.get('schedule_id')).toBe('s1');
});

test('undefined removes a param', () => {
  const { result } = setup('/map?slot=s1&stage=2&evidence_status=complete');
  act(() => result.current.map.update({ slot: undefined, stage: undefined }));
  const p = new URLSearchParams(result.current.loc.search);
  expect(p.has('slot')).toBe(false);
  expect(p.has('stage')).toBe(false);
  expect(p.get('evidence_status')).toBe('complete');
});

test('site only accepts hl: ids; geo: ids never reach the URL', () => {
  const { result } = setup('/map?site=hl:old&allocation_status=open');
  act(() => result.current.map.update({ site: 'geo:123' }));
  let p = new URLSearchParams(result.current.loc.search);
  expect(p.has('site')).toBe(false);
  expect(result.current.map.state.site).toBeUndefined();
  expect(p.get('allocation_status')).toBe('open');
  act(() => result.current.map.update({ site: 'hl:xyz' }));
  p = new URLSearchParams(result.current.loc.search);
  expect(p.get('site')).toBe('hl:xyz');
});

test('center and zoom round-trip with bounded precision', () => {
  const { result } = setup('/map');
  act(() => result.current.map.update({ center: [110.123456789, -7.5], zoom: 8.123456 }));
  expect(result.current.map.state.center).toEqual([110.12346, -7.5]);
  expect(result.current.map.state.zoom).toBe(8.12);
});

test('a filter edit and a map move in the same tick both apply, in either order', () => {
  const { result } = setup('/map?schedule_id=s1');
  act(() => {
    result.current.map.edit((next) => next.set('zone_id', 'z1'));
    result.current.map.update({ center: [108.1, -6.4], zoom: 9 });
  });
  let p = new URLSearchParams(result.current.loc.search);
  expect(p.get('zone_id')).toBe('z1');
  expect(p.get('c')).toBe('108.1,-6.4');
  expect(p.get('schedule_id')).toBe('s1');
  act(() => {
    result.current.map.update({ zoom: 10 });
    result.current.map.edit((next) => next.delete('schedule_id'));
  });
  p = new URLSearchParams(result.current.loc.search);
  expect(p.get('z')).toBe('10');
  expect(p.get('zone_id')).toBe('z1');
  expect(p.has('schedule_id')).toBe(false);
});
