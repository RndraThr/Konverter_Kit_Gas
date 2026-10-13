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
};

/** Posisi bawaan stempel bila belum diatur (sama dengan server). */
export const defaultStampPlacement: MarkPlacement = { x: -0.4, y: 0, size: 0.78, rotation: 0 };
export const placementLimits = { maxX: 3, maxY: 0.5 };
export const sizeLimits: Record<SignatoryKind, { min: number; max: number }> = {
  signature: { min: 0.2, max: 4 },
  stamp: { min: 0.3, max: 2.5 },
};
/** Batas gambar boleh keluar dari kotak, satuan tinggi kotak. */
export const markOverflow = 0.75;

/**
 * Posisi bawaan tanda tangan (sama dengan server): proporsional di tengah
 * kotak, lebar maksimal min(lebar kotak - 6 mm, 60 mm) dan tinggi kotak - 4 mm.
 */
export function defaultSignaturePlacement(aspect: number, boxWidthMM: number, boxHeightMM: number): MarkPlacement {
  const ratio = aspect > 0 ? aspect : 3;
  let width = Math.min(boxWidthMM - 6, 60);
  let height = width / ratio;
  if (height > boxHeightMM - 4) {
    height = boxHeightMM - 4;
    width = height * ratio;
  }
  return { x: 0, y: 0, size: Math.round((Math.max(width, height) / boxHeightMM) * 1000) / 1000, rotation: 0 };
}

export const samePlacement = (a?: MarkPlacement | null, b?: MarkPlacement | null) =>
  (a ?? null) === (b ?? null) || (!!a && !!b && a.x === b.x && a.y === b.y && a.size === b.size && a.rotation === b.rotation);

/** Kotak tanda tangan satu pihak pada halaman PDF (mm). */
export type SignatureBox = { role: SignatoryRole; page: number; x: number; y: number; w: number; h: number };

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
    const blob = await new Promise<Blob>((resolve, reject) => canvas.toBlob((value) => (value ? resolve(value) : reject(new Error('Gagal membuat PNG.'))), 'image/png'));
    const bytes = new Uint8Array(await blob.arrayBuffer());
    let binary = '';
    for (let i = 0; i < bytes.length; i += 0x8000) binary += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
    return btoa(binary);
  } finally {
    URL.revokeObjectURL(url);
  }
}
