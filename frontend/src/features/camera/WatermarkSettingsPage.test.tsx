import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, expect, test, vi } from 'vitest'
import { WatermarkSettingsPage } from './WatermarkSettingsPage'
import { DEFAULT_WATERMARK_SETTINGS } from './watermarkSettings'

const { downscaleLogo } = vi.hoisted(() => ({ downscaleLogo: vi.fn() }))
vi.mock('./imageResize', () => ({ downscaleLogo }))

const KEY = 'konkit.watermark.v1'
const stored = () => JSON.parse(localStorage.getItem(KEY) ?? 'null')

beforeEach(() => {
  vi.restoreAllMocks()
  localStorage.clear()
  downscaleLogo.mockReset()
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null)
})

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}><MemoryRouter><WatermarkSettingsPage /></MemoryRouter></QueryClientProvider>)
}

test('toggles persist', () => {
  renderPage()
  fireEvent.click(screen.getByRole('checkbox', { name: 'Tampilkan logo' }))
  expect(stored().showLogo).toBe(false)
  fireEvent.click(screen.getByRole('checkbox', { name: 'Tampilkan waktu & lokasi' }))
  expect(stored().showTimestamp).toBe(false)
})

test('side, colours, text size, logo size and quality persist', () => {
  renderPage()
  fireEvent.click(within(screen.getByRole('group', { name: 'Posisi waktu' })).getByRole('button', { name: 'Kiri' }))
  expect(stored().timestampSide).toBe('left')
  fireEvent.click(within(screen.getByRole('group', { name: 'Warna teks' })).getByRole('button', { name: 'Putih' }))
  expect(stored().textColor).toBe('#FFFFFF')
  fireEvent.click(within(screen.getByRole('group', { name: 'Warna aksen' })).getByRole('button', { name: 'Biru' }))
  expect(stored().accentColor).toBe('#60A5FA')
  fireEvent.click(within(screen.getByRole('group', { name: 'Ukuran teks' })).getByRole('button', { name: 'Besar' }))
  expect(stored().textScale).toBe(1.2)
  fireEvent.change(screen.getByRole('slider', { name: 'Ukuran logo' }), { target: { value: '0.7' } })
  expect(stored().logoScale).toBe(0.7)
  fireEvent.click(within(screen.getByRole('group', { name: 'Kualitas foto' })).getByRole('button', { name: /Hemat/ }))
  expect(stored().photoQuality).toBe('hemat')
})

test('the selected option is marked pressed', () => {
  renderPage()
  const side = screen.getByRole('group', { name: 'Posisi waktu' })
  expect(within(side).getByRole('button', { name: 'Kanan' })).toHaveAttribute('aria-pressed', 'true')
  expect(within(side).getByRole('button', { name: 'Kiri' })).toHaveAttribute('aria-pressed', 'false')
})

test('backdate is stored and "Hapus backdate" clears it', () => {
  renderPage()
  expect(screen.getByText('Hanya tanggal di watermark yang berubah; waktu asli tetap tercatat.')).toBeInTheDocument()
  fireEvent.change(screen.getByLabelText('Backdate'), { target: { value: '2026-10-20' } })
  expect(stored().backdate).toBe('2026-10-20')
  fireEvent.click(screen.getByRole('button', { name: 'Hapus backdate' }))
  expect(stored().backdate).toBeNull()
})

test('a picked custom logo is downscaled and stored; it can be reverted', async () => {
  downscaleLogo.mockResolvedValue('data:image/png;base64,AAA')
  renderPage()
  const file = new File(['x'], 'logo.png', { type: 'image/png' })
  fireEvent.change(screen.getByLabelText('Pilih gambar'), { target: { files: [file] } })
  await waitFor(() => expect(stored()?.customLogo).toBe('data:image/png;base64,AAA'))
  expect(downscaleLogo).toHaveBeenCalledWith(file)
  fireEvent.click(screen.getByRole('button', { name: 'Kembalikan logo bawaan' }))
  expect(stored().customLogo).toBeNull()
})

test('"Kembalikan posisi" clears both placements', () => {
  localStorage.setItem(KEY, JSON.stringify({ ...DEFAULT_WATERMARK_SETTINGS, logoPlacement: { x: 0.2, y: 0.2, scale: 1 }, timestampPlacement: { x: 0.5, y: 0.5, scale: 2 } }))
  renderPage()
  fireEvent.click(screen.getByRole('button', { name: 'Kembalikan posisi' }))
  expect(stored()).toMatchObject({ logoPlacement: null, timestampPlacement: null })
})

test('"Bawaan" resets everything after confirming', async () => {
  localStorage.setItem(KEY, JSON.stringify({ ...DEFAULT_WATERMARK_SETTINGS, textColor: '#FFFFFF', backdate: '2026-10-01' }))
  renderPage()
  fireEvent.click(screen.getByRole('button', { name: 'Bawaan' }))
  const dialog = await screen.findByRole('alertdialog')
  expect(stored().textColor).toBe('#FFFFFF') // nothing changes before confirming
  fireEvent.click(within(dialog).getByRole('button', { name: 'Kembalikan' }))
  await waitFor(() => expect(within(screen.getByRole('group', { name: 'Warna teks' })).getByRole('button', { name: 'Kuning' })).toHaveAttribute('aria-pressed', 'true'))
  expect(stored() ?? DEFAULT_WATERMARK_SETTINGS).toEqual(DEFAULT_WATERMARK_SETTINGS)
})

test('warns when the device refuses to store the settings', () => {
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new DOMException('full', 'QuotaExceededError') })
  renderPage()
  fireEvent.click(screen.getByRole('checkbox', { name: 'Tampilkan logo' }))
  expect(screen.getByRole('alert')).toHaveTextContent('Pengaturan tidak dapat disimpan di perangkat ini.')
})
