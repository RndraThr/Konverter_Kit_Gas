export type BootstrapUser = {
  id: string;
  full_name: string;
  username: string;
  email: string;
  roles: string[];
  permissions: string[];
  is_active?: boolean;
  last_login_at?: string;
  created_at?: string;
  updated_at?: string;
};

export type BootstrapResponse = {
  data: BootstrapUser;
  meta: { csrf_token: string };
};

type ErrorResponse = {
  error?: { code?: string; message?: string; fields?: Record<string, string> };
};

let csrfToken = '';

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
    public readonly fields: Record<string, string> = {},
  ) {
    super(message);
  }
}

type ApiRequestInit = RequestInit & { acceptedStatuses?: number[] };

export async function apiRequest<T>(path: string, init: ApiRequestInit = {}): Promise<T> {
  const { acceptedStatuses = [], ...requestInit } = init;
  const method = (requestInit.method ?? 'GET').toUpperCase();
  const headers = new Headers(requestInit.headers);
  if (requestInit.body) headers.set('Content-Type', 'application/json');
  if (!['GET', 'HEAD', 'OPTIONS'].includes(method) && csrfToken) {
    headers.set('X-CSRF-Token', csrfToken);
  }

  const response = await fetch(path, { ...requestInit, headers, credentials: 'same-origin' });
  if (response.status === 401) {
    window.location.assign('/login');
    throw new ApiError(401, 'unauthorized', 'Sesi login telah berakhir');
  }
  if (!response.ok && !acceptedStatuses.includes(response.status)) {
    const payload = await response.json().catch(() => ({})) as ErrorResponse;
    throw new ApiError(
      response.status,
      payload.error?.code ?? 'request_failed',
      payload.error?.message ?? 'Permintaan tidak dapat diproses',
      payload.error?.fields,
    );
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

export async function getBootstrap(): Promise<BootstrapResponse> {
  const result = await apiRequest<BootstrapResponse>('/api/v1/me');
  csrfToken = result.meta.csrf_token;
  return result;
}
