// Penandatangan Berita Acara: nama/NIP, tanda tangan digital, dan stempel per
// jadwal (kabupaten). Kunci BA sama dengan nilai tab di halaman Berita Acara.

export type SignatoryRole = 'agriculture_office' | 'installer' | 'supervisor' | 'pertamina';
export type SignatoryKind = 'signature' | 'stamp';

export type SignatureRule = { signature: boolean; stamp: boolean };
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
