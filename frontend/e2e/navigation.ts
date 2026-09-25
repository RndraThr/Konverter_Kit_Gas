import { expect, type Page } from '@playwright/test';

const sidebarGroupByItem: Record<string, string> = {
  'Data Penerima': 'Dashboard',
  'Map Distribusi': 'Dashboard',
  'Persiapan program': 'Operasional',
  DCP3: 'Operasional',
  Pendistribusian: 'Dokumentasi',
  'Ceremony & Sosialisasi': 'Dokumentasi',
  'Pelatihan Teknis': 'Dokumentasi',
  Rakor: 'Dokumentasi',
  'Training 10%': 'Dokumentasi',
  'Training 100%': 'Dokumentasi',
  Unloading: 'Dokumentasi',
  Laporan: 'Laporan',
  Pengguna: 'Administrasi',
  'Role & akses': 'Administrasi',
  Pengaturan: 'Sistem',
  'Kesehatan sistem': 'Sistem',
  'Riwayat aktivitas': 'Sistem',
};

export async function navigateDashboard(page: Page, label: string, mobile: boolean) {
  if (label === 'Profil saya') {
    await page.getByRole('button', { name: 'Menu akun' }).click();
    await page.getByRole('menuitem', { name: 'Profil saya', exact: true }).click();
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
