import { useEffect, useRef, useState } from 'react';
import { BrowserMultiFormatReader } from '@zxing/browser';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';

// Camera barcode scanner, lazy-loaded so @zxing only downloads when a scan is actually started.
// Prefers a rear camera but falls back to whatever is available (e.g. a laptop's front webcam), lets
// the user switch when several cameras exist, and mirrors the preview for front-facing cameras so a
// held-up code lines up naturally. Mirroring is display-only — @zxing decodes the raw frames, so the
// flip never affects reads. The whole component unmounts on close, releasing the camera.
export default function BarcodeScanner({ onResult, onClose }: { onResult: (text: string) => void; onClose: () => void }) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const onResultRef = useRef(onResult);
  onResultRef.current = onResult;
  const [devices, setDevices] = useState<MediaDeviceInfo[]>([]);
  const [deviceId, setDeviceId] = useState<string | undefined>(undefined); // undefined = let the browser pick (prefers rear)
  const [activeDeviceId, setActiveDeviceId] = useState('');
  const [mirror, setMirror] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // (Re)start decoding whenever the chosen camera changes.
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    let stopped = false;
    let controls: { stop: () => void } | undefined;
    setError(null);
    const reader = new BrowserMultiFormatReader();
    const started = deviceId
      ? reader.decodeFromVideoDevice(deviceId, video, (result, _error, scanControls) => { if (result && !stopped) { stopped = true; scanControls.stop(); onResultRef.current(result.getText()); } })
      : reader.decodeFromConstraints({ video: { facingMode: { ideal: 'environment' } } }, video, (result, _error, scanControls) => { if (result && !stopped) { stopped = true; scanControls.stop(); onResultRef.current(result.getText()); } });
    started
      .then((activeControls) => {
        controls = activeControls;
        if (stopped) { activeControls.stop(); return; }
        // Inspect the real track: mirror unless it is explicitly a rear camera (front/unknown -> mirror),
        // reflect the active device in the selector, and refresh labels now that permission is granted.
        const settings = (video.srcObject as MediaStream | null)?.getVideoTracks?.()[0]?.getSettings?.();
        setMirror(settings?.facingMode ? settings.facingMode !== 'environment' : true);
        if (settings?.deviceId) setActiveDeviceId(settings.deviceId);
        BrowserMultiFormatReader.listVideoInputDevices().then((list) => { if (!stopped) setDevices(list); }).catch(() => {});
      })
      .catch(() => setError('Kamera tidak dapat diakses. Beri izin kamera pada browser, lalu coba lagi.'));
    return () => { stopped = true; controls?.stop(); };
  }, [deviceId]);

  const selectValue = deviceId ?? activeDeviceId;

  return <Dialog open onOpenChange={(nextOpen) => { if (!nextOpen) onClose(); }}>
    <DialogContent aria-label="Scan Barcode" className="max-w-md">
      <DialogHeader>
        <DialogTitle>Scan Barcode</DialogTitle>
        <DialogDescription>Arahkan kamera ke barcode pada unit — nomor seri akan terisi otomatis.</DialogDescription>
      </DialogHeader>
      {error ? <p className="text-sm text-destructive" role="alert">{error}</p> : <div className="space-y-3">
        <div className="overflow-hidden rounded-lg border bg-black">
          <video ref={videoRef} className="aspect-video w-full object-cover" style={mirror ? { transform: 'scaleX(-1)' } : undefined} autoPlay muted playsInline />
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
