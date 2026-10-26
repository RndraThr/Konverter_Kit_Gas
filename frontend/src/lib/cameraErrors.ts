export function cameraErrorMessage(error: unknown): string {
  const name = error instanceof DOMException ? error.name : undefined;
  switch (name) {
    case 'NotAllowedError':
    case 'PermissionDeniedError':
      return 'Izin kamera ditolak. Aktifkan izin kamera untuk browser ini di pengaturan perangkat, lalu coba lagi.';
    case 'NotFoundError':
    case 'DevicesNotFoundError':
      return 'Kamera tidak ditemukan pada perangkat ini.';
    case 'NotReadableError':
    case 'TrackStartError':
      return 'Kamera sedang dipakai aplikasi lain. Tutup aplikasi lain yang menggunakan kamera, lalu coba lagi.';
    case 'OverconstrainedError':
      return 'Kamera tidak mendukung pengaturan yang diminta. Coba pilih kamera lain.';
    default:
      return 'Kamera tidak dapat diakses. Beri izin kamera pada browser, lalu coba lagi.';
  }
}
