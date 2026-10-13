// Menggambar sebagian halaman PDF (dalam mm) ke kanvas memakai pdf.js; dipakai
// editor tanda tangan sebagai latar tabel tanda tangan yang persis seperti cetak.

export type RegionMM = { left: number; top: number; width: number; height: number };

const pointsPerMM = 72 / 25.4;

export function base64ToBytes(value: string) {
  const binary = atob(value);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  return bytes;
}

/**
 * Merender region halaman pageNumber ke canvas dengan skala pxPerMM (CSS px
 * per mm). Kanvas memakai resolusi perangkat agar teks tetap tajam.
 */
export async function renderPdfRegion(canvas: HTMLCanvasElement, data: Uint8Array, pageNumber: number, region: RegionMM, pxPerMM: number, signal?: AbortSignal) {
  const [pdfjs, worker] = await Promise.all([import('pdfjs-dist'), import('pdfjs-dist/build/pdf.worker.min.mjs?url')]);
  pdfjs.GlobalWorkerOptions.workerSrc = worker.default;
  // pdf.js memindahkan buffer ke worker; salin agar data asli tetap bisa dipakai ulang.
  const task = pdfjs.getDocument({ data: data.slice() });
  try {
    const pdf = await task.promise;
    if (signal?.aborted) return;
    const page = await pdf.getPage(pageNumber);
    const ratio = Math.min(window.devicePixelRatio || 1, 2);
    const scale = (pxPerMM * ratio) / pointsPerMM;
    const viewport = page.getViewport({ scale, offsetX: -region.left * pxPerMM * ratio, offsetY: -region.top * pxPerMM * ratio });
    canvas.width = Math.max(1, Math.round(region.width * pxPerMM * ratio));
    canvas.height = Math.max(1, Math.round(region.height * pxPerMM * ratio));
    await page.render({ canvas, viewport }).promise;
  } finally {
    void task.destroy();
  }
}
