import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import BarcodeScanner from './BarcodeScanner';

type ResultCallback = (result: { getText: () => string } | null, error: unknown, controls: { stop: () => void }) => void;

const { decodeFromConstraints, decodeFromVideoDevice, listVideoInputDevices, stop } = vi.hoisted(() => ({
  decodeFromConstraints: vi.fn(),
  decodeFromVideoDevice: vi.fn(),
  listVideoInputDevices: vi.fn().mockResolvedValue([]),
  stop: vi.fn(),
}));

vi.mock('@zxing/browser', () => {
  function BrowserMultiFormatReader(this: unknown) {
    return { decodeFromConstraints, decodeFromVideoDevice };
  }
  BrowserMultiFormatReader.listVideoInputDevices = listVideoInputDevices;
  return { BrowserMultiFormatReader };
});

function attachStream(video: HTMLVideoElement, facingMode: string, deviceId = 'device-1') {
  Object.defineProperty(video, 'srcObject', {
    configurable: true,
    value: { getVideoTracks: () => [{ getSettings: () => ({ facingMode, deviceId }) }] },
  });
}

let lastConstraintsCallback: ResultCallback | undefined;

beforeEach(() => {
  vi.clearAllMocks();
  listVideoInputDevices.mockResolvedValue([]);
  decodeFromConstraints.mockImplementation((_constraints: unknown, video: HTMLVideoElement, callback: ResultCallback) => {
    lastConstraintsCallback = callback;
    attachStream(video, 'environment');
    return Promise.resolve({ stop });
  });
});

afterEach(() => { lastConstraintsCallback = undefined; });

test('shows a loading state while the camera starts, then the rear-camera preview unmirrored', async () => {
  let resolveStart: (value: { stop: () => void }) => void = () => {};
  decodeFromConstraints.mockImplementation((_c: unknown, video: HTMLVideoElement, callback: ResultCallback) => {
    lastConstraintsCallback = callback;
    return new Promise((resolve) => { resolveStart = (value) => { attachStream(video, 'environment'); resolve(value); }; });
  });
  render(<BarcodeScanner onResult={vi.fn()} onClose={vi.fn()} />);

  expect(screen.getByText('Menyiapkan kamera...')).toBeVisible();
  resolveStart({ stop });
  await waitFor(() => expect(screen.queryByText('Menyiapkan kamera...')).not.toBeInTheDocument());
  expect(screen.getByRole('dialog').querySelector('video')).not.toHaveStyle({ transform: 'scaleX(-1)' });
});

test('mirrors the preview for a front-facing camera', async () => {
  decodeFromConstraints.mockImplementation((_c: unknown, video: HTMLVideoElement, callback: ResultCallback) => {
    lastConstraintsCallback = callback;
    attachStream(video, 'user');
    return Promise.resolve({ stop });
  });
  render(<BarcodeScanner onResult={vi.fn()} onClose={vi.fn()} />);

  await waitFor(() => expect(screen.getByRole('dialog').querySelector('video')).toHaveStyle({ transform: 'scaleX(-1)' }));
});

test('shows a specific message and a retry action when camera permission is denied', async () => {
  decodeFromConstraints.mockRejectedValue(new DOMException('denied', 'NotAllowedError'));
  render(<BarcodeScanner onResult={vi.fn()} onClose={vi.fn()} />);

  expect(await screen.findByRole('alert')).toHaveTextContent('Izin kamera ditolak');
  expect(screen.getByRole('button', { name: 'Coba lagi' })).toBeVisible();
});

test('shows a specific message when no camera is found', async () => {
  decodeFromConstraints.mockRejectedValue(new DOMException('none', 'NotFoundError'));
  render(<BarcodeScanner onResult={vi.fn()} onClose={vi.fn()} />);

  expect(await screen.findByRole('alert')).toHaveTextContent('Kamera tidak ditemukan');
});

test('retries starting the camera when "Coba lagi" is clicked', async () => {
  decodeFromConstraints.mockRejectedValueOnce(new DOMException('denied', 'NotAllowedError'));
  render(<BarcodeScanner onResult={vi.fn()} onClose={vi.fn()} />);
  expect(await screen.findByRole('alert')).toBeVisible();

  fireEvent.click(screen.getByRole('button', { name: 'Coba lagi' }));

  await waitFor(() => expect(screen.queryByRole('alert')).not.toBeInTheDocument());
  expect(decodeFromConstraints).toHaveBeenCalledTimes(2);
});

test('calls onResult with the decoded text once a barcode is read', async () => {
  const onResult = vi.fn();
  render(<BarcodeScanner onResult={onResult} onClose={vi.fn()} />);
  await waitFor(() => expect(lastConstraintsCallback).toBeTruthy());

  lastConstraintsCallback!({ getText: () => 'SN-1234567890' }, null, { stop: vi.fn() });

  expect(onResult).toHaveBeenCalledWith('SN-1234567890');
});

test('lets the user switch cameras once more than one is available', async () => {
  listVideoInputDevices.mockResolvedValue([
    { deviceId: 'device-1', label: 'Kamera Belakang' } as MediaDeviceInfo,
    { deviceId: 'device-2', label: 'Kamera Depan' } as MediaDeviceInfo,
  ]);
  render(<BarcodeScanner onResult={vi.fn()} onClose={vi.fn()} />);

  const select = await screen.findByRole('combobox', { name: 'Kamera' });
  expect(select).toBeVisible();
  expect(decodeFromVideoDevice).not.toHaveBeenCalled();
});

test('calls onClose when the dialog is dismissed', async () => {
  const onClose = vi.fn();
  render(<BarcodeScanner onResult={vi.fn()} onClose={onClose} />);
  await waitFor(() => expect(screen.queryByText('Menyiapkan kamera...')).not.toBeInTheDocument());

  fireEvent.keyDown(document, { key: 'Escape' });

  await waitFor(() => expect(onClose).toHaveBeenCalled());
});
