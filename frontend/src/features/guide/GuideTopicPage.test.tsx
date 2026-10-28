import { render, screen, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { expect, test } from 'vitest'
import { PermissionsProvider } from '@/lib/permissions'
import { GuideTopicPage } from './GuideTopicPage'
import shots from './guideShots.json'
import { guideTopics } from './guideTopics'

function renderTopic(path: string, permissions: string[]) {
  return render(<MemoryRouter initialEntries={[path]}><PermissionsProvider permissions={permissions}><Routes>
    <Route path="/panduan/:slug" element={<GuideTopicPage />} />
    <Route path="/panduan" element={<p>Panduan utama</p>} />
  </Routes></PermissionsProvider></MemoryRouter>)
}

test('Persiapan program tutorial breaks down every tab with screenshots, steps, and field explanations', () => {
  renderTopic('/panduan/persiapan-program', ['programs.view', 'programs.manage'])
  expect(screen.getByRole('heading', { level: 1, name: 'Persiapan program' })).toBeVisible()
  const toc = screen.getByRole('navigation', { name: 'Daftar isi' })
  for (const title of ['Kabupaten', 'Program', 'Zona', 'Template paket', 'Template dokumentasi', 'Logo Dokumen', 'Jadwal']) {
    expect(within(toc).getByRole('link', { name: new RegExp(`^[0-9]+. ${title}$`) })).toBeVisible()
  }
  const jadwal = screen.getByRole('region', { name: /Jadwal$/ })
  expect(within(jadwal).getByRole('img', { name: 'Contoh tampilan: Modal Edit jadwal' })).toBeVisible()
  expect(within(jadwal).getByRole('cell', { name: 'Kuota slot' })).toBeVisible()
  expect(screen.getByText('Tugas Anda')).toBeVisible()
})

test('marks steps for other roles and hides the open button without view access', () => {
  renderTopic('/panduan/persiapan-program', ['distribution.view'])
  expect(screen.getByText('Dikerjakan tim lain')).toBeVisible()
  expect(screen.queryByRole('link', { name: /^Buka Persiapan program/ })).not.toBeInTheDocument()
  expect(screen.getAllByText('(dikerjakan pemegang akses lain)').length).toBeGreaterThan(0)
})

test('unknown topics go back to the main guide', () => {
  renderTopic('/panduan/tidak-ada', ['*'])
  expect(screen.getByText('Panduan utama')).toBeVisible()
})

test('every tutorial screenshot exists and every number is explained', () => {
  const manifest = shots as Record<string, { marks: { n: number }[] }>
  for (const topic of guideTopics) for (const section of topic.sections) for (const spec of section.shots ?? []) {
    expect(manifest[spec.id], spec.id).toBeDefined()
    for (const mark of manifest[spec.id].marks) expect(mark.n, `${spec.id} #${mark.n}`).toBeLessThanOrEqual(spec.legend.length)
  }
})

test('Laporan topic explains Map Distribusi colours, review marker, titik serah, basemaps, light mode and unlocated points', () => {
  renderTopic('/panduan/laporan', ['distribution.view'])
  const peta = screen.getByRole('region', { name: /Map Distribusi$/ })
  for (const text of ['Terjadwal', 'Penerima tertaut', 'Perlu ditinjau', 'Titik serah', 'Biasa', 'Satelit', 'Hybrid', 'Mode ringan', 'belum ada lokasi']) {
    expect(within(peta).getAllByText(new RegExp(text)).length).toBeGreaterThan(0)
  }
})
