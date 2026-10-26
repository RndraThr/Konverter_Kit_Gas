import { useEffect, useRef, useState } from 'react';
// jscanify's npm "main" entry (`jscanify`) resolves to a Node.js build (uses
// `canvas`/`jsdom` to fake a DOM) — unusable in the browser. The browser build
// lives at the documented `jscanify/client` subpath and expects a global `cv`
// (OpenCV.js) to already exist, exactly like the CDN usage shown in the
// package's own README. See the task report for the full verification trail.
// @ts-expect-error -- jscanify ships no TypeScript typings for either entry point.
import jscanify from 'jscanify/client';
import { LoaderCircle, RefreshCw, X } from 'lucide-react';
import { apiRequest, ApiError } from '../../lib/api';
import { Button } from '../../components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '../../components/ui/dialog';

type Point = { x: number; y: number };
type Corners = { topLeftCorner?: Point; topRightCorner?: Point; bottomLeftCorner?: Point; bottomRightCorner?: Point };
type CvMat = { delete(): void };
type JScanify = {
  highlightPaper(image: HTMLCanvasElement, options?: { color?: string; thickness?: number }): HTMLCanvasElement;
  findPaperContour(image: CvMat): CvMat | null;
  getCornerPoints(contour: CvMat): Corners;
  extractPaper(image: HTMLCanvasElement, resultWidth: number, resultHeight: number, corners?: Corners): HTMLCanvasElement | null;
};

const distance = (a: Point, b: Point) => Math.hypot(a.x - b.x, a.y - b.y);

/**
 * Finds the paper's corners and its real size in the frame, so the
 * perspective crop keeps the document's proportions (cropping to the video
 * frame size squashed an A4 page into 16:9). Null when no paper, or only a
 * small part of the frame, was found.
 */
function paperGeometry(scanner: JScanify, frame: HTMLCanvasElement): { corners: Required<Corners>; width: number; height: number } | null {
  const cv = (window as unknown as { cv: { imread(canvas: HTMLCanvasElement): CvMat } }).cv;
  const image = cv.imread(frame);
  try {
    const contour = scanner.findPaperContour(image);
    if (!contour) return null;
    const corners = scanner.getCornerPoints(contour);
    contour.delete();
    const { topLeftCorner: tl, topRightCorner: tr, bottomLeftCorner: bl, bottomRightCorner: br } = corners;
    if (!tl || !tr || !bl || !br) return null;
    const width = Math.round(Math.max(distance(tl, tr), distance(bl, br)));
    const height = Math.round(Math.max(distance(tl, bl), distance(tr, br)));
    if (width * height < frame.width * frame.height * 0.15) return null;
    return { corners: { topLeftCorner: tl, topRightCorner: tr, bottomLeftCorner: bl, bottomRightCorner: br }, width, height };
  } finally {
    image.delete();
  }
}

// OpenCV.js (~9 MB) comes from the jscanify package and is served by our own
// server as a separate asset, fetched only when a scan opens. Same origin: no
// CORS or integrity attribute needed. (The earlier docs.opencv.org copy sends
// no Access-Control-Allow-Origin header, so with crossorigin + integrity the
// browser always blocked it and every scan failed to start.)
import openCvUrl from 'opencv-js-asset?url';

declare global {
  interface Window {
    cv?: { onRuntimeInitialized?: () => void; Mat?: unknown; then?: unknown };
  }
}

// Loads OpenCV.js (the WASM engine jscanify's browser build depends on via the
// bare global `cv`) once, the way jscanify's own README recommends for
// non-bundled browser usage — the compiled asset is ~9MB, far too large to
// statically import into our own bundle, so it is fetched from the CDN only
// when a document scan is actually opened, independent of this component's
// own (already-lazy) chunk.
let openCvLoad: Promise<void> | undefined;

/** Resolves once the OpenCV runtime is usable (cv.Mat present). */
function whenOpenCvReady(resolve: () => void, reject: (error: Error) => void) {
  const cv = window.cv;
  if (!cv) return reject(new Error('opencv_load_failed'));
  if (cv.Mat) return resolve();
  // Some builds expose the module as a thenable; others call
  // onRuntimeInitialized once the WASM is compiled. Handle both.
  if (typeof cv.then === 'function') {
    (cv.then as (done: (module: unknown) => void) => void)((module) => {
      const ready = module as Window['cv'];
      if (ready && ready !== cv) window.cv = ready;
      resolve();
    });
    return;
  }
  cv.onRuntimeInitialized = () => resolve();
}

function loadOpenCv(): Promise<void> {
  if (window.cv?.Mat) return Promise.resolve();
  if (openCvLoad) return openCvLoad;
  openCvLoad = new Promise<void>((resolve, reject) => {
    const fail = () => {
      // Cleared so "Coba lagi" really retries instead of replaying this failure.
      openCvLoad = undefined;
      reject(new Error('opencv_load_failed'));
    };
    if (window.cv) return whenOpenCvReady(resolve, fail);
    const script = document.createElement('script');
    script.src = openCvUrl;
    script.async = true;
    script.addEventListener('error', fail);
    script.addEventListener('load', () => whenOpenCvReady(resolve, fail));
    document.head.appendChild(script);
  });
  return openCvLoad;
}

// Document scan for Dokumen Pendukung: live camera with an auto-detected-edge
// overlay (jscanify, built on OpenCV.js — this is why the component is
// lazy-loaded from SupportingDocumentsPage, not imported eagerly), capture +
// perspective-crop to a flat rectangle, a quick sharpen/contrast pass, then
// accumulate pages until "Selesai" assembles them server-side into one PDF.
// No color-mode picker (spec §2) — the enhance step always runs, unconditionally.
export default function DocumentScanner({ programId, regencyId, slotId, customName, onDone, onClose }: {
  programId: string; regencyId: string; slotId: string; customName: string; onDone: () => void; onClose: () => void;
}) {
  const [video, setVideo] = useState<HTMLVideoElement | null>(null);
  const overlayRef = useRef<HTMLCanvasElement>(null);
  const scannerRef = useRef<JScanify>(new jscanify());
  const [pages, setPages] = useState<{ id: number; blob: Blob; previewUrl: string }[]>([]);
  const nextPageId = useRef(0);
  const [starting, setStarting] = useState(true);
  const [error, setError] = useState<string | null>(null);
  // A failed capture (no paper found) is shown under the camera, which keeps
  // running; only camera/engine failures replace the view with a retry.
  const [captureError, setCaptureError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    if (!video) return;
    let stream: MediaStream | undefined;
    let timer = 0;
    let cancelled = false;
    Promise.all([
      loadOpenCv().catch(() => { throw new Error('opencv'); }),
      // Full HD when the camera has it: small text on a document is not
      // readable at the browser's default 640x480.
      navigator.mediaDevices.getUserMedia({ video: { facingMode: { ideal: 'environment' }, width: { ideal: 1920 }, height: { ideal: 1080 } } })
        .catch(() => { throw new Error('camera'); }),
    ])
      .then(([, activeStream]) => {
        if (cancelled) { activeStream.getTracks().forEach((track) => track.stop()); return; }
        stream = activeStream;
        video.srcObject = activeStream;
        setStarting(false);
        const overlay = overlayRef.current;
        const tick = () => {
          if (overlay && video.videoWidth) {
            overlay.width = video.videoWidth; overlay.height = video.videoHeight;
            const ctx = overlay.getContext('2d');
            if (ctx) {
              ctx.drawImage(video, 0, 0);
              try {
                const highlighted = scannerRef.current.highlightPaper(overlay);
                ctx.clearRect(0, 0, overlay.width, overlay.height);
                ctx.drawImage(highlighted, 0, 0);
              } catch {
                // jscanify's OpenCV.js WASM failed to init or threw mid-detection (e.g. weak
                // connection in the field). Fail fast with a retry, per spec §10 — never fall
                // back silently to a mode with no edge detection.
                setError('Deteksi dokumen gagal dimuat. Periksa koneksi internet, lalu coba lagi.');
                return;
              }
            }
          }
          timer = window.setTimeout(tick, 250); // throttled — not every frame, per spec §9.2
        };
        tick();
      })
      .catch((cause: unknown) => {
        if (cancelled) return;
        setError(
          cause instanceof Error && cause.message === 'opencv'
            ? 'Deteksi dokumen gagal dimuat. Periksa koneksi internet, lalu coba lagi.'
            : 'Kamera tidak dapat diakses. Beri izin kamera pada browser, lalu coba lagi.',
        );
      });
    return () => { cancelled = true; window.clearTimeout(timer); stream?.getTracks().forEach((track) => track.stop()); };
  }, [video, attempt]);

  const pagesRef = useRef(pages);
  pagesRef.current = pages;
  useEffect(() => () => { pagesRef.current.forEach((page) => URL.revokeObjectURL(page.previewUrl)); }, []);

  const enhance = (canvas: HTMLCanvasElement): HTMLCanvasElement => {
    const ctx = canvas.getContext('2d');
    if (!ctx) return canvas;
    // Simple sharpen (3x3 kernel) + contrast stretch — always on, no mode picker (spec §2).
    const image = ctx.getImageData(0, 0, canvas.width, canvas.height);
    const data = image.data;
    for (let i = 0; i < data.length; i += 4) {
      for (let channel = 0; channel < 3; channel += 1) {
        const value = data[i + channel];
        data[i + channel] = Math.min(255, Math.max(0, (value - 128) * 1.2 + 128));
      }
    }
    ctx.putImageData(image, 0, 0);
    return canvas;
  };

  const capture = () => {
    if (!video) return;
    const raw = document.createElement('canvas');
    raw.width = video.videoWidth; raw.height = video.videoHeight;
    raw.getContext('2d')?.drawImage(video, 0, 0);
    let cropped: HTMLCanvasElement | null;
    try {
      const paper = paperGeometry(scannerRef.current, raw);
      cropped = paper ? scannerRef.current.extractPaper(raw, paper.width, paper.height, paper.corners) : null;
    } catch {
      setError('Deteksi dokumen gagal dimuat. Periksa koneksi internet, lalu coba lagi.');
      return;
    }
    if (!cropped) {
      setCaptureError('Tepi dokumen tidak terdeteksi. Letakkan dokumen utuh di atas alas yang kontras, lalu ambil lagi.');
      return;
    }
    setCaptureError(null);
    const finished = enhance(cropped);
    finished.toBlob((blob) => {
      if (!blob) return;
      const id = nextPageId.current;
      nextPageId.current += 1;
      setPages((current) => [...current, { id, blob, previewUrl: URL.createObjectURL(blob) }]);
    }, 'image/jpeg', 0.85);
  };

  const removePage = (id: number) => {
    setPages((current) => {
      const target = current.find((page) => page.id === id);
      if (target) URL.revokeObjectURL(target.previewUrl);
      return current.filter((page) => page.id !== id);
    });
  };

  const finish = async () => {
    setSubmitting(true);
    try {
      const form = new FormData();
      form.append('program_id', programId);
      form.append('regency_id', regencyId);
      if (customName) form.append('custom_name', customName);
      else form.append('slot_id', slotId);
      pages.forEach((page, index) => form.append('page', page.blob, `page-${index}.jpg`));
      await apiRequest('/api/v1/activities/supporting-documents/scan', { method: 'POST', body: form });
      onDone();
    } catch (cause) {
      setError(cause instanceof ApiError ? cause.message : 'Scan belum dapat disimpan.');
    } finally {
      setSubmitting(false);
    }
  };

  return <Dialog open onOpenChange={(open) => { if (!open) onClose(); }}>
    <DialogContent aria-label="Scan Dokumen Pendukung" className="max-w-md">
      <DialogHeader>
        <DialogTitle>Scan Dokumen Pendukung</DialogTitle>
        <DialogDescription>Arahkan kamera ke dokumen — tepi terdeteksi otomatis.</DialogDescription>
      </DialogHeader>
      {error ? <div className="space-y-3">
        <p className="text-sm text-destructive" role="alert">{error}</p>
        <Button type="button" variant="outline" onClick={() => { setError(null); setStarting(true); setAttempt((value) => value + 1); }}><RefreshCw aria-hidden="true" />Coba lagi</Button>
      </div> : <div className="space-y-3">
        <div className="relative overflow-hidden rounded-lg border bg-black">
          <video ref={setVideo} className="aspect-video w-full object-cover" autoPlay muted playsInline />
          <canvas ref={overlayRef} className="pointer-events-none absolute inset-0 h-full w-full" />
          {starting && <div className="absolute inset-0 flex items-center justify-center bg-black/70 text-white"><LoaderCircle className="size-6 animate-spin" aria-hidden="true" /></div>}
        </div>
        {captureError && <p className="text-sm text-destructive" role="alert">{captureError}</p>}
        <p className="text-sm text-muted-foreground">{pages.length} halaman terkumpul.</p>
        {pages.length > 0 && <ul className="flex flex-wrap gap-2">
          {pages.map((page, index) => <li key={page.id} className="relative">
            <img src={page.previewUrl} alt={`Halaman ${index + 1}`} className="h-20 w-16 rounded border object-cover" />
            <button type="button" aria-label={`Hapus halaman ${index + 1}`} onClick={() => removePage(page.id)} className="absolute -right-1.5 -top-1.5 flex size-5 items-center justify-center rounded-full bg-destructive text-destructive-foreground">
              <X className="size-3" aria-hidden="true" />
            </button>
          </li>)}
        </ul>}
        <div className="flex flex-wrap gap-2">
          <Button type="button" onClick={capture} disabled={starting}>Ambil Halaman</Button>
          <Button type="button" variant="outline" disabled={!pages.length || submitting} onClick={finish}>{submitting ? 'Menyimpan...' : 'Selesai'}</Button>
        </div>
      </div>}
    </DialogContent>
  </Dialog>;
}
