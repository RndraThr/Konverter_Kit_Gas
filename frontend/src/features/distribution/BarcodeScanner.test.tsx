import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { BarcodeFormat } from '@zxing/library';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
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
  // The decoder starts after the (async) native-detector probe, so wait until it has been called.
  await waitFor(() => expect(decodeFromConstraints).toHaveBeenCalled());
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

test('calls onResult only after the same value is decoded on consecutive frames', async () => {
  const onResult = vi.fn();
  const scanStop = vi.fn();
  render(<BarcodeScanner onResult={onResult} onClose={vi.fn()} />);
  await waitFor(() => expect(lastConstraintsCallback).toBeTruthy());

  const read = () => lastConstraintsCallback!({ getText: () => 'SN-1234567890' }, null, { stop: scanStop });
  read();
  read();
  expect(onResult).not.toHaveBeenCalled(); // two frames is not yet enough confidence

  read();
  expect(onResult).toHaveBeenCalledExactlyOnceWith('SN-1234567890');
  expect(scanStop).toHaveBeenCalled();
});

test('ignores a one-off misread and restarts the streak when a frame disagrees', async () => {
  const onResult = vi.fn();
  render(<BarcodeScanner onResult={onResult} onClose={vi.fn()} />);
  await waitFor(() => expect(lastConstraintsCallback).toBeTruthy());

  const read = (text: string) => lastConstraintsCallback!({ getText: () => text }, null, { stop: vi.fn() });
  // A stray wrong value in the middle must not count toward the correct value's streak.
  read('SN-0000000001');
  read('SN-0000000001');
  read('WRONG-9999'); // misread resets the count
  read('SN-0000000001');
  read('SN-0000000001');
  expect(onResult).not.toHaveBeenCalled();

  read('SN-0000000001');
  expect(onResult).toHaveBeenCalledExactlyOnceWith('SN-0000000001');
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

test('asks the camera for a 720p stream on the JavaScript decoder path instead of the 640x480 default', async () => {
  render(<BarcodeScanner onResult={vi.fn()} onClose={vi.fn()} />);
  await waitFor(() => expect(decodeFromConstraints).toHaveBeenCalled());

  expect(decodeFromConstraints.mock.calls[0][0]).toEqual({
    video: { facingMode: { ideal: 'environment' }, width: { ideal: 1280 }, height: { ideal: 720 } },
  });
});

test('accepts a checksum-verified format (Code 128) after two matching reads', async () => {
  const onResult = vi.fn();
  render(<BarcodeScanner onResult={onResult} onClose={vi.fn()} />);
  await waitFor(() => expect(lastConstraintsCallback).toBeTruthy());

  const read = () => lastConstraintsCallback!({ getText: () => 'SN-128', getBarcodeFormat: () => BarcodeFormat.CODE_128 } as never, null, { stop: vi.fn() });
  read();
  expect(onResult).not.toHaveBeenCalled();
  read();
  expect(onResult).toHaveBeenCalledExactlyOnceWith('SN-128');
});

test('still needs three matching reads for a format without a mandatory check digit (Code 39)', async () => {
  const onResult = vi.fn();
  render(<BarcodeScanner onResult={onResult} onClose={vi.fn()} />);
  await waitFor(() => expect(lastConstraintsCallback).toBeTruthy());

  const read = () => lastConstraintsCallback!({ getText: () => 'SN-39', getBarcodeFormat: () => BarcodeFormat.CODE_39 } as never, null, { stop: vi.fn() });
  read();
  read();
  expect(onResult).not.toHaveBeenCalled();
  read();
  expect(onResult).toHaveBeenCalledExactlyOnceWith('SN-39');
});

describe('native BarcodeDetector (Chrome on Android)', () => {
  const trackStop = vi.fn();
  const getUserMedia = vi.fn();
  let detections: Array<Array<{ rawValue: string; format: string; boundingBox: { x: number; y: number; width: number; height: number } }>>;
  let supportedFormats: string[];

  class FakeBarcodeDetector {
    static getSupportedFormats = () => Promise.resolve(supportedFormats);
    detect() { return Promise.resolve(detections.shift() ?? []); }
  }

  const centered = (rawValue: string, format = 'code_128') => ({ rawValue, format, boundingBox: { x: 760, y: 500, width: 400, height: 80 } });

  beforeEach(() => {
    supportedFormats = ['code_128', 'code_39', 'ean_13', 'qr_code'];
    detections = [];
    (globalThis as { BarcodeDetector?: unknown }).BarcodeDetector = FakeBarcodeDetector;
    getUserMedia.mockResolvedValue({
      getTracks: () => [{ stop: trackStop }],
      getVideoTracks: () => [{ getSettings: () => ({ facingMode: 'environment', deviceId: 'device-1' }) }],
    });
    Object.defineProperty(navigator, 'mediaDevices', { configurable: true, value: { getUserMedia } });
    Object.defineProperty(HTMLMediaElement.prototype, 'srcObject', { configurable: true, writable: true, value: null });
    Object.defineProperty(HTMLMediaElement.prototype, 'readyState', { configurable: true, get: () => 4 });
    Object.defineProperty(HTMLVideoElement.prototype, 'videoWidth', { configurable: true, get: () => 1920 });
    Object.defineProperty(HTMLVideoElement.prototype, 'videoHeight', { configurable: true, get: () => 1080 });
    HTMLMediaElement.prototype.play = vi.fn().mockResolvedValue(undefined);
  });

  afterEach(() => {
    delete (globalThis as { BarcodeDetector?: unknown }).BarcodeDetector;
    delete (HTMLMediaElement.prototype as { srcObject?: unknown }).srcObject;
    delete (HTMLMediaElement.prototype as { readyState?: unknown }).readyState;
    delete (HTMLVideoElement.prototype as { videoWidth?: unknown }).videoWidth;
    delete (HTMLVideoElement.prototype as { videoHeight?: unknown }).videoHeight;
  });

  test('uses the native detector with a Full HD stream instead of the JavaScript decoder', async () => {
    const onResult = vi.fn();
    detections = [[centered('SN-NATIVE')], [centered('SN-NATIVE')]];
    render(<BarcodeScanner onResult={onResult} onClose={vi.fn()} />);

    await waitFor(() => expect(onResult).toHaveBeenCalledExactlyOnceWith('SN-NATIVE'));
    expect(getUserMedia).toHaveBeenCalledWith({ video: { facingMode: { ideal: 'environment' }, width: { ideal: 1920 }, height: { ideal: 1080 } } });
    expect(decodeFromConstraints).not.toHaveBeenCalled();
    expect(trackStop).toHaveBeenCalled(); // the camera is released once a value is accepted
  });

  test('ignores barcodes outside the scan frame', async () => {
    const onResult = vi.fn();
    const corner = { rawValue: 'OTHER', format: 'code_128', boundingBox: { x: 0, y: 0, width: 120, height: 40 } };
    detections = [[corner], [corner], [corner], [centered('SN-FRAMED')], [centered('SN-FRAMED')]];
    render(<BarcodeScanner onResult={onResult} onClose={vi.fn()} />);

    await waitFor(() => expect(onResult).toHaveBeenCalled());
    expect(onResult).toHaveBeenCalledExactlyOnceWith('SN-FRAMED');
  });

  test('falls back to the JavaScript decoder when the browser cannot detect linear barcodes', async () => {
    supportedFormats = ['qr_code'];
    render(<BarcodeScanner onResult={vi.fn()} onClose={vi.fn()} />);

    await waitFor(() => expect(decodeFromConstraints).toHaveBeenCalled());
    expect(getUserMedia).not.toHaveBeenCalled();
  });
});

test('calls onClose when the dialog is dismissed', async () => {
  const onClose = vi.fn();
  render(<BarcodeScanner onResult={vi.fn()} onClose={onClose} />);
  await waitFor(() => expect(screen.queryByText('Menyiapkan kamera...')).not.toBeInTheDocument());

  fireEvent.keyDown(document, { key: 'Escape' });

  await waitFor(() => expect(onClose).toHaveBeenCalled());
});
