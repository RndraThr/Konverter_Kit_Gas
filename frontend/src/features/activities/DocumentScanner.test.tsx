import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import DocumentScanner from './DocumentScanner';

vi.mock('../../lib/api');

// jscanify's real browser entry point is the `jscanify/client` subpath (its
// bare `jscanify` import resolves to a Node-only build) — see
// DocumentScanner.tsx's header comment and the task report for why.
vi.mock('jscanify/client', () => ({
  default: class {
    highlightPaper(canvas: HTMLCanvasElement) { return canvas; }
    extractPaper(canvas: HTMLCanvasElement) { return canvas; }
  },
}));

function attachStream(video: HTMLVideoElement) {
  // jsdom's `srcObject` is a getter-only no-op property; make it a plain
  // writable field so the component's own `video.srcObject = stream` works.
  Object.defineProperty(video, 'srcObject', { configurable: true, writable: true, value: null });
  Object.defineProperty(video, 'videoWidth', { configurable: true, value: 640 });
  Object.defineProperty(video, 'videoHeight', { configurable: true, value: 480 });
}

const getUserMedia = vi.fn();

beforeEach(() => {
  vi.clearAllMocks();
  // opencv.js is fetched from a CDN at runtime (see DocumentScanner.tsx); stub
  // `window.cv` as already-loaded so the component doesn't need jsdom to
  // execute an external <script> tag (which it won't).
  (window as unknown as { cv?: unknown }).cv = {};
  Object.defineProperty(navigator, 'mediaDevices', {
    configurable: true,
    value: { getUserMedia },
  });
  getUserMedia.mockImplementation((_constraints: unknown) => {
    const stream = { getTracks: () => [{ stop: vi.fn() }] } as unknown as MediaStream;
    return Promise.resolve(stream);
  });

  // jsdom's canvas backing (via the `canvas` package) can't draw from an
  // HTMLVideoElement the way real browsers can, and has no usable toBlob —
  // this test only needs to prove page bookkeeping/upload wiring, not real
  // pixel manipulation, so canvas drawing is stubbed out entirely.
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({
    drawImage: vi.fn(),
    clearRect: vi.fn(),
    getImageData: vi.fn(() => ({ data: new Uint8ClampedArray(4) })),
    putImageData: vi.fn(),
  } as unknown as CanvasRenderingContext2D);
  vi.spyOn(HTMLCanvasElement.prototype, 'toBlob').mockImplementation((callback) => {
    callback(new Blob(['page'], { type: 'image/jpeg' }));
  });
});

afterEach(() => {
  delete (window as unknown as { cv?: unknown }).cv;
});

describe('DocumentScanner', () => {
  it('accumulates pages across captures and posts them all on finish', async () => {
    const onDone = vi.fn();
    const onClose = vi.fn();
    render(
      <DocumentScanner scheduleId="s1" slotId="slot-1" programId="p1" onDone={onDone} onClose={onClose} />,
    );

    const video = document.querySelector('video');
    expect(video).toBeTruthy();
    attachStream(video as HTMLVideoElement);
    // getUserMedia resolves asynchronously; wait for the loading spinner to clear.
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Ambil Halaman' })).toBeEnabled());

    fireEvent.click(screen.getByRole('button', { name: 'Ambil Halaman' }));
    await waitFor(() => expect(screen.getByText('1 halaman terkumpul.')).toBeVisible());

    fireEvent.click(screen.getByRole('button', { name: 'Ambil Halaman' }));
    await waitFor(() => expect(screen.getByText('2 halaman terkumpul.')).toBeVisible());

    fireEvent.click(screen.getByRole('button', { name: 'Selesai' }));

    await waitFor(() => expect(apiRequest).toHaveBeenCalledTimes(1));
    expect(apiRequest).toHaveBeenCalledWith(
      '/api/v1/activities/supporting-documents/scan',
      expect.objectContaining({ method: 'POST' }),
    );
    const body = (apiRequest as unknown as ReturnType<typeof vi.fn>).mock.calls[0][1].body as FormData;
    expect(body.get('schedule_id')).toBe('s1');
    expect(body.get('slot_id')).toBe('slot-1');
    expect(body.getAll('page')).toHaveLength(2);
    await waitFor(() => expect(onDone).toHaveBeenCalled());
  });
});
