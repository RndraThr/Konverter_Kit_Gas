import { expect, test } from 'vitest';
import type maplibregl from 'maplibre-gl';
import { applyBasemap } from './basemaps';
import type { MapConfig } from './types';

const config: MapConfig = {
  style_url: 'https://style.test/liberty',
  satellite_tiles: 'https://sat.test/{z}/{y}/{x}',
  hybrid_overlay_tiles: ['https://ov.test/a/{z}/{y}/{x}', 'https://ov.test/b/{z}/{y}/{x}'],
  attribution: 'attr',
  api_key: '',
};

function fakeMap(layerIds: string[]) {
  const layers = layerIds.map((id) => ({ id, type: 'fill' }));
  const vis: Record<string, string> = {};
  const sources = new Set<string>();
  const added: string[] = [];
  const map = {
    getStyle: () => ({ layers }),
    getLayer: (id: string) => layers.find((l) => l.id === id),
    getSource: (id: string) => (sources.has(id) ? {} : undefined),
    addSource: (id: string) => { sources.add(id); },
    addLayer: (l: { id: string }, before?: string) => {
      const at = before ? layers.findIndex((x) => x.id === before) : -1;
      const entry = { id: l.id, type: 'raster' };
      if (at >= 0) layers.splice(at, 0, entry); else layers.push(entry);
      added.push(l.id);
    },
    setLayoutProperty: (id: string, prop: string, value: string) => { if (prop === 'visibility') vis[id] = value; },
  };
  return { map: map as unknown as maplibregl.Map, vis, added, layers };
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
