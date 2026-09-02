import { afterEach, expect, test, vi } from 'vitest';
import { apiRequest } from './api';

afterEach(() => vi.unstubAllGlobals());

test('can parse explicitly accepted non-success statuses', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{"data":{"status":"degraded"}}', {
    status: 503,
    headers: { 'Content-Type': 'application/json' },
  })));

  const result = await apiRequest<{ data: { status: string } }>('/api/v1/system/health', { acceptedStatuses: [503] });

  expect(result.data.status).toBe('degraded');
});
