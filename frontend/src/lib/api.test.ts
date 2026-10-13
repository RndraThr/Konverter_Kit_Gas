import { afterEach, expect, test, vi } from 'vitest';
import { apiRequest, getBootstrap, getCSRFToken } from './api';

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

test('exposes the bootstrap CSRF token read-only to upload clients', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ data: { id: 'u1' }, meta: { csrf_token: 'csrf-from-bootstrap' } }), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })));
  await getBootstrap();
  expect(getCSRFToken()).toBe('csrf-from-bootstrap');
});

test('explains a proxy 413 (HTML body) as a file that is too large', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('<html><body>413 Request Entity Too Large</body></html>', {
    status: 413,
    headers: { 'Content-Type': 'text/html' },
  })));

  await expect(apiRequest('/api/v1/bast/wet-signature/scan', { method: 'PUT', body: '{}' })).rejects.toMatchObject({
    status: 413,
    message: 'Berkas terlalu besar untuk diunggah',
  });
});
