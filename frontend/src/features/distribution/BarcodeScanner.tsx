import { useEffect, useRef, useState } from 'react';
import { BrowserMultiFormatReader } from '@zxing/browser';
import { BarcodeFormat, DecodeHintType } from '@zxing/library';
import { LoaderCircle, RefreshCw, ScanLine } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';

// The units carry 1D linear barcodes, so restrict decoding to the common linear symbologies
// and skip 2D formats (QR/DataMatrix/…) entirely — an unrestricted reader occasionally
// mis-decodes a linear code as some other format, producing a wrong value. TRY_HARDER trades a
// little speed for noticeably more reliable reads under blur/glare.
const decodeHints = new Map<DecodeHintType, unknown>([
  [DecodeHintType.POSSIBLE_FORMATS, [
    BarcodeFormat.CODE_128, BarcodeFormat.CODE_39, BarcodeFormat.CODE_93,
    BarcodeFormat.CODABAR, BarcodeFormat.ITF,
    BarcodeFormat.EAN_13, BarcodeFormat.EAN_8, BarcodeFormat.UPC_A, BarcodeFormat.UPC_E,
  ]],
  [DecodeHintType.TRY_HARDER, true],
]);

// A single camera frame can decode to the wrong value under motion blur, glare, or a
// partially-framed code — and those misreads are random, so they never repeat identically.
// Requiring the same text from several consecutive decodes accepts only a value the scanner
// keeps agreeing on, which eliminates the occasional wrong serial number. Formats with a
// mandatory check character already reject a damaged read inside the decoder, so they need
// one fewer confirming frame than formats whose check is optional (Code 39/93, Codabar, ITF).
const STRONG_MATCHES = 2;
const WEAK_MATCHES = 3;
const STRONG_ZXING_FORMATS = new Set<BarcodeFormat>([
  BarcodeFormat.CODE_128, BarcodeFormat.EAN_13, BarcodeFormat.EAN_8, BarcodeFormat.UPC_A, BarcodeFormat.UPC_E,
]);

// Browsers default getUserMedia to roughly 640x480, where a thin serial barcode only resolves
// from very close — too close to fit the frame or focus. The native detector reads Full HD
// cheaply; the JavaScript decoder's cost grows with every pixel, so it gets 720p instead.
const NATIVE_RESOLUTION = { width: 1920, height: 1080 };
const ZXING_RESOLUTION = { width: 1280, height: 720 };

function videoConstraints(deviceId: string | undefined, resolution: { width: number; height: number }): MediaTrackConstraints {
  return {
    ...(deviceId ? { deviceId: { exact: deviceId } } : { facingMode: { ideal: 'environment' } }),
    width: { ideal: resolution.width },
    height: { ideal: resolution.height },
  };
}

// Shape Detection API: on Chrome for Android (and ChromeOS) BarcodeDetector runs the operating
// system's native barcode engine — the same one the mobile app uses — which is far faster than
// decoding frames in JavaScript. Other browsers lack it and keep the @zxing path.
type DetectedBarcode = { rawValue: string; format: string; boundingBox: { x: number; y: number; width: number; height: number } };
type NativeDetector = { detect: (source: HTMLVideoElement) => Promise<DetectedBarcode[]> };
type NativeDetectorConstructor = { new (options: { formats: string[] }): NativeDetector; getSupportedFormats: () => Promise<string[]> };
const NATIVE_LINEAR_FORMATS = ['code_128', 'code_39', 'code_93', 'codabar', 'itf', 'ean_13', 'ean_8', 'upc_a', 'upc_e'];
const STRONG_NATIVE_FORMATS = new Set(['code_128', 'ean_13', 'ean_8', 'upc_a', 'upc_e']);
const NATIVE_SCAN_INTERVAL_MS = 60;

async function createNativeDetector(): Promise<NativeDetector | null> {
  const Detector = (globalThis as { BarcodeDetector?: NativeDetectorConstructor }).BarcodeDetector;
  if (!Detector) return null;
  try {
    const supported = await Detector.getSupportedFormats();
    const formats = NATIVE_LINEAR_FORMATS.filter((format) => supported.includes(format));
    return formats.length ? new Detector({ formats }) : null;
  } catch {
    return null;
  }
}

// Only a code whose centre sits in the middle of the frame — where the on-screen guide is — counts,
// so a neighbouring product barcode at the edge of the picture is never picked up by mistake.
function insideScanFrame(box: DetectedBarcode['boundingBox'], width: number, height: number): boolean {
  if (!width || !height) return true;
  const cx = (box.x + box.width / 2) / width;
  const cy = (box.y + box.height / 2) / height;
  return cx >= 0.1 && cx <= 0.9 && cy >= 0.15 && cy <= 0.85;
}

function cameraErrorMessage(error: unknown): string {
  const name = error instanceof DOMException ? error.name : undefined;
  switch (name) {
    case 'NotAllowedError':
    case 'PermissionDeniedError':
      return 'Izin kamera ditolak. Aktifkan izin kamera untuk browser ini di pengaturan perangkat, lalu coba lagi.';
    case 'NotFoundError':
    case 'DevicesNotFoundError':
      return 'Kamera tidak ditemukan pada perangkat ini.';
    case 'NotReadableError':
    case 'TrackStartError':
      return 'Kamera sedang dipakai aplikasi lain. Tutup aplikasi lain yang menggunakan kamera, lalu coba lagi.';
    case 'OverconstrainedError':
      return 'Kamera tidak mendukung pengaturan yang diminta. Coba pilih kamera lain.';
    default:
      return 'Kamera tidak dapat diakses. Beri izin kamera pada browser, lalu coba lagi.';
  }
}

// Camera barcode scanner, lazy-loaded so @zxing only downloads when a scan is actually started.
// Prefers a rear camera but falls back to whatever is available (e.g. a laptop's front webcam), lets
// the user switch when several cameras exist, and mirrors the preview for front-facing cameras so a
// held-up code lines up naturally. Mirroring is display-only — @zxing decodes the raw frames, so the
// flip never affects reads. The whole component unmounts on close, releasing the camera.
export default function BarcodeScanner({ onResult, onClose }: { onResult: (text: string) => void; onClose: () => void }) {
  // A ref callback (via setState) rather than a plain useRef: the <video> sits inside a Dialog
  // portal, which does not guarantee its children exist in the DOM by the time this component's
  // first effect runs. A callback ref fires exactly when the node mounts/unmounts, so the effect
  // below — keyed on `video` — reliably starts once it does, instead of silently reading a null
  // ref on a useEffect that never re-runs since none of its other dependencies ever change.
  const [video, setVideo] = useState<HTMLVideoElement | null>(null);
  const onResultRef = useRef(onResult);
  onResultRef.current = onResult;
  const [devices, setDevices] = useState<MediaDeviceInfo[]>([]);
  const [deviceId, setDeviceId] = useState<string | undefined>(undefined); // undefined = let the browser pick (prefers rear)
  const [activeDeviceId, setActiveDeviceId] = useState('');
  const [mirror, setMirror] = useState(false);
  const [starting, setStarting] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);

  // (Re)start decoding whenever the video node mounts, the chosen camera changes, or a retry is requested.
  useEffect(() => {
    if (!video) return;
    let stopped = false;
    let stopScan: (() => void) | undefined;
    let lastText = '';
    let matches = 0;
    setError(null);
    setStarting(true);
    // Accept a value once enough consecutive decodes agree on it (STRONG_MATCHES for formats with a
    // mandatory check character, WEAK_MATCHES otherwise). Empty frames are ignored without resetting
    // the streak; a decode that disagrees restarts the count on the new value.
    const accept = (text: string, strong: boolean): boolean => {
      if (stopped) return true;
      if (text === lastText) { matches += 1; } else { lastText = text; matches = 1; }
      if (matches < (strong ? STRONG_MATCHES : WEAK_MATCHES)) return false;
      stopped = true;
      stopScan?.();
      onResultRef.current(text);
      return true;
    };
    const started = (stop: () => void) => {
      if (stopped) { stop(); return; }
      stopScan = stop;
      setStarting(false);
      // Inspect the real track: mirror unless it is explicitly a rear camera (front/unknown -> mirror),
      // reflect the active device in the selector, and refresh labels now that permission is granted.
      const settings = (video.srcObject as MediaStream | null)?.getVideoTracks?.()[0]?.getSettings?.();
      setMirror(settings?.facingMode ? settings.facingMode !== 'environment' : true);
      if (settings?.deviceId) setActiveDeviceId(settings.deviceId);
      BrowserMultiFormatReader.listVideoInputDevices().then((list) => { if (!stopped) setDevices(list); }).catch(() => {});
    };
    const fail = (cause: unknown) => { if (!stopped) { setStarting(false); setError(cameraErrorMessage(cause)); } };

    const startNative = async (detector: NativeDetector) => {
      const stream = await navigator.mediaDevices.getUserMedia({ video: videoConstraints(deviceId, NATIVE_RESOLUTION) });
      let timer = 0;
      const release = () => {
        window.clearTimeout(timer);
        stream.getTracks().forEach((track) => track.stop());
        if (video.srcObject === stream) video.srcObject = null;
      };
      if (stopped) { release(); return; }
      video.srcObject = stream;
      try { await video.play(); } catch { /* autoplay attribute keeps trying */ }
      const tick = async () => {
        if (stopped) return;
        if (video.readyState >= 2) {
          try {
            const found = (await detector.detect(video))
              .find((code) => code.rawValue.trim() && insideScanFrame(code.boundingBox, video.videoWidth, video.videoHeight));
            if (found && accept(found.rawValue.trim(), STRONG_NATIVE_FORMATS.has(found.format))) return;
          } catch { /* a frame the detector could not read; try the next one */ }
        }
        if (!stopped) timer = window.setTimeout(tick, NATIVE_SCAN_INTERVAL_MS);
      };
      started(release);
      void tick();
    };

    const startZxing = () => {
      const reader = new BrowserMultiFormatReader(decodeHints);
      return reader.decodeFromConstraints({ video: videoConstraints(deviceId, ZXING_RESOLUTION) }, video, (result, _error, scanControls) => {
        if (!result) return;
        const format = (result as { getBarcodeFormat?: () => BarcodeFormat }).getBarcodeFormat?.();
        if (accept(result.getText(), format !== undefined && STRONG_ZXING_FORMATS.has(format))) scanControls.stop();
      }).then((controls) => started(() => controls.stop()));
    };

    createNativeDetector()
      .then((detector) => (detector ? startNative(detector) : startZxing()))
      .catch(fail);
    return () => { stopped = true; stopScan?.(); };
  }, [video, deviceId, attempt]);

  const selectValue = deviceId ?? activeDeviceId;
  // Clearing error here (not just inside the effect) matters: the error branch below doesn't
  // render the <video> element, so without this the video node never remounts to give the
  // video-gated effect something to attach to, and the retry would never actually fire.
  const retry = () => { setError(null); setAttempt((value) => value + 1); };

  return <Dialog open onOpenChange={(nextOpen) => { if (!nextOpen) onClose(); }}>
    <DialogContent aria-label="Scan Barcode" className="max-w-md">
      <DialogHeader>
        <DialogTitle>Scan Barcode</DialogTitle>
        <DialogDescription>Arahkan kamera ke barcode pada unit — nomor seri akan terisi otomatis.</DialogDescription>
      </DialogHeader>
      {error ? <div className="space-y-3">
        <p className="text-sm text-destructive" role="alert">{error}</p>
        <Button type="button" variant="outline" onClick={retry}><RefreshCw aria-hidden="true" />Coba lagi</Button>
      </div> : <div className="space-y-3">
        <div className="relative overflow-hidden rounded-lg border bg-black">
          <video ref={setVideo} className="aspect-video w-full object-cover" style={mirror ? { transform: 'scaleX(-1)' } : undefined} autoPlay muted playsInline />
          {starting && <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 bg-black/70 text-white">
            <LoaderCircle className="size-6 animate-spin" aria-hidden="true" />
            <span className="text-xs font-medium">Menyiapkan kamera...</span>
          </div>}
          {!starting && <div className="pointer-events-none absolute inset-0 flex items-center justify-center p-10">
            <div className="relative aspect-video w-full max-w-xs">
              <ScanLine className="absolute inset-x-0 top-1/2 mx-auto size-6 -translate-y-1/2 text-primary/80" aria-hidden="true" />
              <span className="absolute top-0 left-0 size-6 rounded-tl-lg border-t-4 border-l-4 border-white/90" />
              <span className="absolute top-0 right-0 size-6 rounded-tr-lg border-t-4 border-r-4 border-white/90" />
              <span className="absolute bottom-0 left-0 size-6 rounded-bl-lg border-b-4 border-l-4 border-white/90" />
              <span className="absolute bottom-0 right-0 size-6 rounded-br-lg border-r-4 border-b-4 border-white/90" />
            </div>
          </div>}
        </div>
        {devices.length > 1 && <div className="grid gap-1.5">
          <Label id="barcode-camera-label">Kamera</Label>
          <Select value={selectValue} onValueChange={(value) => setDeviceId(value ?? undefined)}>
            <SelectTrigger className="w-full" aria-labelledby="barcode-camera-label"><SelectValue placeholder="Pilih kamera" /></SelectTrigger>
            <SelectContent>{devices.map((device, index) => <SelectItem key={device.deviceId} value={device.deviceId}>{device.label || `Kamera ${index + 1}`}</SelectItem>)}</SelectContent>
          </Select>
        </div>}
      </div>}
    </DialogContent>
  </Dialog>;
}
