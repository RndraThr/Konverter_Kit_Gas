import { useEffect, useState, type ChangeEvent, type ReactNode } from 'react'
import { ImagePlus, RotateCcw } from 'lucide-react'
import { PageHeader } from '@/components/PageHeader'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { Button, buttonVariants } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'
import { downscaleLogo } from './imageResize'
import { useWatermarkLogo } from './useWatermarkLogo'
import { watermarkLines } from './watermarkLayout'
import { paintWatermark } from './watermarkRender'
import {
  loadWatermarkSettings,
  MAX_LOGO_SCALE,
  MIN_LOGO_SCALE,
  resetWatermarkSettings,
  saveWatermarkSettings,
  WATERMARK_COLORS,
  type PhotoQuality,
  type WatermarkSettings,
} from './watermarkSettings'

// Portrait 3:4 sample, the shape of a typical phone photo.
const PREVIEW_W = 390
const PREVIEW_H = 520
const SAMPLE_ADDRESS = 'Jl. Contoh No. 1, Kelurahan, Kecamatan, Kabupaten'
const SAMPLE_COORDS = 'Lat: -6.20000, Lng: 106.81667'

const TEXT_SIZES: { value: WatermarkSettings['textScale']; label: string }[] = [
  { value: 0.85, label: 'Kecil' },
  { value: 1, label: 'Sedang' },
  { value: 1.2, label: 'Besar' },
]

const QUALITIES: { value: PhotoQuality; label: string }[] = [
  { value: 'hemat', label: 'Hemat 1280 px' },
  { value: 'standar', label: 'Standar 1920 px' },
  { value: 'tinggi', label: 'Tinggi 2560 px' },
  { value: 'asli', label: 'Asli' },
]

function OptionGroup<T extends string | number>({ label, options, value, onChange }: {
  label: string
  options: { value: T; label: string }[]
  value: T
  onChange: (value: T) => void
}) {
  return <div className="grid gap-1.5">
    <span className="text-sm font-medium">{label}</span>
    <div role="group" aria-label={label} className="flex flex-wrap gap-2">
      {options.map((option) => <Button
        key={String(option.value)}
        type="button"
        size="sm"
        variant={option.value === value ? 'default' : 'outline'}
        aria-pressed={option.value === value}
        onClick={() => onChange(option.value)}
      >{option.label}</Button>)}
    </div>
  </div>
}

function ColorGroup({ label, value, onChange }: { label: string; value: string; onChange: (hex: string) => void }) {
  return <div className="grid gap-1.5">
    <span className="text-sm font-medium">{label}</span>
    <div role="group" aria-label={label} className="flex flex-wrap gap-2">
      {WATERMARK_COLORS.map((color) => {
        const selected = color.hex.toLowerCase() === value.toLowerCase()
        return <button
          key={color.hex}
          type="button"
          aria-label={color.name}
          title={color.name}
          aria-pressed={selected}
          onClick={() => onChange(color.hex)}
          className={cn('size-8 rounded-full border-2 border-border outline-none focus-visible:ring-3 focus-visible:ring-ring/50', selected && 'ring-2 ring-primary ring-offset-2')}
          style={{ backgroundColor: color.hex }}
        />
      })}
    </div>
  </div>
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return <section className="grid gap-4">
    <h2 className="text-base font-semibold">{title}</h2>
    {children}
  </section>
}

function WatermarkPreview({ settings }: { settings: WatermarkSettings }) {
  const [canvas, setCanvas] = useState<HTMLCanvasElement | null>(null)
  const [now, setNow] = useState(() => new Date())
  const logo = useWatermarkLogo(undefined, settings.customLogo)

  useEffect(() => {
    const timer = setInterval(() => setNow(new Date()), 1000)
    return () => clearInterval(timer)
  }, [])

  useEffect(() => {
    const ctx = canvas?.getContext('2d')
    if (!canvas || !ctx) return
    const dpr = window.devicePixelRatio || 1
    canvas.width = PREVIEW_W * dpr
    canvas.height = PREVIEW_H * dpr
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    const sky = ctx.createLinearGradient(0, 0, 0, PREVIEW_H)
    sky.addColorStop(0, '#5b7a99')
    sky.addColorStop(0.6, '#8a9a7b')
    sky.addColorStop(1, '#4a5a3c')
    ctx.fillStyle = sky
    ctx.fillRect(0, 0, PREVIEW_W, PREVIEW_H)
    paintWatermark(ctx, PREVIEW_W, PREVIEW_H, settings, watermarkLines(now, settings, SAMPLE_ADDRESS, SAMPLE_COORDS), logo)
  }, [canvas, settings, now, logo])

  return <canvas ref={setCanvas} aria-label="Pratinjau watermark" role="img" className="aspect-3/4 w-full max-w-sm rounded-lg border bg-muted" />
}

/** Per-device watermark settings (stored in this browser), with a live preview. */
export function WatermarkSettingsPage() {
  const [settings, setSettings] = useState(loadWatermarkSettings)
  const [saveFailed, setSaveFailed] = useState(false)
  const [logoError, setLogoError] = useState<string | null>(null)
  const [confirmReset, setConfirmReset] = useState(false)

  const update = (patch: Partial<WatermarkSettings>) => {
    const next = { ...settings, ...patch }
    setSettings(next)
    setSaveFailed(!saveWatermarkSettings(next))
  }

  const pickLogo = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) return
    setLogoError(null)
    try {
      update({ customLogo: await downscaleLogo(file) })
    } catch {
      setLogoError('Gambar tidak dapat dipakai sebagai logo. Pilih gambar lain.')
    }
  }

  const resetAll = () => {
    setSettings(resetWatermarkSettings())
    setSaveFailed(false)
    setConfirmReset(false)
  }

  return <div className="mx-auto max-w-6xl space-y-6">
    <PageHeader title="Kamera watermark" description="Atur tampilan watermark pada foto dokumentasi. Pengaturan disimpan di perangkat ini." />
    {saveFailed ? <Alert variant="destructive" role="alert"><AlertDescription>Pengaturan tidak dapat disimpan di perangkat ini.</AlertDescription></Alert> : null}

    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_24rem]">
      <Card className="shadow-none">
        <CardContent className="grid gap-8">
          <Section title="Tampilan">
            <label className="flex items-center gap-3 text-sm">
              <Checkbox checked={settings.showLogo} onCheckedChange={(checked) => update({ showLogo: checked === true })} />
              Tampilkan logo
            </label>
            <label className="flex items-center gap-3 text-sm">
              <Checkbox checked={settings.showTimestamp} onCheckedChange={(checked) => update({ showTimestamp: checked === true })} />
              Tampilkan waktu &amp; lokasi
            </label>
            <OptionGroup label="Posisi waktu" value={settings.timestampSide} onChange={(timestampSide) => update({ timestampSide })} options={[{ value: 'left', label: 'Kiri' }, { value: 'right', label: 'Kanan' }]} />
            <ColorGroup label="Warna teks" value={settings.textColor} onChange={(textColor) => update({ textColor })} />
            <ColorGroup label="Warna aksen" value={settings.accentColor} onChange={(accentColor) => update({ accentColor })} />
            <OptionGroup label="Ukuran teks" value={settings.textScale} onChange={(textScale) => update({ textScale })} options={TEXT_SIZES} />
            <div className="grid gap-1.5">
              <Label htmlFor="watermark-logo-scale">Ukuran logo</Label>
              <div className="flex items-center gap-3">
                <input
                  id="watermark-logo-scale"
                  type="range"
                  min={MIN_LOGO_SCALE}
                  max={MAX_LOGO_SCALE}
                  step={0.05}
                  value={settings.logoScale}
                  onChange={(event) => update({ logoScale: Number(event.target.value) })}
                  className="w-full max-w-xs accent-primary"
                />
                <span className="w-12 text-sm tabular-nums text-muted-foreground">{Math.round(settings.logoScale * 100)}%</span>
              </div>
            </div>
            <div className="grid gap-1.5">
              <span className="text-sm font-medium">Logo kustom</span>
              <div className="flex flex-wrap gap-2">
                <label className={buttonVariants({ variant: 'outline', size: 'sm', className: 'cursor-pointer' })}>
                  <ImagePlus aria-hidden="true" />Pilih gambar
                  <input type="file" accept="image/*" className="sr-only" onChange={(event) => { void pickLogo(event) }} />
                </label>
                <Button type="button" size="sm" variant="outline" disabled={!settings.customLogo} onClick={() => update({ customLogo: null })}>Kembalikan logo bawaan</Button>
              </div>
              <p className="text-xs text-muted-foreground">{settings.customLogo ? 'Memakai logo kustom.' : 'Memakai logo tender program, atau logo bawaan bila belum ada.'}</p>
              {logoError ? <p className="text-xs text-destructive" role="alert">{logoError}</p> : null}
            </div>
          </Section>

          <Section title="Tanggal">
            <div className="grid gap-1.5">
              <Label htmlFor="watermark-backdate">Backdate</Label>
              <div className="flex flex-wrap gap-2">
                <Input id="watermark-backdate" type="date" className="w-auto" value={settings.backdate ?? ''} onChange={(event) => update({ backdate: event.target.value || null })} />
                <Button type="button" size="sm" variant="outline" className="h-auto" disabled={!settings.backdate} onClick={() => update({ backdate: null })}>Hapus backdate</Button>
              </div>
              <p className="text-xs text-muted-foreground">Hanya tanggal di watermark yang berubah; waktu asli tetap tercatat.</p>
            </div>
          </Section>

          <Section title="Foto">
            <OptionGroup label="Kualitas foto" value={settings.photoQuality} onChange={(photoQuality) => update({ photoQuality })} options={QUALITIES} />
          </Section>

          <div className="flex flex-wrap gap-2 border-t pt-5">
            <Button type="button" variant="outline" onClick={() => update({ logoPlacement: null, timestampPlacement: null })}>Kembalikan posisi</Button>
            <Button type="button" variant="outline" onClick={() => setConfirmReset(true)}><RotateCcw aria-hidden="true" />Bawaan</Button>
          </div>
        </CardContent>
      </Card>

      <div className="grid content-start justify-items-center gap-2">
        <WatermarkPreview settings={settings} />
        <p className="text-xs text-muted-foreground">Geser dan ubah ukuran logo atau waktu langsung di layar kamera.</p>
      </div>
    </div>

    <AlertDialog open={confirmReset} onOpenChange={setConfirmReset}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Kembalikan pengaturan bawaan?</AlertDialogTitle>
          <AlertDialogDescription>Semua pengaturan watermark di perangkat ini, termasuk logo kustom, posisi, dan backdate, akan dikembalikan ke bawaan.</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter><AlertDialogCancel>Batal</AlertDialogCancel><AlertDialogAction onClick={resetAll}>Kembalikan</AlertDialogAction></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
}
