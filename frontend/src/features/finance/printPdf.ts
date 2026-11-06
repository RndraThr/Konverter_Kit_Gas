let printing: { frame: HTMLIFrameElement; url: string } | null = null;

/** Browser HP tidak punya penampil PDF di halaman, jadi tidak bisa mencetak dari sini. */
export const canPrintPdf = () => navigator.pdfViewerEnabled === true;

/**
 * Membuka dialog cetak browser, yang sekaligus menjadi preview PDF. Frame harus
 * tetap ada selama dialog terbuka, jadi baru dilepas saat cetak berikutnya.
 */
export function printPdf(blob: Blob) {
  if (printing) {
    printing.frame.remove();
    URL.revokeObjectURL(printing.url);
  }
  const url = URL.createObjectURL(blob);
  const frame = document.createElement('iframe');
  frame.title = 'Cetak PDF';
  frame.setAttribute('aria-hidden', 'true');
  frame.style.cssText = 'position:fixed;right:0;bottom:0;width:0;height:0;border:0';
  frame.onload = () => {
    frame.contentWindow?.focus();
    frame.contentWindow?.print();
  };
  frame.src = url;
  document.body.appendChild(frame);
  printing = { frame, url };
}
