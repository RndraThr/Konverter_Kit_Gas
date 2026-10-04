import { useEffect, useRef, useState } from 'react';
import { BrowserMultiFormatReader } from '@zxing/browser';
import { LoaderCircle, RefreshCw, ScanLine } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';

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
    let controls: { stop: () => void } | undefined;
    setError(null);
    setStarting(true);
    const reader = new BrowserMultiFormatReader();
    const started = deviceId
      ? reader.decodeFromVideoDevice(deviceId, video, (result, _error, scanControls) => { if (result && !stopped) { stopped = true; scanControls.stop(); onResultRef.current(result.getText()); } })
      : reader.decodeFromConstraints({ video: { facingMode: { ideal: 'environment' } } }, video, (result, _error, scanControls) => { if (result && !stopped) { stopped = true; scanControls.stop(); onResultRef.current(result.getText()); } });
    started
      .then((activeControls) => {
        controls = activeControls;
        if (stopped) { activeControls.stop(); return; }
        setStarting(false);
        // Inspect the real track: mirror unless it is explicitly a rear camera (front/unknown -> mirror),
        // reflect the active device in the selector, and refresh labels now that permission is granted.
        const settings = (video.srcObject as MediaStream | null)?.getVideoTracks?.()[0]?.getSettings?.();
        setMirror(settings?.facingMode ? settings.facingMode !== 'environment' : true);
        if (settings?.deviceId) setActiveDeviceId(settings.deviceId);
        BrowserMultiFormatReader.listVideoInputDevices().then((list) => { if (!stopped) setDevices(list); }).catch(() => {});
      })
      .catch((cause) => { if (!stopped) { setStarting(false); setError(cameraErrorMessage(cause)); } });
    return () => { stopped = true; controls?.stop(); };
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
