import { useEffect, useRef, useState, type FormEvent } from 'react';
import { Eye, EyeOff, Pause, Play, ShieldCheck } from 'lucide-react';
import { toast } from 'sonner';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import styles from './LoginPage.module.css';

const slides = [
  { imagePath: '/static/images/konkit-field-1.jpg', alt: 'Foto lapangan penerima program Konkit', label: 'Serah Terima', caption: 'Paket konkit, nomor penerima, dan dokumen lapangan terhubung dalam satu alur kerja.', focus: 'center 48%' },
  { imagePath: '/static/images/konkit-field-2.jpeg', alt: 'Foto kegiatan program Konkit kabupaten', label: 'Program Kabupaten', caption: 'Setiap kegiatan kabupaten dapat dipantau tanpa memisahkan database program.', focus: 'center 50%' },
  { imagePath: '/static/images/konkit-training.jpeg', alt: 'Foto pelatihan teknis Konkit', label: 'Pelatihan Teknis', caption: 'Data penerima, pemasangan, dan BAST disiapkan untuk operasional yang lebih tertib.', focus: 'center 54%' },
  { imagePath: '/static/images/konkit-aceh-socialization.jpg', alt: 'Sosialisasi teknis program Konkit di Aceh Tengah', label: 'Sosialisasi Teknis', caption: 'Petani menerima penjelasan program dan penggunaan paket konkit sebelum pelaksanaan lapangan.', focus: 'center 48%' },
  { imagePath: '/static/images/konkit-aceh-demonstration.jpg', alt: 'Demonstrasi penggunaan konkit bagi petani Aceh Tengah', label: 'Demonstrasi Lapangan', caption: 'Penggunaan mesin dan perangkat konkit diperagakan langsung bersama peserta program.', focus: 'center 52%' },
  { imagePath: '/static/images/konkit-aceh-recipient.jpg', alt: 'Penerima paket konkit di Aceh Tengah', label: 'Penerima Paket', caption: 'Penerima dan kelengkapan paket didokumentasikan sebagai bagian dari proses serah terima.', focus: 'center 52%' },
];

export function LoginPage() {
  const [submitting, setSubmitting] = useState(false);
  const [submissionError, setSubmissionError] = useState<string | null>(null);
  const [queryErrorVisible, setQueryErrorVisible] = useState(true);
  const [passwordVisible, setPasswordVisible] = useState(false);
  const [activeSlide, setActiveSlide] = useState(0);
  const [playing, setPlaying] = useState(() => !window.matchMedia?.('(prefers-reduced-motion: reduce)').matches);
  const noticeConsumed = useRef(false);
  const slide = slides[activeSlide];
  const errorCode = new URLSearchParams(window.location.search).get('error');
  const noticeCode = new URLSearchParams(window.location.search).get('notice');
  const queryErrorMessage = errorCode === 'throttled'
    ? 'Terlalu banyak percobaan. Tunggu 15 menit lalu coba lagi.'
    : errorCode === 'invalid' ? 'Email/username atau password tidak sesuai.' : null;
  const errorMessage = submissionError ?? (queryErrorVisible ? queryErrorMessage : null);

  useEffect(() => {
    if (!playing) return;
    const timer = window.setInterval(() => setActiveSlide((current) => (current + 1) % slides.length), 7000);
    return () => window.clearInterval(timer);
  }, [playing]);

  useEffect(() => {
    if (noticeConsumed.current || (noticeCode !== 'logged_out' && noticeCode !== 'session_expired')) return;
    noticeConsumed.current = true;

    const url = new URL(window.location.href);
    url.searchParams.delete('notice');
    window.history.replaceState(window.history.state, '', `${url.pathname}${url.search}${url.hash}`);

    if (noticeCode === 'logged_out') {
      toast.success('Anda telah keluar dengan aman.');
    } else {
      toast.warning('Sesi Anda telah berakhir. Silakan masuk kembali.');
    }
  }, [noticeCode]);

  function selectSlide(index: number) {
    setActiveSlide(index);
    setPlaying(false);
  }

  async function submitLogin(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    setSubmissionError(null);
    setQueryErrorVisible(false);
    let authenticated = false;

    try {
      const body = new URLSearchParams();
      new FormData(event.currentTarget).forEach((value, key) => {
        if (typeof value === 'string') body.append(key, value);
      });
      const response = await fetch(event.currentTarget.action, {
        method: 'POST',
        body,
        headers: { Accept: 'application/json' },
        credentials: 'same-origin',
      });
      const payload = await response.json().catch(() => null) as { data?: { authenticated?: boolean } } | null;

      if (response.ok && payload?.data?.authenticated === true) {
        authenticated = true;
        toast.success('Login berhasil. Mengarahkan ke dashboard…', { duration: 3000 });
        await new Promise((resolve) => window.setTimeout(resolve, 800));
        window.location.assign('/dashboard');
        return;
      }

      setSubmissionError(response.status === 429
        ? 'Terlalu banyak percobaan. Tunggu 15 menit lalu coba lagi.'
        : response.status === 401 || response.status === 400
          ? 'Email/username atau password tidak sesuai.'
          : 'Login tidak dapat diproses. Coba lagi beberapa saat.');
    } catch {
      setSubmissionError('Login tidak dapat diproses. Periksa koneksi lalu coba lagi.');
    } finally {
      if (!authenticated) setSubmitting(false);
    }
  }

  return <main className={styles.loginShell}>
    <section className={styles.fieldPanel} aria-label="Cerita lapangan Konkit">
      <div className={styles.photoFrame} aria-live={playing ? 'off' : 'polite'} aria-atomic="true">
        <img key={slide.imagePath} src={slide.imagePath} alt={slide.alt} style={{ objectPosition: slide.focus }} />
        <div className={styles.fieldMark}><span aria-hidden="true" /><p>Operasional lintas kabupaten</p></div>
        <div className={styles.slideCopy}>
          <p className={styles.slideIndex}>{String(activeSlide + 1).padStart(2, '0')} / {String(slides.length).padStart(2, '0')}</p>
          <h2>{slide.label}</h2>
          <p>{slide.caption}</p>
        </div>
      </div>

      <div className={styles.carouselControls} role="group" aria-label="Pilih foto lapangan">
        <button type="button" className={styles.playControl} onClick={() => setPlaying((current) => !current)} aria-label={playing ? 'Jeda pergantian foto' : 'Lanjutkan pergantian foto'}>
          {playing ? <Pause aria-hidden="true" /> : <Play aria-hidden="true" />}
        </button>
        <div className={styles.slideSelectors}>
          {slides.map((item, index) => <button
            type="button"
            key={item.imagePath}
            aria-label={`Tampilkan foto ${item.label}`}
            aria-current={index === activeSlide ? 'true' : undefined}
            onClick={() => selectSlide(index)}
          ><span /></button>)}
        </div>
      </div>
    </section>

    <section className={styles.formPanel} aria-label="Form login">
      <div className={styles.formWrap}>
        <header className={styles.brandHeader}>
          <div className={styles.brandLockup}>
            <img src="/static/images/logo-ergas.png" alt="Ergas" />
            <span aria-hidden="true" />
            <img src="/static/images/logo-ksm-cropped.png" alt="PT Kian Santang Mulitama Tbk" />
          </div>
          <p>Sistem Manajemen Program Konkit Gas</p>
        </header>

        <div className={styles.intro}>
          <h1>Selamat datang kembali</h1>
          <p>Masuk untuk mengelola penerima, distribusi, dokumentasi, dan laporan program.</p>
        </div>

        <form className={styles.loginForm} method="post" action="/login" aria-label="Masuk ke dashboard" onSubmit={submitLogin}>
          {errorMessage ? <Alert variant="destructive" id="login-error"><AlertDescription>{errorMessage}</AlertDescription></Alert> : null}
          <div className={styles.formField}>
            <Label htmlFor="identity">Email atau username</Label>
            <Input id="identity" name="identity" type="text" autoComplete="username" placeholder="Masukkan email atau username" maxLength={254} aria-describedby={errorMessage ? 'login-error' : undefined} required autoFocus />
          </div>
          <div className={styles.formField}>
            <Label htmlFor="password">Password</Label>
            <div className={styles.passwordField}>
              <Input id="password" name="password" type={passwordVisible ? 'text' : 'password'} autoComplete="current-password" placeholder="Masukkan password" maxLength={256} aria-describedby={errorMessage ? 'login-error' : undefined} required />
              <button type="button" onClick={() => setPasswordVisible((current) => !current)} aria-label={passwordVisible ? 'Sembunyikan kata sandi' : 'Tampilkan kata sandi'}>
                {passwordVisible ? <EyeOff aria-hidden="true" /> : <Eye aria-hidden="true" />}
              </button>
            </div>
          </div>
          <div className={styles.rememberRow}>
            <Checkbox id="remember" name="remember" />
            <div><Label htmlFor="remember" className="font-normal">Ingat perangkat ini</Label><p>Gunakan hanya di perangkat pribadi.</p></div>
          </div>
          <Button type="submit" size="lg" className={styles.submitButton} disabled={submitting}>{submitting ? 'Memproses...' : 'Masuk'}</Button>
        </form>

        <footer className={styles.securityNote}><ShieldCheck aria-hidden="true" /><p>Akses terbatas untuk pengguna internal yang berwenang.</p></footer>
      </div>
    </section>
  </main>;
}
