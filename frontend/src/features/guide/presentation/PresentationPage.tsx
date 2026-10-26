import { ChevronLeft, ChevronRight, LayoutGrid, Maximize, Minimize, X } from 'lucide-react';
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import styles from './Presentation.module.css';
import { slides } from './slides';

const STAGE_WIDTH = 1280;
const STAGE_HEIGHT = 720;

/** Slide number from the URL hash (#3), 0-based; the first slide otherwise. */
function slideFromHash(hash: string) {
  const n = Number.parseInt(hash.replace('#', ''), 10);
  return Number.isFinite(n) && n >= 1 && n <= slides.length ? n - 1 : 0;
}

/**
 * Presentasi alur Konkit (/panduan/presentasi): slide 16:9 yang diskalakan
 * ke layar, untuk dipresentasikan di depan tim atau dibaca sendiri.
 * Navigasi: ← → / spasi / PageUp PageDown, Home/End, O untuk ikhtisar,
 * F untuk layar penuh, Esc untuk kembali.
 */
export function PresentationPage() {
  const navigate = useNavigate();
  const location = useLocation();
  const [index, setIndex] = useState(() => slideFromHash(location.hash));
  const [direction, setDirection] = useState<'forward' | 'back'>('forward');
  const [overview, setOverview] = useState(false);
  const [fullscreen, setFullscreen] = useState(false);
  const [scale, setScale] = useState(1);
  const viewport = useRef<HTMLDivElement>(null);
  const shell = useRef<HTMLDivElement>(null);

  const go = useCallback((next: number) => {
    const target = Math.max(0, Math.min(slides.length - 1, next));
    if (target === index) return;
    setDirection(target < index ? 'back' : 'forward');
    setIndex(target);
  }, [index]);

  // The slide number stays in the URL (shareable, survives a reload).
  useEffect(() => {
    window.history.replaceState(null, '', `#${index + 1}`);
    document.title = `${slides[index].title} · Presentasi Konkit`;
  }, [index]);

  useLayoutEffect(() => {
    const element = viewport.current;
    if (!element) return;
    const fit = () => {
      const padding = 36;
      const width = element.clientWidth - padding;
      const height = element.clientHeight - padding;
      setScale(Math.max(0.2, Math.min(width / STAGE_WIDTH, height / STAGE_HEIGHT)));
    };
    fit();
    const observer = new ResizeObserver(fit);
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    const sync = () => setFullscreen(Boolean(document.fullscreenElement));
    document.addEventListener('fullscreenchange', sync);
    return () => document.removeEventListener('fullscreenchange', sync);
  }, []);

  const toggleFullscreen = useCallback(() => {
    if (document.fullscreenElement) void document.exitFullscreen();
    else void shell.current?.requestFullscreen?.();
  }, []);

  const close = useCallback(() => {
    if (document.fullscreenElement) void document.exitFullscreen();
    navigate('/panduan');
  }, [navigate]);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.metaKey || event.ctrlKey || event.altKey) return;
      switch (event.key) {
        case 'ArrowRight':
        case 'PageDown':
        case ' ':
          event.preventDefault();
          go(index + 1);
          break;
        case 'ArrowLeft':
        case 'PageUp':
          event.preventDefault();
          go(index - 1);
          break;
        case 'Home':
          go(0);
          break;
        case 'End':
          go(slides.length - 1);
          break;
        case 'o':
        case 'O':
          setOverview((value) => !value);
          break;
        case 'f':
        case 'F':
          toggleFullscreen();
          break;
        case 'Escape':
          if (overview) setOverview(false);
          else if (!document.fullscreenElement) close();
          break;
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [close, go, index, overview, toggleFullscreen]);

  // Swipe on touch screens.
  const touchStart = useRef<number | null>(null);
  const slide = slides[index];

  return <div
    ref={shell}
    className={styles.shell}
    role="region"
    aria-roledescription="presentasi"
    aria-label="Presentasi alur Konkit"
    onTouchStart={(event) => { touchStart.current = event.touches[0]?.clientX ?? null; }}
    onTouchEnd={(event) => {
      const start = touchStart.current;
      const end = event.changedTouches[0]?.clientX;
      touchStart.current = null;
      if (start == null || end == null || Math.abs(end - start) < 50) return;
      go(end < start ? index + 1 : index - 1);
    }}
  >
    <div ref={viewport} className={styles.viewport}>
      <div className={styles.stage} style={{ transform: `scale(${scale})` }} aria-live="polite">
        <div key={slide.id} className={styles.slide} data-direction={direction} role="group" aria-roledescription="slide" aria-label={`${index + 1} dari ${slides.length}: ${slide.title}`}>
          {slide.render()}
        </div>
        {/* Click the left/right third of the slide to move. */}
        <button type="button" aria-label="Slide sebelumnya" className="absolute inset-y-0 left-0 w-[12%] cursor-w-resize opacity-0" onClick={() => go(index - 1)} tabIndex={-1} />
        <button type="button" aria-label="Slide berikutnya" className="absolute inset-y-0 right-0 w-[12%] cursor-e-resize opacity-0" onClick={() => go(index + 1)} tabIndex={-1} />
      </div>

      {overview && <div className={styles.overview} role="dialog" aria-label="Semua slide">
        <div className="mx-auto mb-5 flex max-w-[1200px] items-center justify-between">
          <h2 className="text-lg font-bold">Semua slide</h2>
          <button type="button" className={styles.controlButton} aria-label="Tutup ikhtisar" onClick={() => setOverview(false)}><X className="size-5" /></button>
        </div>
        <div className={styles.overviewGrid}>
          {slides.map((item, i) => <button key={item.id} type="button" className={styles.thumb} aria-current={i === index} onClick={() => { go(i); setOverview(false); }}>
            <span className="flex items-center gap-3"><span className={styles.thumbNumber}>{i + 1}</span><span className="text-sm font-semibold">{item.title}</span></span>
          </button>)}
        </div>
      </div>}
    </div>

    <div className={styles.controls}>
      <button type="button" className={styles.controlButton} aria-label="Tutup presentasi" title="Kembali ke Panduan (Esc)" onClick={close}><X className="size-5" /></button>
      <button type="button" className={styles.controlButton} aria-label="Semua slide" title="Semua slide (O)" aria-pressed={overview} onClick={() => setOverview((value) => !value)}><LayoutGrid className="size-5" /></button>
      <button type="button" className={styles.controlButton} aria-label="Slide sebelumnya" title="Sebelumnya (←)" disabled={index === 0} onClick={() => go(index - 1)}><ChevronLeft className="size-5" /></button>
      <span className={styles.counter} aria-hidden="true">{index + 1} / {slides.length}</span>
      <button type="button" className={styles.controlButton} aria-label="Slide berikutnya" title="Berikutnya (→)" disabled={index === slides.length - 1} onClick={() => go(index + 1)}><ChevronRight className="size-5" /></button>
      <div className={styles.progress} aria-hidden="true"><div className={styles.progressFill} style={{ width: `${((index + 1) / slides.length) * 100}%` }} /></div>
      <span className="hidden max-w-[240px] truncate text-sm text-[#cfe3d4] md:inline">{slide.title}</span>
      <button type="button" className={styles.controlButton} aria-label={fullscreen ? 'Keluar layar penuh' : 'Layar penuh'} title="Layar penuh (F)" onClick={toggleFullscreen}>{fullscreen ? <Minimize className="size-5" /> : <Maximize className="size-5" />}</button>
    </div>
  </div>;
}
