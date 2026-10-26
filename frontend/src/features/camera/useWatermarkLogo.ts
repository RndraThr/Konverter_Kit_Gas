import { useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import ergasUrl from '@/assets/watermark/logo-ergas.png'
import ksmUrl from '@/assets/watermark/logo-ksm.png'
import { brandingLogosQueryKey, fetchBrandingLogos } from '../berita-acara/brandingLogos'
import { loadWatermarkSettings } from './watermarkSettings'

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

/**
 * Watermark logo in priority order: the officer's custom logo, the first visible
 * tender logo of the program, or the built-in KSM + ERGAS banner (also used when
 * a tender logo fails to load). Null while loading.
 *
 * `customLogo` defaults to the stored setting; pass it to follow unsaved edits.
 */
export function useWatermarkLogo(programId?: string, customLogo?: string | null): HTMLImageElement | null {
  const [storedCustom] = useState(() => loadWatermarkSettings().customLogo)
  const custom = customLogo === undefined ? storedCustom : customLogo
  const tender = useQuery({
    queryKey: brandingLogosQueryKey(programId ?? ''),
    queryFn: () => fetchBrandingLogos(programId!),
    enabled: Boolean(programId) && !custom,
    staleTime: 5 * 60 * 1000,
  })
  const tenderPending = Boolean(programId) && !custom && tender.isPending
  const tenderUrl = custom ? null : (tender.data?.find((logo) => logo.is_visible)?.content_url ?? null)
  const source = custom ?? tenderUrl

  const [logo, setLogo] = useState<{ key: string; img: HTMLImageElement } | null>(null)
  const key = tenderPending ? '' : (source ?? 'banner')

  useEffect(() => {
    if (!key) return
    let cancelled = false
    const load = source ? loadImage(source).catch(() => builtInBanner()) : builtInBanner()
    load.then((img) => { if (!cancelled) setLogo({ key, img }) }).catch(() => { /* no logo at all */ })
    return () => { cancelled = true }
  }, [key, source])

  return logo && logo.key === key ? logo.img : null
}
