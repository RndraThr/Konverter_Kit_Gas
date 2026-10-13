// Tanda tangan basah: halaman tanda tangan BA dicetak, ditandatangani dan
// distempel basah, lalu hasil pindai/fotonya diunggah sebagai PNG.

export type WetSignatureStatus = {
  supported: boolean;
  enabled: boolean;
  code?: string;
  scan?: { uploaded_at: string; valid: boolean };
};

/** Kunci query memuat 'preview' agar ikut dimuat ulang setiap preview BA berubah. */
export const wetSignatureKey = (scheduleID: string, document: string, date: string) => ['bast', 'wet-signature', 'preview', scheduleID, document, date];

export function wetSignaturePath(scheduleID: string, document: string, date: string, suffix = '') {
  const query = new URLSearchParams({ schedule_id: scheduleID, document, date });
  return `/api/v1/bast/wet-signature${suffix ? `/${suffix}` : ''}?${query.toString()}`;
}

/**
 * Sisi terpanjang gambar pindaian: ~170 dpi di A4, cukup tajam untuk dicetak
 * dan foto berwarna tetap di bawah batas unggah 8 MB.
 */
const maxSide = 2000;

/**
 * Mengubah berkas pindaian (PDF halaman pertama, atau foto/gambar PNG/JPG)
 * menjadi PNG base64 berlatar putih. Gambar dipakai apa adanya, tanpa
 * pemotongan otomatis.
 */
export async function wetScanFileToPNGBase64(file: File): Promise<string> {
  const canvas = file.type === 'application/pdf' || file.name.toLowerCase().endsWith('.pdf')
    ? await renderPDFFirstPage(file)
    : await renderImage(file);
  const blob = await new Promise<Blob>((resolve, reject) => canvas.toBlob((value) => (value ? resolve(value) : reject(new Error('Gagal membuat PNG.'))), 'image/png'));
  const bytes = new Uint8Array(await blob.arrayBuffer());
  let binary = '';
  for (let i = 0; i < bytes.length; i += 0x8000) binary += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  return btoa(binary);
}

function whiteCanvas(width: number, height: number) {
  const canvas = document.createElement('canvas');
  canvas.width = Math.max(1, Math.round(width));
  canvas.height = Math.max(1, Math.round(height));
  const context = canvas.getContext('2d');
  if (!context) throw new Error('Browser tidak mendukung pengolahan gambar.');
  context.fillStyle = '#ffffff';
  context.fillRect(0, 0, canvas.width, canvas.height);
  return { canvas, context };
}

async function renderImage(file: File) {
  const url = URL.createObjectURL(file);
  try {
    const image = await new Promise<HTMLImageElement>((resolve, reject) => {
      const element = new Image();
      element.onload = () => resolve(element);
      element.onerror = () => reject(new Error('Berkas bukan gambar atau PDF yang valid.'));
      element.src = url;
    });
    const scale = Math.min(1, maxSide / Math.max(image.naturalWidth, image.naturalHeight));
    const { canvas, context } = whiteCanvas(image.naturalWidth * scale, image.naturalHeight * scale);
    context.drawImage(image, 0, 0, canvas.width, canvas.height);
    return canvas;
  } finally {
    URL.revokeObjectURL(url);
  }
}

async function renderPDFFirstPage(file: File) {
  const [pdfjs, worker] = await Promise.all([import('pdfjs-dist'), import('pdfjs-dist/build/pdf.worker.min.mjs?url')]);
  pdfjs.GlobalWorkerOptions.workerSrc = worker.default;
  const task = pdfjs.getDocument({ data: new Uint8Array(await file.arrayBuffer()) });
  try {
    const pdf = await task.promise.catch(() => { throw new Error('Berkas PDF tidak dapat dibaca.'); });
    const page = await pdf.getPage(1);
    const base = page.getViewport({ scale: 1 });
    const viewport = page.getViewport({ scale: maxSide / Math.max(base.width, base.height) });
    const { canvas } = whiteCanvas(viewport.width, viewport.height);
    await page.render({ canvas, viewport }).promise;
    return canvas;
  } finally {
    void task.destroy();
  }
}
