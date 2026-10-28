import { useCallback, useEffect, useMemo, useRef } from 'react';
import { useSearchParams } from 'react-router-dom';
import type { BasemapMode, ColorMode, MapUrlState } from './types';

const BASES: BasemapMode[] = ['map', 'sat', 'hyb'];
const COLORS: ColorMode[] = ['zone', 'evidence', 'progress', 'recipients'];
const SITE_PREFIX = 'hl:';

const round = (n: number, digits: number) => Number(n.toFixed(digits));

function parseState(params: URLSearchParams): MapUrlState {
  const base = params.get('base') as BasemapMode | null;
  const color = params.get('color') as ColorMode | null;
  const state: MapUrlState = {
    base: base && BASES.includes(base) ? base : 'map',
    color: color && COLORS.includes(color) ? color : 'zone',
  };
  const c = params.get('c')?.split(',');
  if (c?.length === 2 && c.every((v) => v.trim() !== '')) {
    const lng = Number(c[0]);
    const lat = Number(c[1]);
    if (Number.isFinite(lng) && Number.isFinite(lat)) state.center = [lng, lat];
  }
  const z = params.get('z');
  if (z !== null && z.trim() !== '' && Number.isFinite(Number(z))) state.zoom = Number(z);
  const slot = params.get('slot');
  if (slot) state.slot = slot;
  const site = params.get('site');
  if (site?.startsWith(SITE_PREFIX)) state.site = site;
  const stage = Number(params.get('stage'));
  if (Number.isInteger(stage) && stage >= 1 && stage <= 5) state.stage = stage;
  return state;
}

function setOrDelete(params: URLSearchParams, key: string, value: string | undefined) {
  if (value === undefined || value === '') params.delete(key);
  else params.set(key, value);
}

// Map view state lives in the URL next to the existing filter params (regency_id,
// program_id, ...). Only keys that belong to the map are ever touched.
export function useMapUrlState(): { state: MapUrlState; update(patch: Partial<MapUrlState>): void; edit(change: (next: URLSearchParams) => void): void } {
  const [params, setParams] = useSearchParams();
  const query = params.toString();
  const state = useMemo(() => parseState(new URLSearchParams(query)), [query]);
  // react-router's functional updater reads the committed location, so two update()
  // calls in the same tick would overwrite each other. Track the latest value here.
  const latest = useRef(query);
  useEffect(() => { latest.current = query; }, [query]);

  // Every URL write goes through here (also the page's filters), so a map move and a filter change
  // in the same tick never overwrite each other with a stale copy of the query.
  const edit = useCallback((change: (next: URLSearchParams) => void) => {
    const next = new URLSearchParams(latest.current);
    change(next);
    latest.current = next.toString();
    setParams(next, { replace: true });
  }, [setParams]);

  const update = useCallback((patch: Partial<MapUrlState>) => edit((next) => {
    if ('base' in patch) setOrDelete(next, 'base', patch.base);
    if ('color' in patch) setOrDelete(next, 'color', patch.color);
    if ('slot' in patch) setOrDelete(next, 'slot', patch.slot);
    // Only handover-location ids (`hl:`) may live in the URL; anything else
    // (e.g. `geo:` clusters) clears the param and is kept in component state.
    if ('site' in patch) setOrDelete(next, 'site', patch.site?.startsWith(SITE_PREFIX) ? patch.site : undefined);
    if ('stage' in patch) setOrDelete(next, 'stage', patch.stage === undefined ? undefined : String(patch.stage));
    if ('center' in patch) setOrDelete(next, 'c', patch.center ? `${round(patch.center[0], 5)},${round(patch.center[1], 5)}` : undefined);
    if ('zoom' in patch) setOrDelete(next, 'z', patch.zoom === undefined ? undefined : String(round(patch.zoom, 2)));
  }), [edit]);

  return { state, update, edit };
}
