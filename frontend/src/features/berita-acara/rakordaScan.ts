export type ScanColorMode = 'original' | 'enhanced' | 'grayscale' | 'black-white';
export type ScanPoint = { x: number; y: number };
export type ScanCorners = [ScanPoint, ScanPoint, ScanPoint, ScanPoint];
export type ScanPageOptions = { corners: ScanCorners; rotation: 0 | 90 | 180 | 270; colorMode: ScanColorMode };
export type JpegPage = { bytes: Uint8Array; width: number; height: number };

const clamp = (value: number) => Math.max(0, Math.min(255, Math.round(value)));

// Separable box-blur via a summed-area table, used both for unsharp-mask
// sharpening and for the adaptive black-white threshold below. O(n) after
// the one-time integral pass regardless of blur radius.
function buildIntegral(values: Float32Array, width: number, height: number): Float64Array {
  const stride = width + 1;
  const integral = new Float64Array(stride * (height + 1));
  for (let y = 0; y < height; y++) {
    let rowSum = 0;
    const row = y * width;
    const outRow = (y + 1) * stride;
    const prevOutRow = y * stride;
    for (let x = 0; x < width; x++) {
      rowSum += values[row + x];
      integral[outRow + x + 1] = integral[prevOutRow + x + 1] + rowSum;
    }
  }
  return integral;
}

function boxMean(integral: Float64Array, width: number, height: number, x: number, y: number, radius: number): number {
  const stride = width + 1;
  const x0 = Math.max(0, x - radius), x1 = Math.min(width - 1, x + radius);
  const y0 = Math.max(0, y - radius), y1 = Math.min(height - 1, y + radius);
  const sum = integral[(y1 + 1) * stride + (x1 + 1)] - integral[y0 * stride + (x1 + 1)] - integral[(y1 + 1) * stride + x0] + integral[y0 * stride + x0];
  return sum / ((x1 - x0 + 1) * (y1 - y0 + 1));
}

// Restrained unsharp mask: boosts edge contrast (handwriting, signatures)
// without amplifying paper-grain noise. Reads from a snapshot so each pixel
// sees only the pre-sharpen values of its neighbors.
function sharpen(pixels: Uint8ClampedArray, width: number, height: number, amount: number) {
  const radius = 1;
  for (const channel of [0, 1, 2]) {
    const plane = new Float32Array(width * height);
    for (let i = 0; i < plane.length; i++) plane[i] = pixels[i * 4 + channel];
    const integral = buildIntegral(plane, width, height);
    for (let y = 0; y < height; y++) {
      for (let x = 0; x < width; x++) {
        const index = y * width + x;
        const blurred = boxMean(integral, width, height, x, y, radius);
        const original = plane[index];
        pixels[index * 4 + channel] = clamp(original + (original - blurred) * amount);
      }
    }
  }
}

export function applyScanColor(pixels: Uint8ClampedArray, width: number, height: number, mode: ScanColorMode) {
  if (mode === 'original') return;

  if (mode === 'black-white') {
    const luminance = new Float32Array(width * height);
    for (let index = 0, pixel = 0; index < pixels.length; index += 4, pixel++) {
      luminance[pixel] = pixels[index] * 0.299 + pixels[index + 1] * 0.587 + pixels[index + 2] * 0.114;
    }
    // Adaptive local-mean threshold instead of one fixed cutoff, so uneven
    // phone-camera lighting (a shadow across half the page) doesn't blow
    // out one side while leaving the other illegible.
    const radius = Math.max(8, Math.round(Math.min(width, height) / 12));
    const integral = buildIntegral(luminance, width, height);
    const bias = 12;
    for (let y = 0; y < height; y++) {
      for (let x = 0; x < width; x++) {
        const index = (y * width + x) * 4;
        const local = boxMean(integral, width, height, x, y, radius);
        const value = luminance[y * width + x] > local - bias ? 255 : 0;
        pixels[index] = pixels[index + 1] = pixels[index + 2] = value;
      }
    }
    return;
  }

  for (let index = 0; index < pixels.length; index += 4) {
    const red = pixels[index], green = pixels[index + 1], blue = pixels[index + 2];
    const luminance = red * 0.299 + green * 0.587 + blue * 0.114;
    if (mode === 'grayscale') {
      const gray = clamp((luminance - 128) * 1.12 + 128);
      pixels[index] = pixels[index + 1] = pixels[index + 2] = gray;
    } else {
      const contrast = 1.22;
      const saturation = 1.16;
      pixels[index] = clamp((((red - 128) * contrast + 128) - luminance) * saturation + luminance + 5);
      pixels[index + 1] = clamp((((green - 128) * contrast + 128) - luminance) * saturation + luminance + 5);
      pixels[index + 2] = clamp((((blue - 128) * contrast + 128) - luminance) * saturation + luminance + 5);
    }
  }
  // Restrained sharpening pass for photographed paper; skipped for
  // black-white (already binarized) and kept gentle for grayscale.
  sharpen(pixels, width, height, mode === 'enhanced' ? 0.45 : 0.3);
}

export async function processScanImage(file: File, options: ScanPageOptions): Promise<{ blob: Blob; width: number; height: number }> {
  const bitmap = await createImageBitmap(file);
  const maxSide = 1800;
  const scale = Math.min(1, maxSide / Math.max(bitmap.width, bitmap.height));
  const sourceWidth = Math.max(1, Math.round(bitmap.width * scale));
  const sourceHeight = Math.max(1, Math.round(bitmap.height * scale));
  const sourceCanvas = document.createElement('canvas');
  sourceCanvas.width = sourceWidth;
  sourceCanvas.height = sourceHeight;
  const sourceContext = sourceCanvas.getContext('2d', { willReadFrequently: true });
  if (!sourceContext) throw new Error('Canvas tidak tersedia');
  sourceContext.drawImage(bitmap, 0, 0, sourceWidth, sourceHeight);
  bitmap.close();
  const source = sourceContext.getImageData(0, 0, sourceWidth, sourceHeight);

  const corners = options.corners.map((point) => ({ x: point.x * (sourceWidth - 1), y: point.y * (sourceHeight - 1) })) as ScanCorners;
  const distance = (a: ScanPoint, b: ScanPoint) => Math.hypot(a.x - b.x, a.y - b.y);
  const cropWidth = Math.max(1, Math.round(Math.max(distance(corners[0], corners[1]), distance(corners[3], corners[2]))));
  const cropHeight = Math.max(1, Math.round(Math.max(distance(corners[0], corners[3]), distance(corners[1], corners[2]))));
  const cropped = new ImageData(cropWidth, cropHeight);

  for (let y = 0; y < cropHeight; y++) {
    const v = cropHeight === 1 ? 0 : y / (cropHeight - 1);
    for (let x = 0; x < cropWidth; x++) {
      const u = cropWidth === 1 ? 0 : x / (cropWidth - 1);
      const topX = corners[0].x + (corners[1].x - corners[0].x) * u;
      const topY = corners[0].y + (corners[1].y - corners[0].y) * u;
      const bottomX = corners[3].x + (corners[2].x - corners[3].x) * u;
      const bottomY = corners[3].y + (corners[2].y - corners[3].y) * u;
      const sourceX = Math.max(0, Math.min(sourceWidth - 1, Math.round(topX + (bottomX - topX) * v)));
      const sourceY = Math.max(0, Math.min(sourceHeight - 1, Math.round(topY + (bottomY - topY) * v)));
      const sourceIndex = (sourceY * sourceWidth + sourceX) * 4;
      const targetIndex = (y * cropWidth + x) * 4;
      cropped.data[targetIndex] = source.data[sourceIndex];
      cropped.data[targetIndex + 1] = source.data[sourceIndex + 1];
      cropped.data[targetIndex + 2] = source.data[sourceIndex + 2];
      cropped.data[targetIndex + 3] = 255;
    }
  }
  applyScanColor(cropped.data, cropWidth, cropHeight, options.colorMode);

  const rotated = options.rotation === 90 || options.rotation === 270;
  const output = document.createElement('canvas');
  output.width = rotated ? cropHeight : cropWidth;
  output.height = rotated ? cropWidth : cropHeight;
  const outputContext = output.getContext('2d');
  if (!outputContext) throw new Error('Canvas tidak tersedia');
  const cropCanvas = document.createElement('canvas');
  cropCanvas.width = cropWidth;
  cropCanvas.height = cropHeight;
  cropCanvas.getContext('2d')!.putImageData(cropped, 0, 0);
  outputContext.translate(output.width / 2, output.height / 2);
  outputContext.rotate((options.rotation * Math.PI) / 180);
  outputContext.drawImage(cropCanvas, -cropWidth / 2, -cropHeight / 2);
  const blob = await new Promise<Blob>((resolve, reject) => output.toBlob((value) => value ? resolve(value) : reject(new Error('Foto gagal diproses')), 'image/jpeg', 0.9));
  return { blob, width: output.width, height: output.height };
}

const encode = (value: string) => new TextEncoder().encode(value);
const concat = (chunks: Uint8Array[]) => {
  const output = new Uint8Array(chunks.reduce((sum, chunk) => sum + chunk.length, 0));
  let offset = 0;
  for (const chunk of chunks) { output.set(chunk, offset); offset += chunk.length; }
  return output;
};

export function buildJpegPdf(pages: JpegPage[]): Uint8Array {
  if (pages.length === 0) throw new Error('Belum ada halaman yang dipindai');
  const objectCount = 2 + pages.length * 3;
  const objects: Uint8Array[] = new Array(objectCount + 1);
  const pageRefs = pages.map((_, index) => `${3 + index * 3} 0 R`).join(' ');
  objects[1] = encode('<< /Type /Catalog /Pages 2 0 R >>');
  objects[2] = encode(`<< /Type /Pages /Count ${pages.length} /Kids [${pageRefs}] >>`);
  pages.forEach((page, index) => {
    const pageObject = 3 + index * 3;
    const imageObject = pageObject + 1;
    const contentObject = pageObject + 2;
    const pageWidth = 595.28, pageHeight = 841.89, margin = 18;
    const fit = Math.min((pageWidth - margin * 2) / page.width, (pageHeight - margin * 2) / page.height);
    const width = page.width * fit, height = page.height * fit;
    const x = (pageWidth - width) / 2, y = (pageHeight - height) / 2;
    const commands = `q\n${width.toFixed(2)} 0 0 ${height.toFixed(2)} ${x.toFixed(2)} ${y.toFixed(2)} cm\n/Im${index + 1} Do\nQ\n`;
    objects[pageObject] = encode(`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 ${pageWidth} ${pageHeight}] /Resources << /XObject << /Im${index + 1} ${imageObject} 0 R >> >> /Contents ${contentObject} 0 R >>`);
    objects[imageObject] = concat([encode(`<< /Type /XObject /Subtype /Image /Width ${page.width} /Height ${page.height} /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length ${page.bytes.length} >>\nstream\n`), page.bytes, encode('\nendstream')]);
    objects[contentObject] = encode(`<< /Length ${encode(commands).length} >>\nstream\n${commands}endstream`);
  });

  const chunks: Uint8Array[] = [encode('%PDF-1.4\n%SCAN\n')];
  const offsets = new Array(objectCount + 1).fill(0);
  let length = chunks[0].length;
  for (let id = 1; id <= objectCount; id++) {
    offsets[id] = length;
    const chunk = concat([encode(`${id} 0 obj\n`), objects[id], encode('\nendobj\n')]);
    chunks.push(chunk); length += chunk.length;
  }
  const xrefOffset = length;
  let xref = `xref\n0 ${objectCount + 1}\n0000000000 65535 f \n`;
  for (let id = 1; id <= objectCount; id++) xref += `${String(offsets[id]).padStart(10, '0')} 00000 n \n`;
  xref += `trailer\n<< /Size ${objectCount + 1} /Root 1 0 R >>\nstartxref\n${xrefOffset}\n%%EOF`;
  chunks.push(encode(xref));
  return concat(chunks);
}

export const defaultScanCorners = (): ScanCorners => [{ x: 0.04, y: 0.04 }, { x: 0.96, y: 0.04 }, { x: 0.96, y: 0.96 }, { x: 0.04, y: 0.96 }];
