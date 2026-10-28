import { act, cleanup, render } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import type { MapConfig } from './types';

type Handler = (e?: unknown) => void;
const instances: FakeMap[] = [];

class FakeMap {
  handlers: Record<string, Handler[]> = {};
  resize = vi.fn();
  remove = vi.fn();
  addControl = vi.fn();
  options: Record<string, unknown>;
  constructor(options: Record<string, unknown>) {
    this.options = options;
    instances.push(this);
  }
  on(type: string, fn: Handler) { (this.handlers[type] ??= []).push(fn); return this; }
  once(type: string, fn: Handler) { return this.on(type, fn); }
  emit(type: string, e?: unknown) { (this.handlers[type] ?? []).forEach((fn) => fn(e)); }
}

vi.mock('maplibre-gl', () => ({
  default: { Map: FakeMap, NavigationControl: class {}, ScaleControl: class {} },
}));
vi.mock('maplibre-gl/dist/maplibre-gl.css', () => ({}));

import { MapCanvas } from './MapCanvas';

const config: MapConfig = { style_url: 'https://style.test', satellite_tiles: '', hybrid_overlay_tiles: [], attribution: 'a', api_key: '' };
let roCallback: (() => void) | undefined;

beforeEach(() => {
  instances.length = 0;
  vi.useFakeTimers();
  vi.stubGlobal('ResizeObserver', class {
    constructor(cb: () => void) { roCallback = cb; }
    observe() {}
    disconnect() {}
  });
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

async function mount(onReady = vi.fn(), onStyleError = vi.fn()) {
  render(<MapCanvas config={config} onReady={onReady} onStyleError={onStyleError} />);
  await act(async () => { await vi.advanceTimersByTimeAsync(0); });
  return { onReady, onStyleError, map: instances[0] };
}

test('calls onStyleError when load does not arrive within 8000 ms', async () => {
  const { onStyleError, onReady } = await mount();
  await act(async () => { await vi.advanceTimersByTimeAsync(7999); });
  expect(onStyleError).not.toHaveBeenCalled();
  await act(async () => { await vi.advanceTimersByTimeAsync(1); });
  expect(onStyleError).toHaveBeenCalledTimes(1);
  expect(onReady).not.toHaveBeenCalled();
});

test('load before the deadline calls onReady and cancels the timeout', async () => {
  const { onStyleError, onReady, map } = await mount();
  act(() => map.emit('load'));
  expect(onReady).toHaveBeenCalledWith(map);
  await act(async () => { await vi.advanceTimersByTimeAsync(20000); });
  expect(onStyleError).not.toHaveBeenCalled();
});

test('a style error before load fails fast, a tile error does not', async () => {
  const first = await mount();
  act(() => first.map.emit('error', { error: new Error('tile'), sourceId: 'konkit-img' }));
  expect(first.onStyleError).not.toHaveBeenCalled();
  act(() => first.map.emit('error', { error: new Error('style 500') }));
  expect(first.onStyleError).toHaveBeenCalledTimes(1);
  await act(async () => { await vi.advanceTimersByTimeAsync(9000); });
  expect(first.onStyleError).toHaveBeenCalledTimes(1);
});

test('errors after load are ignored; ResizeObserver resizes; unmount removes the map', async () => {
  const { onStyleError, map } = await mount();
  act(() => map.emit('load'));
  act(() => map.emit('error', { error: new Error('late') }));
  expect(onStyleError).not.toHaveBeenCalled();
  roCallback?.();
  expect(map.resize).toHaveBeenCalled();
  cleanup();
  expect(map.remove).toHaveBeenCalled();
});
