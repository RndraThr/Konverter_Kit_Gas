// Penandatangan Berita Acara: nama/NIP, tanda tangan digital, dan stempel per
// jadwal (kabupaten). Kunci BA sama dengan nilai tab di halaman Berita Acara.

export type SignatoryRole = 'agriculture_office' | 'installer' | 'supervisor' | 'pertamina';
export type SignatoryKind = 'signature' | 'stamp';

/**
 * Posisi tanda tangan/stempel dalam satuan tinggi kotak tanda tangan (sama
 * seperti PDF): x/y geseran pusat dari tengah kotak, size sisi terpanjang,
 * rotation derajat (positif searah jarum jam).
 */
export type MarkPlacement = { x: number; y: number; size: number; rotation: number };

export type SignatureRule = {
  signature: boolean;
  stamp: boolean;
  signature_placement?: MarkPlacement | null;
  stamp_placement?: MarkPlacement | null;
  /** Kepekatan stempel 0,2-1 (kosong = bawaan 0,92, sama dengan server). */
  stamp_opacity?: number | null;
  /** Stempel di bawah tanda tangan (bawaan: di atas). */
  stamp_below?: boolean;
};

export const defaultStampOpacity = 0.92;
export const minStampOpacity = 0.2;

/** Posisi bawaan stempel bila belum diatur (sama dengan server). */
export const defaultStampPlacement: MarkPlacement = { x: -0.4, y: 0, size: 0.78, rotation: 0 };
export const placementLimits = { maxX: 3, maxY: 0.5 };
export const sizeLimits: Record<SignatoryKind, { min: number; max: number }> = {
  signature: { min: 0.2, max: 4 },
  stamp: { min: 0.3, max: 2.5 },
};
/**
 * Satu satuan posisi tanda tangan/stempel (mm), sama dengan server: tetap,
 * bukan tinggi kotak, agar ukuran dan letaknya sama di semua BA walau
 * kotaknya berbeda (dulu BA Perorangan 15,5 mm, BA lain 28-29 mm).
 */
export const markUnitMM = 28;
/** Batas gambar boleh keluar dari kotak, satuan posisi. */
export const markOverflow = 0.75;

/**
 * Posisi bawaan tanda tangan (sama dengan server): proporsional di tengah
 * kotak, lebar maksimal min(lebar kotak - 6 mm, 38 mm) dan tinggi kotak - 4 mm.
 */
export function defaultSignaturePlacement(aspect: number, boxWidthMM: number, boxHeightMM: number, unitMM = markUnitMM): MarkPlacement {
  const ratio = aspect > 0 ? aspect : 3;
  let width = Math.min(boxWidthMM - 6, (38 * unitMM) / markUnitMM);
  let height = width / ratio;
  if (height > boxHeightMM - 4) {
    height = boxHeightMM - 4;
    width = height * ratio;
  }
  return { x: 0, y: 0, size: Math.round((Math.max(width, height) / unitMM) * 1000) / 1000, rotation: 0 };
}

export const samePlacement = (a?: MarkPlacement | null, b?: MarkPlacement | null) =>
  (a ?? null) === (b ?? null) || (!!a && !!b && a.x === b.x && a.y === b.y && a.size === b.size && a.rotation === b.rotation);

/** Kotak tanda tangan satu pihak pada halaman PDF (mm). */
/** unit: satu satuan posisi di kotak ini (mm); kosong = markUnitMM. */
export type SignatureBox = { role: SignatoryRole; page: number; x: number; y: number; w: number; h: number; unit?: number };

/** Isi editor: PDF halaman tanda tangan tanpa tanda (base64) dan letak kotak. */
export type SignatureLayout = {
  document_key: string;
  page: number;
  page_width: number;
  page_height: number;
  boxes: SignatureBox[];
  pdf: string;
};

export type SignatureRules = Partial<Record<SignatoryRole, SignatureRule>>;

export type SignatoryCard = {
  role: SignatoryRole;
  name: string;
  nip: string;
  signature_updated_at?: string;
  stamp_updated_at?: string;
};

export type SignatureSettings = {
  document_key: string;
  override: boolean;
  /** Mode tanda tangan basah: halaman tanda tangan dicetak lalu pindaiannya diunggah. */
  wet_signature: boolean;
  wet_supported: boolean;
  rules: SignatureRules;
  global_rules: SignatureRules;
  signatories: SignatoryCard[];
};

/** Judul kolom tanda tangan, seperti tercetak di BA. */
export const signatoryHeadings: Record<SignatoryRole, string> = {
  agriculture_office: 'Dinas Pertanian Daerah',
  installer: 'Pelaksana Pemasangan & Pendistribusian',
  supervisor: 'Konsultan Pengawas',
  pertamina: 'PT Pertamina Patra Niaga',
};

export function signatoryPath(scheduleID: string, role: SignatoryRole, kind?: SignatoryKind, version?: string) {
  const query = new URLSearchParams({ schedule_id: scheduleID });
  if (version) query.set('v', version);
  return `/api/v1/bast/signatures/signatories/${role}${kind ? `/${kind}` : ''}?${query.toString()}`;
}

/**
 * Mengubah berkas gambar stempel (PNG/JPG hasil pindai atau foto) menjadi PNG
 * berlatar transparan: diperkecil ke sisi terpanjang 900 px, dan bila
 * removeWhite aktif piksel hampir putih dijadikan transparan.
 */
/**
 * The canvas cut to its visible content plus a small margin: a scanned page
 * with only a stamp on it becomes the stamp, not a sheet of empty paper.
 * Unchanged when nothing is visible.
 */
export function trimTransparent(canvas: HTMLCanvasElement): HTMLCanvasElement {
  const context = canvas.getContext('2d');
  if (!context) return canvas;
  const { width, height } = canvas;
  const data = context.getImageData(0, 0, width, height).data;
  let left = width, top = height, right = -1, bottom = -1;
  for (let y = 0; y < height; y += 1) {
    for (let x = 0; x < width; x += 1) {
      if (data[(y * width + x) * 4 + 3] > 24) {
        if (x < left) left = x;
        if (x > right) right = x;
        if (y < top) top = y;
        if (y > bottom) bottom = y;
      }
    }
  }
  if (right < 0) return canvas;
  const pad = Math.round(Math.max(right - left, bottom - top) * 0.03);
  left = Math.max(0, left - pad); top = Math.max(0, top - pad);
  right = Math.min(width - 1, right + pad); bottom = Math.min(height - 1, bottom + pad);
  if (left === 0 && top === 0 && right === width - 1 && bottom === height - 1) return canvas;
  const trimmed = document.createElement('canvas');
  trimmed.width = right - left + 1;
  trimmed.height = bottom - top + 1;
  trimmed.getContext('2d')?.drawImage(canvas, left, top, trimmed.width, trimmed.height, 0, 0, trimmed.width, trimmed.height);
  return trimmed;
}

export async function imageFileToPNGBase64(file: File, removeWhite: boolean): Promise<string> {
  const url = URL.createObjectURL(file);
  try {
    const image = await new Promise<HTMLImageElement>((resolve, reject) => {
      const element = new Image();
      element.onload = () => resolve(element);
      element.onerror = () => reject(new Error('Berkas bukan gambar yang valid.'));
      element.src = url;
    });
    const scale = Math.min(1, 900 / Math.max(image.naturalWidth, image.naturalHeight));
    const canvas = document.createElement('canvas');
    canvas.width = Math.max(1, Math.round(image.naturalWidth * scale));
    canvas.height = Math.max(1, Math.round(image.naturalHeight * scale));
    const context = canvas.getContext('2d');
    if (!context) throw new Error('Browser tidak mendukung pengolahan gambar.');
    context.drawImage(image, 0, 0, canvas.width, canvas.height);
    if (removeWhite) {
      const pixels = context.getImageData(0, 0, canvas.width, canvas.height);
      const data = pixels.data;
      for (let i = 0; i < data.length; i += 4) {
        const lightest = Math.min(data[i], data[i + 1], data[i + 2]);
        if (lightest > 235) data[i + 3] = 0;
        else if (lightest > 200) data[i + 3] = Math.round(data[i + 3] * (235 - lightest) / 35);
      }
      context.putImageData(pixels, 0, 0);
    }
    const output = removeWhite ? trimTransparent(canvas) : canvas;
    const blob = await new Promise<Blob>((resolve, reject) => output.toBlob((value) => (value ? resolve(value) : reject(new Error('Gagal membuat PNG.'))), 'image/png'));
    const bytes = new Uint8Array(await blob.arrayBuffer());
    let binary = '';
    for (let i = 0; i < bytes.length; i += 0x8000) binary += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
    return btoa(binary);
  } finally {
    URL.revokeObjectURL(url);
  }
}
