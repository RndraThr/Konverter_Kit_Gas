export type WatermarkColor = { name: 'Kuning' | 'Putih' | 'Oranye' | 'Hijau' | 'Biru'; hex: string }

export const WATERMARK_COLORS: WatermarkColor[] = [
  { name: 'Kuning', hex: '#FFEB3B' },
  { name: 'Putih', hex: '#FFFFFF' },
  { name: 'Oranye', hex: '#FF6B35' },
  { name: 'Hijau', hex: '#4ADE80' },
  { name: 'Biru', hex: '#60A5FA' },
]

export type PhotoQuality = 'hemat' | 'standar' | 'tinggi' | 'asli'

/** Longest side in px; null = original size (asli). */
export const PHOTO_QUALITY_MAX_SIDE: Record<PhotoQuality, number | null> = {
  hemat: 1280,
  standar: 1920,
  tinggi: 2560,
  asli: null,
}

/** x/y = centre as a fraction (0-1) of the photo; null = default position. */
export type ElementPlacement = { x: number; y: number; scale: number } | null

export type WatermarkSettings = {
  showLogo: boolean
  showTimestamp: boolean
  timestampSide: 'left' | 'right'
  textColor: string
  accentColor: string
  textScale: 0.85 | 1 | 1.2
  logoScale: number
  /** PNG data URL */
  customLogo: string | null
  logoPlacement: ElementPlacement
  timestampPlacement: ElementPlacement
  /** YYYY-MM-DD */
  backdate: string | null
  photoQuality: PhotoQuality
}

export const MIN_LOGO_SCALE = 0.5
export const MAX_LOGO_SCALE = 1.0

export const DEFAULT_WATERMARK_SETTINGS: WatermarkSettings = {
  showLogo: true,
  showTimestamp: true,
  timestampSide: 'right',
  textColor: '#FFEB3B',
  accentColor: '#FF6B35',
  textScale: 1,
  logoScale: 1,
  customLogo: null,
  logoPlacement: null,
  timestampPlacement: null,
  backdate: null,
  photoQuality: 'standar',
}

const STORAGE_KEY = 'konkit.watermark.v1'
const HEX = /^#[0-9a-fA-F]{6}$/
const DATE = /^\d{4}-\d{2}-\d{2}$/

function isFraction(v: unknown): v is number {
  return typeof v === 'number' && Number.isFinite(v) && v >= 0 && v <= 1
}

function parsePlacement(v: unknown): ElementPlacement {
  if (!v || typeof v !== 'object') return null
  const { x, y, scale } = v as Record<string, unknown>
  if (!isFraction(x) || !isFraction(y)) return null
  if (typeof scale !== 'number' || !Number.isFinite(scale) || scale <= 0) return null
  return { x, y, scale }
}

function sanitize(raw: unknown): WatermarkSettings {
  const d = DEFAULT_WATERMARK_SETTINGS
  if (!raw || typeof raw !== 'object') return { ...d }
  const r = raw as Record<string, unknown>
  return {
    showLogo: typeof r.showLogo === 'boolean' ? r.showLogo : d.showLogo,
    showTimestamp: typeof r.showTimestamp === 'boolean' ? r.showTimestamp : d.showTimestamp,
    timestampSide: r.timestampSide === 'left' || r.timestampSide === 'right' ? r.timestampSide : d.timestampSide,
    textColor: typeof r.textColor === 'string' && HEX.test(r.textColor) ? r.textColor : d.textColor,
    accentColor: typeof r.accentColor === 'string' && HEX.test(r.accentColor) ? r.accentColor : d.accentColor,
    textScale: r.textScale === 0.85 || r.textScale === 1 || r.textScale === 1.2 ? r.textScale : d.textScale,
    logoScale:
      typeof r.logoScale === 'number' && Number.isFinite(r.logoScale)
        ? Math.min(MAX_LOGO_SCALE, Math.max(MIN_LOGO_SCALE, r.logoScale))
        : d.logoScale,
    customLogo: typeof r.customLogo === 'string' && r.customLogo.startsWith('data:image/') ? r.customLogo : null,
    logoPlacement: parsePlacement(r.logoPlacement),
    timestampPlacement: parsePlacement(r.timestampPlacement),
    backdate: typeof r.backdate === 'string' && DATE.test(r.backdate) ? r.backdate : null,
    photoQuality:
      typeof r.photoQuality === 'string' && Object.hasOwn(PHOTO_QUALITY_MAX_SIDE, r.photoQuality)
        ? (r.photoQuality as PhotoQuality)
        : d.photoQuality,
  }
}

export function loadWatermarkSettings(): WatermarkSettings {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (!stored) return { ...DEFAULT_WATERMARK_SETTINGS }
    return sanitize(JSON.parse(stored))
  } catch {
    return { ...DEFAULT_WATERMARK_SETTINGS }
  }
}

/** Returns false when the browser refuses to store (quota, private mode). */
export function saveWatermarkSettings(s: WatermarkSettings): boolean {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(s))
    return true
  } catch {
    return false
  }
}

export function resetWatermarkSettings(): WatermarkSettings {
  try {
    localStorage.removeItem(STORAGE_KEY)
  } catch {
    // storage unavailable; defaults still apply in memory
  }
  return { ...DEFAULT_WATERMARK_SETTINGS }
}
