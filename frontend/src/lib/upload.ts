import { ApiError, getCSRFToken } from './api';

type UploadOptions = {
  signal?: AbortSignal;
  onProgress?: (percent: number) => void;
};

type ErrorResponse = {
  error?: { code?: string; message?: string; fields?: Record<string, string> };
};

function parseJSON(text: string): unknown {
  if (!text) return undefined;
  try {
    return JSON.parse(text);
  } catch {
    return undefined;
  }
}

export function uploadRequest<T>(path: string, form: FormData, options: UploadOptions = {}): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    let settled = false;

    const cleanup = () => {
      xhr.removeEventListener('load', handleLoad);
      xhr.removeEventListener('error', handleNetworkError);
      xhr.removeEventListener('abort', handleAbort);
      xhr.upload.removeEventListener('progress', handleProgress);
      options.signal?.removeEventListener('abort', handleSignalAbort);
    };
    const succeed = (value: T) => {
      if (settled) return;
      settled = true;
      cleanup();
      resolve(value);
    };
    const fail = (error: unknown) => {
      if (settled) return;
      settled = true;
      cleanup();
      reject(error);
    };
    const handleProgress = (event: ProgressEvent) => {
      if (!event.lengthComputable || event.total <= 0) return;
      const percent = Math.max(0, Math.min(100, Math.round((event.loaded / event.total) * 100)));
      options.onProgress?.(percent);
    };
    const handleLoad = () => {
      const payload = parseJSON(xhr.responseText) as ErrorResponse | undefined;
      if (xhr.status === 401) {
        window.location.assign('/login?notice=session_expired');
        fail(new ApiError(401, 'unauthorized', 'Sesi login telah berakhir'));
        return;
      }
      if (xhr.status < 200 || xhr.status >= 300) {
        fail(new ApiError(
          xhr.status,
          payload?.error?.code ?? 'request_failed',
          payload?.error?.message ?? 'Permintaan tidak dapat diproses',
          payload?.error?.fields,
        ));
        return;
      }
      succeed(payload as T);
    };
    const handleNetworkError = () => fail(new Error('Koneksi terputus saat mengunggah berkas'));
    const handleAbort = () => fail(new DOMException('Unggahan dibatalkan', 'AbortError'));
    const handleSignalAbort = () => xhr.abort();

    xhr.addEventListener('load', handleLoad);
    xhr.addEventListener('error', handleNetworkError);
    xhr.addEventListener('abort', handleAbort);
    xhr.upload.addEventListener('progress', handleProgress);
    options.signal?.addEventListener('abort', handleSignalAbort, { once: true });

    xhr.open('POST', path);
    xhr.withCredentials = true;
    const csrfToken = getCSRFToken();
    if (csrfToken) xhr.setRequestHeader('X-CSRF-Token', csrfToken);

    if (options.signal?.aborted) {
      handleAbort();
      return;
    }
    xhr.send(form);
  });
}
