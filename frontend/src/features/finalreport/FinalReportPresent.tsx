import { useQuery } from '@tanstack/react-query';
import { ChevronLeft, ChevronRight, LoaderCircle, X } from 'lucide-react';
import { useEffect, useRef, useState, type ReactNode } from 'react';
import { apiRequest } from '../../lib/api';
import { buildDeck, SLIDE_H, SLIDE_W } from './PresentSlides';
import type { FinalReportPreview } from './types';

type DataResponse<T> = { data: T };

const iconButton = 'rounded p-2 text-white hover:bg-white/10 focus-visible:outline focus-visible:outline-2 focus-visible:outline-white disabled:opacity-30 disabled:hover:bg-transparent';

export default function FinalReportPresent({ programId, regencyId, onClose }: { programId: string; regencyId: string; onClose: () => void }) {
  const [index, setIndex] = useState(0);
  const preview = useQuery({
    queryKey: ['final-report', 'preview', programId, regencyId],
    queryFn: () => apiRequest<DataResponse<FinalReportPreview>>(`/api/v1/final-report/preview?program_id=${encodeURIComponent(programId)}&regency_id=${encodeURIComponent(regencyId)}`),
  });

  const slides = preview.data ? buildDeck(preview.data.data) : [];
  const total = slides.length;

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose();
      if (event.key === 'ArrowRight') setIndex((i) => Math.min(i + 1, Math.max(total - 1, 0)));
      if (event.key === 'ArrowLeft') setIndex((i) => Math.max(i - 1, 0));
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [total, onClose]);

  return <div className="fixed inset-0 z-50 flex flex-col bg-black text-white" role="dialog" aria-modal="true" aria-label="Tampilan Laporan Akhir">
    <div className="flex items-center justify-end p-2"><button type="button" aria-label="Tutup" className={iconButton} onClick={onClose}><X aria-hidden="true" /></button></div>
    <div className="flex min-h-0 flex-1 items-center justify-center px-4 sm:px-10">
      {preview.isPending ? <LoaderCircle className="size-10 animate-spin" aria-label="Memuat" role="status" />
        : preview.isError ? <div className="space-y-4 text-center">
          <p>Gagal memuat tampilan laporan.</p>
          <button type="button" className="rounded border border-white/60 px-4 py-2 hover:bg-white/10" onClick={() => { void preview.refetch(); }}>Coba lagi</button>
        </div>
          : <FitSlide>{slides[index]}</FitSlide>}
    </div>
    <div className="flex items-center justify-center gap-6 p-4">
      <button type="button" aria-label="Slide sebelumnya" className={iconButton} disabled={index === 0} onClick={() => setIndex((i) => Math.max(i - 1, 0))}><ChevronLeft aria-hidden="true" /></button>
      <span>{index + 1} / {total || 1}</span>
      <button type="button" aria-label="Slide berikutnya" className={iconButton} disabled={index >= total - 1} onClick={() => setIndex((i) => Math.min(i + 1, total - 1))}><ChevronRight aria-hidden="true" /></button>
    </div>
  </div>;
}

/** Scales the fixed-size 16:9 slide to the largest size that fits its area. */
function FitSlide({ children }: { children: ReactNode }) {
  const area = useRef<HTMLDivElement>(null);
  const [scale, setScale] = useState(1);
  useEffect(() => {
    const el = area.current;
    if (!el) return;
    const fit = () => {
      if (el.clientWidth && el.clientHeight) setScale(Math.min(el.clientWidth / SLIDE_W, el.clientHeight / SLIDE_H));
    };
    fit();
    if (typeof ResizeObserver === 'undefined') return;
    const observer = new ResizeObserver(fit);
    observer.observe(el);
    return () => observer.disconnect();
  }, []);
  return <div ref={area} className="flex size-full items-center justify-center overflow-hidden">
    <div className="shadow-2xl" style={{ width: SLIDE_W * scale, height: SLIDE_H * scale }}>
      <div style={{ width: SLIDE_W, height: SLIDE_H, transform: `scale(${scale})`, transformOrigin: 'top left' }}>{children}</div>
    </div>
  </div>;
}
