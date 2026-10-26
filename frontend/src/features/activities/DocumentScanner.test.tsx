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
    // A paper filling most of the 640x480 frame.
    findPaperContour() { return { delete() {} }; }
    getCornerPoints() { return { topLeftCorner: { x: 40, y: 20 }, topRightCorner: { x: 600, y: 20 }, bottomLeftCorner: { x: 40, y: 460 }, bottomRightCorner: { x: 600, y: 460 } }; }
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
  (window as unknown as { cv?: unknown }).cv = { Mat: class {}, imread: () => ({ delete() {} }) };
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
      <DocumentScanner programId="p1" regencyId="r1" slotId="slot-1" customName="" onDone={onDone} onClose={onClose} />,
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
    expect(body.get('program_id')).toBe('p1');
    expect(body.get('regency_id')).toBe('r1');
    expect(body.get('slot_id')).toBe('slot-1');
    expect(body.getAll('page')).toHaveLength(2);
    await waitFor(() => expect(onDone).toHaveBeenCalled());
  });

  it('shows the OpenCV-specific error on CDN load failure, then actually recovers on "Coba lagi" (not a stale cached rejection)', async () => {
    // This scenario needs full control over the OpenCV <script> load lifecycle, so unlike
    // the other test it does NOT rely on the global beforeEach's `window.cv` stub — it
    // deletes it immediately to force the component down the real CDN-script-tag path.
    delete (window as unknown as { cv?: unknown }).cv;

    const appendedScripts: HTMLScriptElement[] = [];
    const originalAppendChild = document.head.appendChild.bind(document.head);
    vi.spyOn(document.head, 'appendChild').mockImplementation(((node: Node) => {
      if (node instanceof HTMLScriptElement) appendedScripts.push(node);
      return originalAppendChild(node);
    }) as typeof document.head.appendChild);

    const onDone = vi.fn();
    const onClose = vi.fn();
    render(
      <DocumentScanner programId="p1" regencyId="r1" slotId="slot-1" customName="" onDone={onDone} onClose={onClose} />,
    );

    // First attempt: the CDN <script> fails to load.
    await waitFor(() => expect(appendedScripts).toHaveLength(1));
    appendedScripts[0].dispatchEvent(new Event('error'));

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toMatch(/Deteksi dokumen gagal dimuat/);
    expect(alert.textContent).not.toMatch(/Kamera tidak dapat diakses/);

    // "Coba lagi" must trigger a genuinely NEW load attempt (a fresh <script> tag), not
    // replay the already-rejected cached promise from the first attempt.
    fireEvent.click(screen.getByRole('button', { name: 'Coba lagi' }));
    await waitFor(() => expect(appendedScripts).toHaveLength(2));

    const video = document.querySelector('video');
    expect(video).toBeTruthy();
    attachStream(video as HTMLVideoElement);

    // Second attempt succeeds: opencv.js's UMD wrapper assigns `window.cv` on script
    // `load`, then calls `onRuntimeInitialized` once its WASM finishes compiling.
    (window as unknown as { cv?: { onRuntimeInitialized?: () => void } }).cv = {};
    appendedScripts[1].dispatchEvent(new Event('load'));
    await waitFor(() => expect(window.cv?.onRuntimeInitialized).toBeTypeOf('function'));
    window.cv!.onRuntimeInitialized!();

    await waitFor(() => expect(screen.queryByRole('button', { name: 'Ambil Halaman' })).toBeEnabled());
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });
});
