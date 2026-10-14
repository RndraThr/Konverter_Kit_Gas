import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { expect, test } from 'vitest'
import { PermissionsProvider } from '@/lib/permissions'
import { GuidePage } from './GuidePage'
import { GuideStartCard } from './GuideStartCard'
import { PageHelp } from './PageHelp'

function renderGuide(permissions: string[]) {
  return render(<MemoryRouter><PermissionsProvider permissions={permissions}><GuidePage /></PermissionsProvider></MemoryRouter>)
}

const stepSection = (title: string) => screen.getByRole('region', { name: title })

test('marks only the steps the role can do as its tasks and keeps the rest for context', () => {
  renderGuide(['distribution.view', 'distribution.pos_dokumen'])
  const distribusi = stepSection('Pendistribusian di lapangan')
  expect(within(distribusi).getByText('Tugas Anda')).toBeVisible()
  expect(within(distribusi).getByRole('link', { name: /Buka halaman/ })).toHaveAttribute('href', '/dokumentasi/pendistribusian')
  // Langkah POS lain tetap terlihat, dengan keterangan siapa yang mengerjakan.
  expect(within(distribusi).getAllByText('(dikerjakan pemegang akses lain)').length).toBeGreaterThan(0)

  const persiapan = stepSection('Persiapan program')
  expect(within(persiapan).getByText('Dikerjakan tim lain')).toBeVisible()
  expect(within(persiapan).queryByRole('link', { name: /Buka halaman/ })).not.toBeInTheDocument()
  expect(within(stepSection('Laporan & pemantauan')).getByText('Anda dapat melihat')).toBeVisible()
})

test('shows example buttons and numbered screenshots of the real page', () => {
  renderGuide(['*'])
  const ba = stepSection('Berita Acara')
  expect(within(ba).getByRole('img', { name: 'Contoh tombol Finalisasi & sinkronkan' })).toBeVisible()
  expect(within(ba).getByRole('img', { name: 'Contoh tampilan: Berita Acara, Closing Titik Serah' })).toHaveAttribute('src', '/static/images/guide/berita-acara.jpg')
  expect(within(ba).getByText('Finalisasi setelah preview benar (aktif bila data lengkap)')).toBeVisible()
})

test('Hanya tugas saya hides the steps of other teams', async () => {
  renderGuide(['activities.view', 'activities.manage'])
  await userEvent.click(screen.getByRole('button', { name: 'Hanya tugas saya' }))
  expect(stepSection('Dokumentasi kegiatan')).toBeVisible()
  expect(screen.queryByRole('region', { name: 'Persiapan program' })).not.toBeInTheDocument()
})

test('start card lists the role tasks in order and explains accounts without access', () => {
  const { unmount } = render(<MemoryRouter><GuideStartCard permissions={['dcp3.view', 'dcp3.import', 'recipients.view']} /></MemoryRouter>)
  const items = screen.getAllByRole('listitem')
  expect(items[0]).toHaveTextContent('1. Import data penerima (DCP3)')
  expect(within(items[0]).getByRole('link', { name: /Buka/ })).toHaveAttribute('href', '/dcp3')
  unmount()

  render(<MemoryRouter><GuideStartCard permissions={[]} /></MemoryRouter>)
  expect(screen.getByText(/belum memiliki akses ke menu kerja/)).toBeVisible()
})

test('the help button explains the current page for the current role', async () => {
  render(<MemoryRouter initialEntries={['/berita-acara']}><PageHelp permissions={['bast.view']} /></MemoryRouter>)
  await userEvent.click(screen.getByRole('button', { name: 'Bantuan halaman ini' }))
  const ba = await screen.findByRole('region', { name: 'Berita Acara' })
  expect(within(ba).getByText('Anda dapat melihat')).toBeVisible()
  expect(screen.getByRole('link', { name: /Buka Panduan lengkap/ })).toHaveAttribute('href', '/panduan')
})

test('every screenshot referenced by the guide exists with its numbered marks', async () => {
  const { guideSteps } = await import('./guideContent')
  const shots = (await import('./guideShots.json')).default as Record<string, { marks: { n: number }[] }>
  for (const step of guideSteps) {
    for (const spec of step.shots ?? []) {
      const shot = shots[spec.id]
      expect(shot, spec.id).toBeDefined()
      // Setiap nomor di gambar dijelaskan di legenda.
      for (const mark of shot.marks) expect(mark.n, `${spec.id} #${mark.n}`).toBeLessThanOrEqual(spec.legend.length)
    }
  }
})
