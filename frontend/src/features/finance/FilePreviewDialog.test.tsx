import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { toast } from 'sonner';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fetchFile } from './api';
import { FilePreviewButton } from './FilePreviewDialog';
import { canPrintPdf, printPdf } from './printPdf';

vi.mock('@/features/berita-acara/PdfCanvasPreview', () => ({ default: ({ label }: { label: string }) => <div title={`Preview ${label}`} /> }));
vi.mock('./printPdf', () => ({ canPrintPdf: vi.fn(), printPdf: vi.fn() }));
vi.mock('./api', async (original) => ({ ...await original<typeof import('./api')>(), fetchFile: vi.fn() }));

function renderButton() {
  render(<QueryClientProvider client={new QueryClient()}><FilePreviewButton path="/file" filename="nota.pdf">Lihat</FilePreviewButton></QueryClientProvider>);
}

const pdf = new Blob(['x'], { type: 'application/pdf' });

describe('FilePreviewButton', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(canPrintPdf).mockReturnValue(true);
    URL.createObjectURL = vi.fn(() => 'blob:preview');
    URL.revokeObjectURL = vi.fn();
  });

  it('sends a PDF straight to the print dialog', async () => {
    vi.mocked(fetchFile).mockResolvedValue(pdf);
    renderButton();
    await userEvent.click(screen.getByRole('button', { name: 'Lihat' }));
    await waitFor(() => expect(printPdf).toHaveBeenCalledWith(pdf));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('shows a PDF in the viewer where the browser cannot print it, and frees it on close', async () => {
    vi.mocked(canPrintPdf).mockReturnValue(false);
    vi.mocked(fetchFile).mockResolvedValue(pdf);
    renderButton();
    await userEvent.click(screen.getByRole('button', { name: 'Lihat' }));
    expect(await screen.findByTitle('Preview nota.pdf')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Perbesar' }));
    expect(screen.getByText('125%')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Tutup' }));
    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:preview');
    expect(printPdf).not.toHaveBeenCalled();
  });

  it('keeps images in the viewer', async () => {
    vi.mocked(fetchFile).mockResolvedValue(new Blob(['x'], { type: 'image/png' }));
    renderButton();
    await userEvent.click(screen.getByRole('button', { name: 'Lihat' }));
    expect(await screen.findByRole('img', { name: 'nota.pdf' })).toHaveAttribute('src', 'blob:preview');
    expect(printPdf).not.toHaveBeenCalled();
  });

  it('zooms an image from the buttons and the mouse wheel, and resets', async () => {
    vi.mocked(fetchFile).mockResolvedValue(new Blob(['x'], { type: 'image/png' }));
    renderButton();
    await userEvent.click(screen.getByRole('button', { name: 'Lihat' }));
    const stage = await screen.findByRole('group', { name: 'Area gambar' });
    expect(screen.getByRole('button', { name: 'Perkecil' })).toBeDisabled();
    await userEvent.click(screen.getByRole('button', { name: 'Perbesar' }));
    expect(screen.getByText('125%')).toBeInTheDocument();
    fireEvent.wheel(stage, { deltaY: -200 });
    expect(screen.getByText('169%')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Kembali ke ukuran awal' }));
    expect(screen.getByText('100%')).toBeInTheDocument();
  });

  it('says so when the file cannot be fetched', async () => {
    const error = vi.spyOn(toast, 'error');
    vi.mocked(fetchFile).mockRejectedValue(new Error('down'));
    renderButton();
    await userEvent.click(screen.getByRole('button', { name: 'Lihat' }));
    await waitFor(() => expect(error).toHaveBeenCalled());
  });
});
