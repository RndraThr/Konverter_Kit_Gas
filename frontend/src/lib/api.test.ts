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

test('lets the browser set the multipart boundary for FormData', async () => {
  const fetchMock = vi.fn().mockResolvedValue(new Response('{"data":{"id":"preview-1"}}', {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  }));
  vi.stubGlobal('fetch', fetchMock);
  const body = new FormData();
  body.set('schedule_id', 'schedule-1');
  body.set('file', new File(['workbook'], 'dcp3.xlsx'));

  await apiRequest('/api/v1/dcp3/previews', { method: 'POST', body });

  const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
  expect(new Headers(request.headers).has('Content-Type')).toBe(false);
});
