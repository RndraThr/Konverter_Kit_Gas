import { resizeImage } from './imageResize'
import { loadWatermarkSettings } from './watermarkSettings'

/** Shrinks a gallery photo to the watermark settings' quality, keeping its name and timestamp. */
export async function shrinkGalleryImage(file: File): Promise<File> {
  try {
    const resized = await resizeImage(file, loadWatermarkSettings().photoQuality)
    return new File([resized], `${file.name.replace(/\.[^.]+$/, '') || 'foto'}.jpg`, { type: 'image/jpeg', lastModified: file.lastModified })
  } catch {
    return file // the browser cannot decode it; upload the original and let the size check decide
  }
}
