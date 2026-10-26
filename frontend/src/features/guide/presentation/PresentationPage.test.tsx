import { fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeAll, expect, test } from 'vitest';
import { PresentationPage } from './PresentationPage';
import { slides } from './slides';

beforeAll(() => {
  globalThis.ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
});

function renderPresentation() {
  return render(<MemoryRouter initialEntries={['/panduan/presentasi']}>
    <Routes>
      <Route path="/panduan/presentasi" element={<PresentationPage />} />
      <Route path="/panduan" element={<p>Halaman panduan</p>} />
    </Routes>
  </MemoryRouter>);
}

test('opens on the cover and moves with the arrow keys', () => {
  renderPresentation();
  expect(screen.getByRole('group', { name: `1 dari ${slides.length}: Konkit` })).toBeVisible();
  fireEvent.keyDown(window, { key: 'ArrowRight' });
  expect(screen.getByRole('group', { name: new RegExp(`^2 dari ${slides.length}`) })).toBeVisible();
  fireEvent.keyDown(window, { key: 'ArrowLeft' });
  fireEvent.keyDown(window, { key: 'ArrowLeft' });
  expect(screen.getByRole('group', { name: new RegExp(`^1 dari ${slides.length}`) })).toBeVisible();
  fireEvent.keyDown(window, { key: 'End' });
  expect(screen.getByRole('group', { name: new RegExp(`^${slides.length} dari ${slides.length}`) })).toBeVisible();
});

test('jumps to any slide from the overview and closes back to the guide', async () => {
  renderPresentation();
  await userEvent.click(screen.getByRole('button', { name: 'Semua slide' }));
  await userEvent.click(screen.getByRole('button', { name: /Aplikasi HP/ }));
  expect(screen.getByRole('group', { name: /Aplikasi HP$/ })).toBeVisible();
  await userEvent.click(screen.getByRole('button', { name: 'Tutup presentasi' }));
  expect(screen.getByText('Halaman panduan')).toBeVisible();
});

test('covers every step of the guide', () => {
  for (const id of ['persiapan', 'akses', 'dcp3', 'penerima', 'distribusi', 'kegiatan', 'berita-acara', 'laporan']) {
    expect(slides.some((slide) => slide.id === id)).toBe(true);
  }
});
