import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  DEFAULT_WATERMARK_SETTINGS,
  PHOTO_QUALITY_MAX_SIDE,
  WATERMARK_COLORS,
  loadWatermarkSettings,
  resetWatermarkSettings,
  saveWatermarkSettings,
} from './watermarkSettings'

const KEY = 'konkit.watermark.v1'

describe('watermarkSettings', () => {
  beforeEach(() => localStorage.clear())
  afterEach(() => vi.restoreAllMocks())

  it('has the documented defaults', () => {
    expect(DEFAULT_WATERMARK_SETTINGS).toEqual({
      showLogo: true,
      showTimestamp: true,
      timestampSide: 'right',
      textColor: '#FFEB3B',
      accentColor: '#FF6B35',
      textScale: 1,
      logoScale: 1,
      customLogo: null,
      tenderLogos: {},
      logoPlacement: null,
      timestampPlacement: null,
      backdate: null,
      photoQuality: 'standar',
    })
    expect(WATERMARK_COLORS).toEqual([
      { name: 'Kuning', hex: '#FFEB3B' },
      { name: 'Putih', hex: '#FFFFFF' },
      { name: 'Oranye', hex: '#FF6B35' },
      { name: 'Hijau', hex: '#4ADE80' },
      { name: 'Biru', hex: '#60A5FA' },
    ])
    expect(PHOTO_QUALITY_MAX_SIDE).toEqual({ hemat: 1280, standar: 1920, tinggi: 2560, asli: null })
  })

  it('returns defaults when nothing is stored', () => {
    expect(loadWatermarkSettings()).toEqual(DEFAULT_WATERMARK_SETTINGS)
  })

  it('merges partial stored JSON over defaults and ignores invalid fields', () => {
    localStorage.setItem(KEY, JSON.stringify({ showLogo: false, textScale: 7, textColor: 'red', logoScale: 0.7, photoQuality: 'tinggi', timestampSide: 'up' }))
    const s = loadWatermarkSettings()
    expect(s.showLogo).toBe(false)
    expect(s.textScale).toBe(1)
    expect(s.textColor).toBe('#FFEB3B')
    expect(s.logoScale).toBe(0.7)
    expect(s.photoQuality).toBe('tinggi')
    expect(s.timestampSide).toBe('right')
  })

  it('clamps logoScale and validates placement and backdate', () => {
    localStorage.setItem(KEY, JSON.stringify({ logoScale: 5, logoPlacement: { x: 0.2, y: 0.3, scale: 1 }, timestampPlacement: { x: 'a' }, backdate: '2026-01-02' }))
    const s = loadWatermarkSettings()
    expect(s.logoScale).toBe(1)
    expect(s.logoPlacement).toEqual({ x: 0.2, y: 0.3, scale: 1 })
    expect(s.timestampPlacement).toBeNull()
    expect(s.backdate).toBe('2026-01-02')
  })

  it('returns defaults when storage throws or JSON is corrupt', () => {
    localStorage.setItem(KEY, '{nope')
    expect(loadWatermarkSettings()).toEqual(DEFAULT_WATERMARK_SETTINGS)
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('denied')
    })
    expect(loadWatermarkSettings()).toEqual(DEFAULT_WATERMARK_SETTINGS)
  })

  it('round-trips through save/load', () => {
    const next = { ...DEFAULT_WATERMARK_SETTINGS, showTimestamp: false, textScale: 1.2 as const }
    expect(saveWatermarkSettings(next)).toBe(true)
    expect(loadWatermarkSettings()).toEqual(next)
  })

  it('save returns false instead of throwing when storage refuses', () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('full', 'QuotaExceededError')
    })
    expect(saveWatermarkSettings(DEFAULT_WATERMARK_SETTINGS)).toBe(false)
  })

  it('reset clears storage and returns defaults', () => {
    saveWatermarkSettings({ ...DEFAULT_WATERMARK_SETTINGS, showLogo: false })
    expect(resetWatermarkSettings()).toEqual(DEFAULT_WATERMARK_SETTINGS)
    expect(loadWatermarkSettings()).toEqual(DEFAULT_WATERMARK_SETTINGS)
  })
})
