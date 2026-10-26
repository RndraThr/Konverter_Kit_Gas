import { useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import ergasUrl from '@/assets/watermark/logo-ergas.png'
import ksmUrl from '@/assets/watermark/logo-ksm.png'
import { brandingLogosQueryKey, fetchBrandingLogos } from '../berita-acara/brandingLogos'
import type { BaLogo } from '../berita-acara/types'
import { BUILTIN_LOGO_ID, loadWatermarkSettings } from './watermarkSettings'

// The built-in banner matches the mobile app: KSM left, ERGAS right on a
// 4850:876 strip, 14 / 6 units of padding on a 382-unit wide banner.
const BANNER_WIDTH = 1940
const BANNER_HEIGHT = Math.round((BANNER_WIDTH * 876) / 4850)
const BANNER_PAD_X = (14 / 382) * BANNER_WIDTH
const BANNER_PAD_Y = (6 / 382) * BANNER_WIDTH

function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image()
    img.onload = () => resolve(img)
    img.onerror = () => reject(new Error(`Gagal memuat logo: ${src}`))
    img.src = src
  })
}

async function composeBanner(): Promise<HTMLImageElement> {
  const [ksm, ergas] = await Promise.all([loadImage(ksmUrl), loadImage(ergasUrl)])
  const canvas = document.createElement('canvas')
  canvas.width = BANNER_WIDTH
  canvas.height = BANNER_HEIGHT
  const ctx = canvas.getContext('2d')
  if (!ctx) throw new Error('Canvas tidak didukung')
  const boxH = BANNER_HEIGHT - 2 * BANNER_PAD_Y
  const boxW = BANNER_WIDTH / 2 - BANNER_PAD_X
  const fit = (img: HTMLImageElement) => {
    const k = Math.min(boxW / img.naturalWidth, boxH / img.naturalHeight)
    return { w: img.naturalWidth * k, h: img.naturalHeight * k }
  }
  const a = fit(ksm)
  ctx.drawImage(ksm, BANNER_PAD_X, (BANNER_HEIGHT - a.h) / 2, a.w, a.h)
  const b = fit(ergas)
  ctx.drawImage(ergas, BANNER_WIDTH - BANNER_PAD_X - b.w, (BANNER_HEIGHT - b.h) / 2, b.w, b.h)
  return loadImage(canvas.toDataURL('image/png'))
}

let bannerPromise: Promise<HTMLImageElement> | null = null

function builtInBanner(): Promise<HTMLImageElement> {
  bannerPromise ??= composeBanner().catch((error) => {
    bannerPromise = null
    throw error
  })
  return bannerPromise
}

// Several chosen logos are drawn side by side on one strip, each fitted to the strip height.
const STRIP_HEIGHT = 400
const STRIP_PAD = 24
const STRIP_GAP = 48

async function composeStrip(images: HTMLImageElement[]): Promise<HTMLImageElement> {
  const innerH = STRIP_HEIGHT - 2 * STRIP_PAD
  const widths = images.map((img) => (img.naturalWidth / img.naturalHeight) * innerH)
  const canvas = document.createElement('canvas')
  canvas.width = Math.round(widths.reduce((sum, w) => sum + w, 0) + STRIP_GAP * (images.length - 1) + 2 * STRIP_PAD)
  canvas.height = STRIP_HEIGHT
  const ctx = canvas.getContext('2d')
  if (!ctx) throw new Error('Canvas tidak didukung')
  let x = STRIP_PAD
  images.forEach((img, i) => {
    ctx.drawImage(img, x, STRIP_PAD, widths[i], innerH)
    x += widths[i] + STRIP_GAP
  })
  return loadImage(canvas.toDataURL('image/png'))
}

/** Tender logos uploaded for a program in Persiapan, in display order. */
export function useTenderLogos(programId?: string, enabled = true) {
  return useQuery({
    queryKey: brandingLogosQueryKey(programId ?? ''),
    queryFn: () => fetchBrandingLogos(programId!),
    enabled: Boolean(programId) && enabled,
    staleTime: 5 * 60 * 1000,
  })
}

/** The logos a program draws: the stored choice, else its first visible tender logo, else the built-in banner. */
export function effectiveLogoSelection(selection: string[] | undefined, tenderLogos: BaLogo[] | undefined): string[] {
  if (selection) return selection
  const first = tenderLogos?.find((logo) => logo.is_visible)
  return [first ? first.id : BUILTIN_LOGO_ID]
}

/**
 * Watermark logo in priority order: the officer's custom logo, the logos chosen for the program
 * (tender logos from Persiapan and/or the built-in KSM + ERGAS banner, composed side by side),
 * or by default the program's first visible tender logo. A tender logo that is gone or fails to load
 * is skipped; with nothing left the built-in banner is used. Null while loading.
 *
 * `customLogo` and `selection` default to the stored settings; pass them to follow unsaved edits.
 */
export function useWatermarkLogo(programId?: string, customLogo?: string | null, selection?: string[]): HTMLImageElement | null {
  const [stored] = useState(loadWatermarkSettings)
  const custom = customLogo === undefined ? stored.customLogo : customLogo
  const chosen = selection ?? (programId ? stored.tenderLogos[programId] : undefined)
  const tender = useTenderLogos(programId, !custom)
  const tenderPending = Boolean(programId) && !custom && tender.isPending

  let sources: string[] = []
  if (custom) sources = [custom]
  else if (!tenderPending) {
    sources = effectiveLogoSelection(chosen, tender.data).flatMap((id) => {
      if (id === BUILTIN_LOGO_ID) return [BUILTIN_LOGO_ID]
      const url = tender.data?.find((logo) => logo.id === id)?.content_url
      return url ? [url] : []
    })
  }
  const key = tenderPending ? '' : (sources.join('|') || BUILTIN_LOGO_ID)

  const [logo, setLogo] = useState<{ key: string; img: HTMLImageElement } | null>(null)

  useEffect(() => {
    if (!key) return
    let cancelled = false
    const urls = key.split('|')
    const load = async () => {
      const loaded = await Promise.all(urls.map((url) => (url === BUILTIN_LOGO_ID ? builtInBanner() : loadImage(url)).catch(() => null)))
      const images = loaded.filter((img): img is HTMLImageElement => img !== null)
      if (images.length === 0) return builtInBanner()
      return images.length === 1 ? images[0] : composeStrip(images)
    }
    load().then((img) => { if (!cancelled) setLogo({ key, img }) }).catch(() => { /* no logo at all */ })
    return () => { cancelled = true }
  }, [key])

  return logo && logo.key === key ? logo.img : null
}
