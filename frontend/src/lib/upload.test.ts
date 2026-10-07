import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { getBootstrap } from './api';
import { uploadRequest } from './upload';

class FakeXMLHttpRequest extends EventTarget {
  static latest: FakeXMLHttpRequest;
  upload = new EventTarget();
  headers = new Map<string, string>();
  method = '';
  url = '';
  body: Document | XMLHttpRequestBodyInit | null = null;
  status = 0;
  responseText = '';
  responseType: XMLHttpRequestResponseType = '';
  withCredentials = false;

  constructor() {
    super();
    FakeXMLHttpRequest.latest = this;
  }

  open(method: string, url: string) {
    this.method = method;
    this.url = url;
  }

  setRequestHeader(name: string, value: string) {
    this.headers.set(name, value);
  }

  send(body: Document | XMLHttpRequestBodyInit | null) {
    this.body = body;
  }

  abort() {
    this.dispatchEvent(new Event('abort'));
  }

  respond(status: number, payload: unknown) {
    this.status = status;
    this.responseText = payload === undefined ? '' : JSON.stringify(payload);
    this.dispatchEvent(new Event('load'));
  }
}

beforeEach(async () => {
  vi.stubGlobal('XMLHttpRequest', FakeXMLHttpRequest);
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ data: { id: 'u1' }, meta: { csrf_token: 'csrf-upload' } }), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })));
  await getBootstrap();
});

afterEach(() => vi.unstubAllGlobals());

describe('uploadRequest', () => {
  test('sends FormData with same-origin credentials, CSRF, JSON parsing, and integer progress', async () => {
    const onProgress = vi.fn();
    const form = new FormData();
    form.set('file', new File(['bytes'], 'proof.jpg'));
    const promise = uploadRequest<{ data: { id: string } }>('/api/v1/upload', form, { onProgress });
    const xhr = FakeXMLHttpRequest.latest;

    expect(xhr.method).toBe('POST');
    expect(xhr.url).toBe('/api/v1/upload');
    expect(xhr.withCredentials).toBe(true);
    expect(xhr.headers.get('X-CSRF-Token')).toBe('csrf-upload');
    expect(xhr.headers.has('Content-Type')).toBe(false);
    expect(xhr.body).toBe(form);

    xhr.upload.dispatchEvent(new ProgressEvent('progress', { lengthComputable: true, loaded: 1, total: 3 }));
    expect(onProgress).toHaveBeenCalledWith(33);
    xhr.respond(201, { data: { id: 'media-1' } });
    await expect(promise).resolves.toEqual({ data: { id: 'media-1' } });
  });

  test.each([
    [413, 'media_too_large'],
    [429, 'video_upload_busy'],
  ])('parses structured API error %i', async (status, code) => {
    const promise = uploadRequest('/api/v1/upload', new FormData());
    FakeXMLHttpRequest.latest.respond(status, { error: { code, message: 'Upload ditolak', fields: { file: 'Periksa berkas' } } });
    await expect(promise).rejects.toMatchObject({ status, code, message: 'Upload ditolak', fields: { file: 'Periksa berkas' } });
  });

  test('propagates AbortSignal and aborts the native request', async () => {
    const controller = new AbortController();
    const promise = uploadRequest('/api/v1/upload', new FormData(), { signal: controller.signal });
    const abortSpy = vi.spyOn(FakeXMLHttpRequest.latest, 'abort');
    controller.abort();
    expect(abortSpy).toHaveBeenCalledOnce();
    await expect(promise).rejects.toMatchObject({ name: 'AbortError' });
  });

  test('returns a useful network failure message', async () => {
    const promise = uploadRequest('/api/v1/upload', new FormData());
    FakeXMLHttpRequest.latest.dispatchEvent(new Event('error'));
    await expect(promise).rejects.toThrow('Koneksi terputus saat mengunggah berkas');
  });

  test('redirects to login on 401', async () => {
    const location = window.location;
    Object.defineProperty(window, 'location', { configurable: true, value: { ...location, assign: vi.fn() } });
    const promise = uploadRequest('/api/v1/upload', new FormData());
    FakeXMLHttpRequest.latest.respond(401, { error: { code: 'unauthorized' } });
    await expect(promise).rejects.toMatchObject({ status: 401, code: 'unauthorized' });
    expect(window.location.assign).toHaveBeenCalledWith('/login?notice=session_expired');
    Object.defineProperty(window, 'location', { configurable: true, value: location });
  });
});
