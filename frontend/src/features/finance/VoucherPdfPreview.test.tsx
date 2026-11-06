import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { blankSheet, previewVoucher } from './api';
import { printPdf } from './printPdf';
import { VoucherPdfPreview } from './VoucherPdfPreview';

vi.mock('./printPdf', () => ({ canPrintPdf: () => true, printPdf: vi.fn() }));
vi.mock('./api', async (original) => ({ ...await original<typeof import('./api')>(), previewVoucher: vi.fn() }));

function show(blocked: boolean) {
  render(<QueryClientProvider client={new QueryClient()}><VoucherPdfPreview form={blankSheet()} blocked={blocked} /></QueryClientProvider>);
}

describe('VoucherPdfPreview', () => {
  beforeEach(() => vi.clearAllMocks());

  it('builds the PDF on demand and opens the print dialog', async () => {
    const pdf = new Blob(['x'], { type: 'application/pdf' });
    vi.mocked(previewVoucher).mockResolvedValue(pdf);
    show(false);
    expect(previewVoucher).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole('button', { name: 'Preview PDF' }));
    await waitFor(() => expect(printPdf).toHaveBeenCalledWith(pdf));
  });

  it('waits for a complete sheet', () => {
    show(true);
    expect(screen.getByRole('button', { name: 'Preview PDF' })).toBeDisabled();
  });
});
