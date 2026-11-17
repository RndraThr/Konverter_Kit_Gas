import { expect, type Page } from '@playwright/test';

const sidebarGroupByItem: Record<string, string> = {
  'Data Penerima': 'Ringkasan',
  'Map Distribusi': 'Ringkasan',
  'Persiapan program': 'Persiapan',
  DCP3: 'Persiapan',
  Pendistribusian: 'Distribusi',
  'Dokumen Pendukung': 'Distribusi',
  'Berita Acara': 'Distribusi',
  'Ceremony & Sosialisasi': 'Dokumentasi Kegiatan',
  'Pelatihan Teknis': 'Dokumentasi Kegiatan',
  Rakor: 'Dokumentasi Kegiatan',
  'Training 10%': 'Dokumentasi Kegiatan',
  'Training 100%': 'Dokumentasi Kegiatan',
  Unloading: 'Dokumentasi Kegiatan',
  'Laporan DP3': 'Laporan',
  'Laporan Akhir': 'Laporan',
  Pengguna: 'Administrasi',
  'Role & akses': 'Administrasi',
  Pengaturan: 'Administrasi',
  'Kesehatan sistem': 'Administrasi',
  'Riwayat aktivitas': 'Administrasi',
};

export async function navigateDashboard(page: Page, label: string, mobile: boolean) {
  if (label === 'Profil saya') {
    // On CI the menu item sometimes takes the click without navigating, so the
    // whole step is retried until the profile URL is reached.
    await expect(async () => {
      await page.getByRole('button', { name: 'Menu akun' }).click({ timeout: 3_000 });
      await page.getByRole('menuitem', { name: 'Profil saya', exact: true }).click({ timeout: 3_000 });
      await expect(page).toHaveURL(/\/profil/, { timeout: 3_000 });
    }).toPass({ timeout: 20_000 });
    return;
  }

  if (mobile) await page.getByRole('button', { name: 'Buka navigasi' }).click();

  const link = page.getByRole('link', { name: label, exact: true });
  if (!await link.isVisible()) {
    const group = sidebarGroupByItem[label];
    if (!group) throw new Error(`Grup sidebar untuk "${label}" belum didaftarkan`);
    await page.getByRole('button', { name: group, exact: true }).click();
    await expect(link).toBeVisible({ timeout: 3_000 });
  }

  await link.click();
  if (mobile) await expect(page.locator('[data-slot="sheet-overlay"]')).not.toBeVisible();
}
