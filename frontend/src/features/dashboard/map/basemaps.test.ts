import { expect, test } from 'vitest';
import type maplibregl from 'maplibre-gl';
import { applyBasemap, markStyleReady, withApiKey } from './basemaps';
import type { MapConfig } from './types';

const config: MapConfig = {
  style_url: 'https://style.test/liberty',
  satellite_tiles: 'https://sat.test/{z}/{y}/{x}',
  hybrid_overlay_tiles: ['https://ov.test/a/{z}/{y}/{x}', 'https://ov.test/b/{z}/{y}/{x}'],
  attribution: 'attr',
  api_key: '',
};

function fakeMap(layerIds: string[], loaded = true) {
  const onceLoad: Array<() => void> = [];
  let isLoaded = loaded;
  let busy = false;
  const layers = layerIds.map((id) => ({ id, type: 'fill' }));
  const vis: Record<string, string> = {};
  const sources = new Set<string>();
  const added: string[] = [];
  const map = {
    isStyleLoaded: () => isLoaded && !busy,
    once: (_t: string, fn: () => void) => { onceLoad.push(fn); },
    getStyle: () => (isLoaded ? { layers } : undefined),
    getLayer: (id: string) => layers.find((l) => l.id === id),
    getSource: (id: string) => (sources.has(id) ? {} : undefined),
    addSource: (id: string) => {
      if (!isLoaded) throw new Error('Style is not done loading');
      sources.add(id);
    },
    addLayer: (l: { id: string }, before?: string) => {
      const at = before ? layers.findIndex((x) => x.id === before) : -1;
      const entry = { id: l.id, type: 'raster' };
      if (at >= 0) layers.splice(at, 0, entry); else layers.push(entry);
      added.push(l.id);
    },
    setLayoutProperty: (id: string, prop: string, value: string) => { if (prop === 'visibility') vis[id] = value; },
  };
  const fire = () => { isLoaded = true; onceLoad.splice(0).forEach((fn) => fn()); };
  return { map: map as unknown as maplibregl.Map, vis, added, layers, fire, setBusy: (v: boolean) => { busy = v; }, onceCount: () => onceLoad.length };
}

const base = ['background', 'water', 'road'];

test('sat hides every base style layer and shows imagery without touching konkit-dots', () => {
  const { map, vis } = fakeMap([...base, 'konkit-dots']);
  applyBasemap(map, 'sat', config);
  for (const id of base) expect(vis[id]).toBe('none');
  expect(vis['konkit-img']).toBe('visible');
  expect(vis['konkit-hyb-0']).toBe('none');
  expect(vis['konkit-hyb-1']).toBe('none');
  expect('konkit-dots' in vis).toBe(false);
});

test('hyb also shows both overlays', () => {
  const { map, vis } = fakeMap([...base, 'konkit-dots']);
  applyBasemap(map, 'hyb', config);
  expect(vis['konkit-img']).toBe('visible');
  expect(vis['konkit-hyb-0']).toBe('visible');
  expect(vis['konkit-hyb-1']).toBe('visible');
  for (const id of base) expect(vis[id]).toBe('none');
});

test('map shows the base style and hides raster layers', () => {
  const { map, vis } = fakeMap([...base, 'konkit-dots']);
  applyBasemap(map, 'hyb', config);
  applyBasemap(map, 'map', config);
  for (const id of base) expect(vis[id]).toBe('visible');
  expect(vis['konkit-img']).toBe('none');
  expect(vis['konkit-hyb-0']).toBe('none');
  expect('konkit-dots' in vis).toBe(false);
});

test('raster layers are created once and sit below konkit data layers', () => {
  const { map, added, layers } = fakeMap([...base, 'konkit-dots']);
  applyBasemap(map, 'sat', config);
  applyBasemap(map, 'hyb', config);
  expect(added).toEqual(['konkit-img', 'konkit-hyb-0', 'konkit-hyb-1']);
  const ids = layers.map((l) => l.id);
  expect(ids.indexOf('konkit-dots')).toBe(ids.length - 1);
});

test('before the style loads nothing throws and the mode is applied on load', () => {
  const { map, vis, fire } = fakeMap([...base, 'konkit-dots'], false);
  expect(() => applyBasemap(map, 'sat', config)).not.toThrow();
  expect(vis).toEqual({});
  fire();
  expect(vis['konkit-img']).toBe('visible');
  expect(vis.water).toBe('none');
});

test('several calls before load apply only the latest mode', () => {
  const { map, vis, fire, onceCount } = fakeMap([...base, 'konkit-dots'], false);
  applyBasemap(map, 'hyb', config);
  applyBasemap(map, 'map', config);
  expect(onceCount()).toBe(1);
  fire();
  expect(vis.water).toBe('visible');
  expect(vis['konkit-img']).toBe('none');
  expect(vis['konkit-hyb-0']).toBe('none');
});

test('withApiKey appends key with ? or & and leaves URLs alone when empty', () => {
  expect(withApiKey('https://a.test/s', '')).toBe('https://a.test/s');
  expect(withApiKey('https://a.test/s', 'k 1')).toBe('https://a.test/s?key=k%201');
  expect(withApiKey('https://a.test/{z}/{y}/{x}?v=2', 'k')).toBe('https://a.test/{z}/{y}/{x}?v=2&key=k');
});

test('raster tile URLs get the api key', () => {
  const sources: Record<string, unknown> = {};
  const { map } = fakeMap(base);
  (map as unknown as { addSource: (id: string, s: unknown) => void }).addSource = (id, s) => { sources[id] = s; };
  applyBasemap(map, 'hyb', { ...config, api_key: 'K' });
  expect((sources['konkit-img'] as { tiles: string[] }).tiles[0]).toBe('https://sat.test/{z}/{y}/{x}?key=K');
  expect((sources['konkit-hyb-1'] as { tiles: string[] }).tiles[0]).toBe('https://ov.test/b/{z}/{y}/{x}?key=K');
});

test('after load, a basemap switch applies immediately even while tiles are loading', () => {
  const { map, vis, fire, setBusy } = fakeMap([...base, 'konkit-dots'], false);
  applyBasemap(map, 'map', config);
  fire();
  setBusy(true); // isStyleLoaded() is false again while tiles load
  applyBasemap(map, 'sat', config);
  expect(vis['konkit-img']).toBe('visible');
  expect(vis.water).toBe('none');
  applyBasemap(map, 'hyb', config);
  expect(vis['konkit-hyb-1']).toBe('visible');
});

test('a map marked ready applies immediately regardless of isStyleLoaded', () => {
  const { map, vis, setBusy } = fakeMap([...base, 'konkit-dots']);
  setBusy(true);
  markStyleReady(map);
  applyBasemap(map, 'sat', config);
  expect(vis['konkit-img']).toBe('visible');
});
