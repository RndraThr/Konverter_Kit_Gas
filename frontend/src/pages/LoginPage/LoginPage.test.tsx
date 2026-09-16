import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, test, vi } from 'vitest';
import { Toaster } from '@/components/ui/sonner';
import { LoginPage } from './LoginPage';

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  history.replaceState({}, '', '/');
});

test('keeps field imagery, brands, and submission feedback accessible', async () => {
  history.replaceState({}, '', '/login?error=invalid');
  const user = userEvent.setup();
  render(<LoginPage />);

  expect(screen.getByRole('main')).toBeInTheDocument();
  expect(screen.getByRole('form', { name: 'Masuk ke dashboard' })).toBeInTheDocument();
  expect(screen.getByAltText('Ergas')).toBeInTheDocument();
  expect(screen.getByAltText('PT Kian Santang Mulitama Tbk')).toBeInTheDocument();
  expect(screen.getByRole('alert')).toHaveTextContent('Email/username atau password tidak sesuai.');

  await user.type(screen.getByLabelText('Email atau username'), 'admin');
  await user.type(screen.getByLabelText('Password'), 'password-rahasia');

  expect(screen.getByRole('button', { name: 'Masuk' })).toBeEnabled();
});

test('shows verified login success before navigating to the dashboard', async () => {
  vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() }));
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{"data":{"authenticated":true}}', {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })));
  const location = window.location;
  Object.defineProperty(window, 'location', { configurable: true, value: { ...location, assign: vi.fn() } });

  try {
    const user = userEvent.setup();
    render(<><LoginPage /><Toaster /></>);
    await user.type(screen.getByLabelText('Email atau username'), 'admin');
    await user.type(screen.getByLabelText('Password'), 'password-rahasia');
    await user.click(screen.getByRole('button', { name: 'Masuk' }));

    expect(await screen.findByText('Login berhasil. Mengarahkan ke dashboard…')).toBeVisible();
    expect(window.location.assign).not.toHaveBeenCalled();
    await waitFor(() => expect(window.location.assign).toHaveBeenCalledWith('/dashboard'), { timeout: 1500 });
  } finally {
    Object.defineProperty(window, 'location', { configurable: true, value: location });
  }
});

test('hides stale query feedback when a new login attempt succeeds', async () => {
  vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() }));
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{"data":{"authenticated":true}}', {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })));
  history.replaceState({}, '', '/login?error=invalid');
  const location = window.location;
  Object.defineProperty(window, 'location', { configurable: true, value: { ...location, assign: vi.fn() } });

  try {
    const user = userEvent.setup();
    render(<><LoginPage /><Toaster /></>);
    expect(screen.getByRole('alert')).toBeVisible();
    await user.type(screen.getByLabelText('Email atau username'), 'admin');
    await user.type(screen.getByLabelText('Password'), 'password-rahasia');
    await user.click(screen.getByRole('button', { name: 'Masuk' }));

    await waitFor(() => expect(screen.queryByRole('alert')).not.toBeInTheDocument());
    await waitFor(() => expect(window.location.assign).toHaveBeenCalledWith('/dashboard'), { timeout: 1500 });
  } finally {
    Object.defineProperty(window, 'location', { configurable: true, value: location });
  }
});

test('keeps rejected asynchronous login feedback in the form', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{"error":{"code":"invalid_credentials"}}', {
    status: 401,
    headers: { 'Content-Type': 'application/json' },
  })));
  const user = userEvent.setup();
  render(<LoginPage />);
  await user.type(screen.getByLabelText('Email atau username'), 'admin');
  await user.type(screen.getByLabelText('Password'), 'salah');
  await user.click(screen.getByRole('button', { name: 'Masuk' }));

  expect(await screen.findByRole('alert')).toHaveTextContent('Email/username atau password tidak sesuai.');
  expect(screen.getByRole('button', { name: 'Masuk' })).toBeEnabled();
});

test.each([
  ['logged_out', 'Anda telah keluar dengan aman.'],
  ['session_expired', 'Sesi Anda telah berakhir. Silakan masuk kembali.'],
])('shows a transient notification for the %s login notice', async (notice, message) => {
  vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() }));
  history.replaceState({}, '', `/login?notice=${notice}`);
  render(<><LoginPage /><Toaster /></>);

  expect(await screen.findByText(message)).toBeVisible();
  expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  expect(new URLSearchParams(window.location.search).has('notice')).toBe(false);
});

test('consumes a transient notice without discarding the persistent login error', async () => {
  vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() }));
  history.replaceState({}, '', '/login?error=invalid&notice=logged_out');
  render(<><LoginPage /><Toaster /></>);

  expect(screen.getByRole('alert')).toHaveTextContent('Email/username atau password tidak sesuai.');
  await waitFor(() => expect(window.location.search).toBe('?error=invalid'));
});

test('lets the user reveal and hide the password without clearing it', async () => {
  const user = userEvent.setup();
  render(<LoginPage />);

  const password = screen.getByLabelText('Password');
  await user.type(password, 'password-rahasia');
  await user.click(screen.getByRole('button', { name: 'Tampilkan kata sandi' }));

  expect(password).toHaveAttribute('type', 'text');
  expect(password).toHaveValue('password-rahasia');

  await user.click(screen.getByRole('button', { name: 'Sembunyikan kata sandi' }));
  expect(password).toHaveAttribute('type', 'password');
});

test('offers explicit carousel controls and updates the visible field story', async () => {
  const user = userEvent.setup();
  render(<LoginPage />);

  expect(screen.getByText('Serah Terima')).toBeVisible();
  await user.click(screen.getByRole('button', { name: 'Tampilkan foto Program Kabupaten' }));

  expect(screen.getByText('Program Kabupaten')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Lanjutkan pergantian foto' })).toBeInTheDocument();
  expect(screen.getByRole('group', { name: 'Pilih foto lapangan' })).toBeInTheDocument();
  expect(screen.getByRole('region', { name: 'Cerita lapangan Konkit' }).firstElementChild).toHaveAttribute('aria-live', 'polite');
});

test('explains when remembering a device is appropriate', () => {
  render(<LoginPage />);

  expect(screen.getByText('Gunakan hanya di perangkat pribadi.')).toBeInTheDocument();
});

test('preserves the server login form contract', () => {
  render(<LoginPage />);

  const form = screen.getByRole('form', { name: 'Masuk ke dashboard' });
  expect(form).toHaveAttribute('method', 'post');
  expect(form).toHaveAttribute('action', '/login');
  expect(screen.getByLabelText('Email atau username')).toHaveAttribute('name', 'identity');
  expect(screen.getByLabelText('Password')).toHaveAttribute('name', 'password');
  expect(form.querySelector('input[name="remember"]')).toBeInTheDocument();
});

test('advances automatically, pauses on request, and clears its timer on unmount', () => {
  vi.useFakeTimers();
  const { unmount } = render(<LoginPage />);
  const story = screen.getByRole('region', { name: 'Cerita lapangan Konkit' });

  expect(story.firstElementChild).toHaveAttribute('aria-live', 'off');
  act(() => vi.advanceTimersByTime(7000));
  expect(screen.getByText('Program Kabupaten')).toBeInTheDocument();

  fireEvent.click(screen.getByRole('button', { name: 'Jeda pergantian foto' }));
  act(() => vi.advanceTimersByTime(7000));
  expect(screen.getByText('Program Kabupaten')).toBeInTheDocument();

  unmount();
  expect(vi.getTimerCount()).toBe(0);
});
