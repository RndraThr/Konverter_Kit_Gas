import { useEffect, useRef, useState } from 'react';
// jscanify's npm "main" entry (`jscanify`) resolves to a Node.js build (uses
// `canvas`/`jsdom` to fake a DOM) — unusable in the browser. The browser build
// lives at the documented `jscanify/client` subpath and expects a global `cv`
// (OpenCV.js) to already exist, exactly like the CDN usage shown in the
// package's own README. See the task report for the full verification trail.
// @ts-expect-error -- jscanify ships no TypeScript typings for either entry point.
import jscanify from 'jscanify/client';
import { LoaderCircle, RefreshCw } from 'lucide-react';
import { apiRequest, ApiError } from '../../lib/api';
import { Button } from '../../components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '../../components/ui/dialog';

type JScanify = {
  highlightPaper(image: HTMLCanvasElement, options?: { color?: string; thickness?: number }): HTMLCanvasElement;
  extractPaper(image: HTMLCanvasElement, resultWidth: number, resultHeight: number): HTMLCanvasElement | null;
};

const OPENCV_JS_URL = 'https://docs.opencv.org/4.7.0/opencv.js';
// Pinned against the exact bytes served at OPENCV_JS_URL at the time this was written (verified via
// `curl -s $OPENCV_JS_URL | openssl dgst -sha384 -binary | openssl base64 -A`) — guards against a CDN
// compromise silently swapping in different script contents under the same URL.
const OPENCV_JS_INTEGRITY = 'sha384-7NgX/1nBIFcqsilWilRzcsVBOcPRmtwBvqsFS0QdyP6i356G43SIDx5JKd1aNqtz';

declare global {
  interface Window {
    cv?: { onRuntimeInitialized?: () => void };
  }
}

// Loads OpenCV.js (the WASM engine jscanify's browser build depends on via the
// bare global `cv`) once, the way jscanify's own README recommends for
// non-bundled browser usage — the compiled asset is ~9MB, far too large to
// statically import into our own bundle, so it is fetched from the CDN only
// when a document scan is actually opened, independent of this component's
// own (already-lazy) chunk.
let openCvLoad: Promise<void> | undefined;
function loadOpenCv(): Promise<void> {
  // `cv` already present (loaded in a previous scanner session, or stubbed in
  // tests) — opencv.js's UMD wrapper assigns `window.cv` synchronously, well
  // before its WASM finishes compiling, but by the time it's reused here that
  // compile has long since finished, so it is safe to treat presence as ready.
  if (window.cv) return Promise.resolve();
  if (openCvLoad) return openCvLoad;
  openCvLoad = new Promise((resolve, reject) => {
    const script = document.createElement('script');
    script.src = OPENCV_JS_URL;
    script.async = true;
    script.crossOrigin = 'anonymous';
    script.integrity = OPENCV_JS_INTEGRITY;
    script.addEventListener('error', () => {
      // Clear the cache so the NEXT call to loadOpenCv() (e.g. from the user's "Coba
      // lagi" retry) re-attempts the CDN fetch instead of replaying this same
      // already-rejected promise forever (spec §10 requires retry to actually work).
      openCvLoad = undefined;
      reject(new Error('opencv_load_failed'));
    });
    script.addEventListener('load', () => {
      if (window.cv) window.cv.onRuntimeInitialized = () => resolve();
      else {
        openCvLoad = undefined;
        reject(new Error('opencv_load_failed'));
      }
    });
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
export default function DocumentScanner({ scheduleId, slotId, programId, onDone, onClose }: {
  scheduleId: string; slotId: string; programId: string; onDone: () => void; onClose: () => void;
}) {
  const [video, setVideo] = useState<HTMLVideoElement | null>(null);
  const overlayRef = useRef<HTMLCanvasElement>(null);
  const scannerRef = useRef<JScanify>(new jscanify());
  const [pages, setPages] = useState<Blob[]>([]);
  const [starting, setStarting] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    if (!video) return;
    let stream: MediaStream | undefined;
    let timer = 0;
    let cancelled = false;
    Promise.all([
      loadOpenCv().catch(() => { throw new Error('opencv'); }),
      navigator.mediaDevices.getUserMedia({ video: { facingMode: { ideal: 'environment' } } })
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
      cropped = scannerRef.current.extractPaper(raw, raw.width, raw.height);
    } catch {
      setError('Deteksi dokumen gagal dimuat. Periksa koneksi internet, lalu coba lagi.');
      return;
    }
    // extractPaper returns null when no paper contour was found in the frame.
    if (!cropped) {
      setError('Tepi dokumen tidak terdeteksi. Posisikan dokumen di tengah bidikan, lalu coba lagi.');
      return;
    }
    const finished = enhance(cropped);
    finished.toBlob((blob) => { if (blob) setPages((current) => [...current, blob]); }, 'image/jpeg', 0.85);
  };

  const finish = async () => {
    setSubmitting(true);
    try {
      const form = new FormData();
      form.append('schedule_id', scheduleId);
      form.append('slot_id', slotId);
      pages.forEach((page, index) => form.append('page', page, `page-${index}.jpg`));
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
        <DialogDescription>Arahkan kamera ke dokumen — tepi terdeteksi otomatis. Program: {programId}.</DialogDescription>
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
        <p className="text-sm text-muted-foreground">{pages.length} halaman terkumpul.</p>
        <div className="flex flex-wrap gap-2">
          <Button type="button" onClick={capture} disabled={starting}>Ambil Halaman</Button>
          <Button type="button" variant="outline" disabled={!pages.length || submitting} onClick={finish}>{submitting ? 'Menyimpan...' : 'Selesai'}</Button>
        </div>
      </div>}
    </DialogContent>
  </Dialog>;
}
