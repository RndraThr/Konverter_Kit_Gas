import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { Camera, Check, LoaderCircle, RefreshCw, RotateCcw, Settings, SwitchCamera, X } from 'lucide-react'
import { Button, buttonVariants } from '@/components/ui/button'
import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog'
import { cameraErrorMessage } from '@/lib/cameraErrors'
import { cn } from '@/lib/utils'
import { resizeImage } from './imageResize'
import { AddressTracker, formatCoordinates, getFix, type Fix } from './location'
import type { CapturedPhoto } from './types'
import { useWatermarkLogo } from './useWatermarkLogo'
import { createWatermarkGestures, type ElementKey } from './watermarkGestures'
import { coverCrop, watermarkLines, type Rect, type WatermarkLayout } from './watermarkLayout'
import { paintWatermark } from './watermarkRender'
import { loadWatermarkSettings, saveWatermarkSettings, type ElementPlacement, type WatermarkSettings } from './watermarkSettings'

const RESOLUTION = { width: { ideal: 1920 }, height: { ideal: 1080 } }
// GPS is refreshed in the background so the shutter never waits on it.
const FIX_REFRESH_MS = 15_000
// Intermediate encode of the composed frame before the quality preset re-encodes it.
const COMPOSE_JPEG_QUALITY = 0.98

type Review = { file: File; url: string; capturedAt: Date; fix: Fix | null }

function videoConstraints(deviceId: string | undefined): MediaStreamConstraints {
  return deviceId
    ? { video: { deviceId: { exact: deviceId }, ...RESOLUTION } }
    : { video: { facingMode: { ideal: 'environment' }, ...RESOLUTION } }
}

function stopStream(stream: MediaStream | null) {
  stream?.getTracks().forEach((track) => track.stop())
}

function canvasToBlob(canvas: HTMLCanvasElement, type: string, quality: number): Promise<Blob> {
  return new Promise((resolve, reject) => {
    canvas.toBlob((blob) => (blob ? resolve(blob) : reject(new Error('Gagal menyusun foto'))), type, quality)
  })
}

/**
 * Full-screen camera that burns the watermark (logo, date/time, address, coordinates) into the photo,
 * matching the mobile app. The live overlay and the captured photo are drawn by the same layout and
 * render functions on a frame cropped to the displayed box, so the photo matches the preview. Lazy-load it.
 */
export default function WatermarkCamera({ title, programId, onCapture, onClose }: {
  title: string
  programId?: string
  onCapture: (photo: CapturedPhoto) => void
  onClose: () => void
}) {
  // Callback refs (via setState): the dialog portal does not guarantee the nodes exist on first effect run.
  const [video, setVideo] = useState<HTMLVideoElement | null>(null)
  const [overlay, setOverlay] = useState<HTMLCanvasElement | null>(null)
  const [stage, setStage] = useState<HTMLDivElement | null>(null)
  const streamRef = useRef<MediaStream | null>(null)

  const [settings, setSettings] = useState(loadWatermarkSettings)
  const settingsRef = useRef(settings)
  settingsRef.current = settings

  const [devices, setDevices] = useState<MediaDeviceInfo[]>([])
  const [deviceId, setDeviceId] = useState<string | undefined>(undefined)
  const [activeDeviceId, setActiveDeviceId] = useState('')
  const [mirror, setMirror] = useState(false)
  const [starting, setStarting] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [attempt, setAttempt] = useState(0)
  const [box, setBox] = useState({ width: 0, height: 0 })
  const [now, setNow] = useState(() => new Date())
  const [fix, setFix] = useState<Fix | null>(null)
  const fixRef = useRef<Fix | null>(null)
  const [address, setAddress] = useState('')
  const [busy, setBusy] = useState(false)
  const [review, setReview] = useState<Review | null>(null)
  const layoutRef = useRef<WatermarkLayout | null>(null)

  const logo = useWatermarkLogo(programId, settings.customLogo)
  const tracker = useMemo(() => new AddressTracker(), [])

  // Camera stream: (re)started for the chosen device, on retry, and after leaving the review screen.
  useEffect(() => {
    if (!video || review) return
    let stopped = false
    setError(null)
    setStarting(true)
    navigator.mediaDevices.getUserMedia(videoConstraints(deviceId))
      .then((stream) => {
        if (stopped) { stopStream(stream); return }
        streamRef.current = stream
        video.srcObject = stream // the element is autoPlay + muted + playsInline
        setStarting(false)
        const track = stream.getVideoTracks()[0]
        const trackSettings = track?.getSettings?.()
        setMirror(trackSettings?.facingMode ? trackSettings.facingMode !== 'environment' : true)
        if (trackSettings?.deviceId) setActiveDeviceId(trackSettings.deviceId)
        navigator.mediaDevices.enumerateDevices?.()
          .then((list) => { if (!stopped) setDevices(list.filter((device) => device.kind === 'videoinput')) })
          .catch(() => {})
      })
      .catch((cause) => { if (!stopped) { setStarting(false); setError(cameraErrorMessage(cause)) } })
    return () => {
      stopped = true
      stopStream(streamRef.current)
      streamRef.current = null
    }
  }, [video, deviceId, attempt, review])

  // Live clock for the timestamp line.
  useEffect(() => {
    const timer = setInterval(() => setNow(new Date()), 1000)
    return () => clearInterval(timer)
  }, [])

  // GPS + address, refreshed in the background.
  useEffect(() => {
    let cancelled = false
    const unsubscribe = tracker.onChange(setAddress)
    const refresh = () => {
      void getFix().then((next) => {
        if (cancelled || !next) return
        fixRef.current = next
        setFix(next)
        tracker.update(next)
      })
    }
    refresh()
    const timer = setInterval(refresh, FIX_REFRESH_MS)
    return () => { cancelled = true; clearInterval(timer); unsubscribe() }
  }, [tracker])

  // Track the displayed box so the overlay and the capture crop use its exact size.
  useEffect(() => {
    if (!stage) return
    const measure = () => {
      const rect = stage.getBoundingClientRect()
      setBox((prev) => (prev.width === rect.width && prev.height === rect.height ? prev : { width: rect.width, height: rect.height }))
    }
    measure()
    if (typeof ResizeObserver === 'undefined') {
      window.addEventListener('resize', measure)
      return () => window.removeEventListener('resize', measure)
    }
    const observer = new ResizeObserver(measure)
    observer.observe(stage)
    return () => observer.disconnect()
  }, [stage])

  const lines = watermarkLines(now, settings, address || tracker.current(), fix ? formatCoordinates(fix) : null)

  // Live overlay, drawn in CSS px on a devicePixelRatio-sized canvas.
  useEffect(() => {
    if (!overlay || box.width === 0 || box.height === 0) return
    const dpr = window.devicePixelRatio || 1
    overlay.width = Math.round(box.width * dpr)
    overlay.height = Math.round(box.height * dpr)
    const ctx = overlay.getContext('2d')
    if (!ctx) return
    ctx.setTransform(1, 0, 0, 1, 0, 0)
    ctx.clearRect(0, 0, overlay.width, overlay.height)
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    layoutRef.current = paintWatermark(ctx, box.width, box.height, settings, lines, logo)
    // `lines` changes every second through `now`; its text is the dependency that matters.
  }, [overlay, box, settings, lines.join('\n'), logo]) // eslint-disable-line react-hooks/exhaustive-deps

  const updateSettings = useCallback((next: WatermarkSettings, persist: boolean) => {
    settingsRef.current = next
    setSettings(next)
    if (persist) saveWatermarkSettings(next)
  }, [])

  const gestures = useMemo(() => createWatermarkGestures({
    box: () => box,
    rects: () => ({ logo: layoutRef.current?.logo ?? null, timestamp: layoutRef.current?.timestamp ?? null }),
    placement: (key: ElementKey) => (key === 'logo' ? settingsRef.current.logoPlacement : settingsRef.current.timestampPlacement),
    setPlacement: (key: ElementKey, placement: ElementPlacement) => {
      const current = settingsRef.current
      updateSettings(key === 'logo' ? { ...current, logoPlacement: placement } : { ...current, timestampPlacement: placement }, false)
    },
    commit: () => { saveWatermarkSettings(settingsRef.current) },
  }), [box, updateSettings])

  useEffect(() => () => gestures.flush(), [gestures])

  // Wheel scaling needs a non-passive listener to stop the page from scrolling.
  useEffect(() => {
    if (!overlay) return
    const onWheel = (event: WheelEvent) => {
      const rect = overlay.getBoundingClientRect()
      if (gestures.wheel(event.clientX - rect.left, event.clientY - rect.top, event.deltaY)) event.preventDefault()
    }
    overlay.addEventListener('wheel', onWheel, { passive: false })
    return () => overlay.removeEventListener('wheel', onWheel)
  }, [overlay, gestures])

  const pointerPoint = (event: React.PointerEvent<HTMLCanvasElement>) => {
    const rect = event.currentTarget.getBoundingClientRect()
    return { x: event.clientX - rect.left, y: event.clientY - rect.top }
  }

  useEffect(() => () => { if (review) URL.revokeObjectURL(review.url) }, [review])

  const close = () => {
    stopStream(streamRef.current)
    streamRef.current = null
    onClose()
  }

  const toggle = (key: 'showLogo' | 'showTimestamp') => updateSettings({ ...settings, [key]: !settings[key] }, true)

  const switchCamera = () => {
    if (devices.length < 2) return
    const currentId = deviceId ?? activeDeviceId
    const index = devices.findIndex((device) => device.deviceId === currentId)
    setDeviceId(devices[(index + 1) % devices.length].deviceId)
  }

  const shoot = async () => {
    if (!video || busy) return
    setBusy(true)
    try {
      const capturedAt = new Date()
      const shotFix = fixRef.current
      const current = settingsRef.current
      const sourceW = video.videoWidth
      const sourceH = video.videoHeight
      // Crop the frame to what the user saw; without a measured box keep the whole frame.
      const crop: Rect = box.width > 0 && box.height > 0
        ? coverCrop(sourceW, sourceH, box.width, box.height)
        : { x: 0, y: 0, width: sourceW, height: sourceH }
      const canvas = document.createElement('canvas')
      canvas.width = Math.round(crop.width)
      canvas.height = Math.round(crop.height)
      const ctx = canvas.getContext('2d')
      if (!ctx) throw new Error('Canvas tidak didukung')
      if (mirror) {
        ctx.translate(canvas.width, 0)
        ctx.scale(-1, 1)
      }
      ctx.drawImage(video, crop.x, crop.y, crop.width, crop.height, 0, 0, canvas.width, canvas.height)
      ctx.setTransform(1, 0, 0, 1, 0, 0)
      const shotLines = watermarkLines(capturedAt, current, tracker.current(), shotFix ? formatCoordinates(shotFix) : null)
      paintWatermark(ctx, canvas.width, canvas.height, current, shotLines, logo)
      const composed = await canvasToBlob(canvas, 'image/jpeg', COMPOSE_JPEG_QUALITY)
      const file = await resizeImage(composed, current.photoQuality)
      setReview({ file, url: URL.createObjectURL(file), capturedAt, fix: shotFix })
    } catch {
      setError('Foto gagal diproses. Coba ambil ulang.')
    } finally {
      setBusy(false)
    }
  }

  const retake = () => setReview(null)

  const usePhoto = () => {
    if (!review) return
    onCapture({
      file: review.file,
      capturedAt: review.capturedAt,
      latitude: review.fix?.latitude ?? null,
      longitude: review.fix?.longitude ?? null,
    })
  }

  const retry = () => { setError(null); setAttempt((value) => value + 1) }

  return <Dialog open onOpenChange={(nextOpen) => { if (!nextOpen) close() }}>
    <DialogContent
      aria-label={title}
      showCloseButton={false}
      className="flex h-dvh max-h-dvh w-screen max-w-none flex-col gap-0 rounded-none bg-black p-0 text-white ring-0 sm:max-w-none"
    >
      <DialogTitle className="sr-only">{title}</DialogTitle>
      <div className="flex items-center justify-between gap-2 p-2">
        <Button type="button" variant="ghost" size="icon" className="text-white hover:bg-white/10" aria-label="Tutup kamera" onClick={close}><X aria-hidden="true" /></Button>
        <span className="truncate text-sm font-medium">{title}</span>
        <Link to="/pengaturan-watermark" aria-label="Pengaturan watermark" onClick={close} className={buttonVariants({ variant: 'ghost', size: 'icon', className: 'text-white hover:bg-white/10' })}>
          <Settings aria-hidden="true" />
        </Link>
      </div>

      {review ? <div className="flex min-h-0 flex-1 flex-col">
        <div className="flex min-h-0 flex-1 items-center justify-center">
          <img src={review.url} alt="Pratinjau foto" className="max-h-full max-w-full object-contain" />
        </div>
        <div className="flex justify-center gap-3 p-4">
          <Button type="button" variant="outline" className="text-foreground" onClick={retake}><RotateCcw aria-hidden="true" />Ambil Ulang</Button>
          <Button type="button" onClick={usePhoto}><Check aria-hidden="true" />Gunakan Foto</Button>
        </div>
      </div> : error ? <div className="flex flex-1 flex-col items-center justify-center gap-3 p-6 text-center">
        <p className="text-sm text-red-300" role="alert">{error}</p>
        <Button type="button" variant="outline" className="text-foreground" onClick={retry}><RefreshCw aria-hidden="true" />Coba lagi</Button>
      </div> : <>
        <div ref={setStage} className="relative min-h-0 flex-1 overflow-hidden">
          <video ref={setVideo} className="absolute inset-0 size-full object-cover" style={mirror ? { transform: 'scaleX(-1)' } : undefined} autoPlay muted playsInline />
          <canvas
            ref={setOverlay}
            className="absolute inset-0 size-full touch-none"
            onPointerDown={(event) => {
              const p = pointerPoint(event)
              if (gestures.down(event.pointerId, p.x, p.y)) event.currentTarget.setPointerCapture?.(event.pointerId)
            }}
            onPointerMove={(event) => { const p = pointerPoint(event); gestures.move(event.pointerId, p.x, p.y) }}
            onPointerUp={(event) => gestures.up(event.pointerId)}
            onPointerCancel={(event) => gestures.up(event.pointerId)}
          />
          {starting && <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 bg-black/70">
            <LoaderCircle className="size-6 animate-spin" aria-hidden="true" />
            <span className="text-xs font-medium">Menyiapkan kamera...</span>
          </div>}
        </div>
        <div className="flex items-center justify-between gap-2 p-4">
          <div className="flex gap-2">
            {([['showLogo', 'Logo'], ['showTimestamp', 'Waktu']] as const).map(([key, label]) => (
              <Button
                key={key}
                type="button"
                size="sm"
                variant="outline"
                aria-pressed={settings[key]}
                className={cn('text-foreground', !settings[key] && 'opacity-60 line-through')}
                onClick={() => toggle(key)}
              >{label}</Button>
            ))}
          </div>
          <Button
            type="button"
            size="icon-lg"
            className="size-16 rounded-full border-4 border-white bg-white/20 hover:bg-white/30"
            aria-label="Ambil foto"
            disabled={starting || busy}
            onClick={() => { void shoot() }}
          >
            {busy ? <LoaderCircle className="size-6 animate-spin" aria-hidden="true" /> : <Camera className="size-6" aria-hidden="true" />}
          </Button>
          <div className="flex w-30 justify-end">
            {devices.length > 1 && <Button type="button" variant="ghost" size="icon" className="text-white hover:bg-white/10" aria-label="Ganti kamera" onClick={switchCamera}>
              <SwitchCamera aria-hidden="true" />
            </Button>}
          </div>
        </div>
      </>}
    </DialogContent>
  </Dialog>
}
