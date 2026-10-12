/**
 * Status validasi yang membuat preview tidak dapat dibuat sama sekali. Status
 * lain (pengaturan atau data penerima belum lengkap) hanya menahan finalisasi;
 * preview tetap dibuat dengan isian yang belum ada dibiarkan kosong.
 */
export const previewBlockingStatus = new Set(['zone_not_configured', 'ba_logo_required', 'no_recipients', 'configuration_required', 'error']);

export const incompletePreviewNote = 'Preview memakai data yang ada; isian yang belum ada dibiarkan kosong. Lengkapi data sebelum finalisasi.';
