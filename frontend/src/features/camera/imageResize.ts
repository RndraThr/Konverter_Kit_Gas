import { PHOTO_QUALITY_MAX_SIDE, type PhotoQuality } from './watermarkSettings'

const LOGO_MAX_SIDE = 512

function fitSize(width: number, height: number, maxSide: number | null): { width: number; height: number } {
  const longest = Math.max(width, height)
  if (maxSide === null || longest <= maxSide) return { width, height }
  const ratio = maxSide / longest
  return { width: Math.round(width * ratio), height: Math.round(height * ratio) }
}

async function renderToBlob(source: Blob, maxSide: number | null, type: string, quality?: number): Promise<Blob> {
  const bitmap = await createImageBitmap(source)
  try {
    const { width, height } = fitSize(bitmap.width, bitmap.height, maxSide)
    const canvas = document.createElement('canvas')
    canvas.width = width
    canvas.height = height
    const ctx = canvas.getContext('2d')
    if (!ctx) throw new Error('Canvas tidak didukung')
    ctx.drawImage(bitmap, 0, 0, width, height)
    return await new Promise<Blob>((resolve, reject) => {
      canvas.toBlob(
        (blob) => (blob ? resolve(blob) : reject(new Error('Gagal mengompres gambar'))),
        type,
        quality,
      )
    })
  } finally {
    bitmap.close()
  }
}

/** Re-encodes to JPEG `photo.jpg`, longest side capped by the quality preset (never upscaled). */
export async function resizeImage(source: Blob, quality: PhotoQuality): Promise<File> {
  const blob = await renderToBlob(source, PHOTO_QUALITY_MAX_SIDE[quality], 'image/jpeg', quality === 'asli' ? 0.95 : 0.92)
  return new File([blob], 'photo.jpg', { type: 'image/jpeg' })
}

/** Downscales a picked logo to at most 512px and returns it as a PNG data URL. */
export async function downscaleLogo(file: Blob): Promise<string> {
  const blob = await renderToBlob(file, LOGO_MAX_SIDE, 'image/png')
  return await new Promise<string>((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result))
    reader.onerror = () => reject(reader.error ?? new Error('Gagal membaca logo'))
    reader.readAsDataURL(blob)
  })
}
