import { useState } from 'react';
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
  const errorCode = new URLSearchParams(window.location.search).get('error');
  const errorMessage = errorCode === 'throttled'
    ? 'Terlalu banyak percobaan. Tunggu 15 menit lalu coba lagi.'
    : errorCode === 'invalid' ? 'Email/username atau password tidak sesuai.' : null;

  return <main className={styles.loginShell}>
    <section className={styles.fieldPanel} aria-label="Foto lapangan Konkit">
      <div className={styles.carousel} aria-roledescription="carousel">
        <div className={styles.carouselTrack}>
          {slides.map((slide, index) => <article className={styles.slide} key={slide.imagePath} style={{ animationDelay: `${index * 6}s` }}>
            <img src={slide.imagePath} alt={slide.alt} style={{ objectPosition: slide.focus }} />
            <div className={styles.slideCopy}><span>{slide.label}</span><p>{slide.caption}</p></div>
          </article>)}
        </div>
      </div>
      <div className={styles.carouselRail} aria-hidden="true" style={{ gridTemplateColumns: `repeat(${slides.length}, 1fr)` }}>
        {slides.map((slide, index) => <span key={slide.imagePath} style={{ animationDelay: `${index * 6}s` }} />)}
      </div>
      <div className={styles.fieldPanelFooter}><span className={styles.statusDot} aria-hidden="true" /><span>Operasional lintas kabupaten</span></div>
    </section>

    <section className="flex min-w-0 bg-card px-5 py-8 sm:px-10 sm:py-12 lg:px-14" aria-label="Form login">
      <div className="my-auto w-full max-w-md space-y-7">
        <div className="flex items-center gap-3 text-sm font-semibold text-foreground"><span className="h-px w-7 bg-primary" aria-hidden="true" />Sistem Konkit Gas</div>
        <div className="space-y-3">
          <div className="grid grid-cols-[minmax(0,.92fr)_1px_minmax(0,1.08fr)] items-center gap-4 sm:gap-5">
            <img src="/static/images/logo-ergas.png" alt="Ergas" className="w-full max-w-47 justify-self-end" />
            <span className="h-10 bg-border" aria-hidden="true" />
            <img src="/static/images/logo-ksm-cropped.png" alt="PT Kian Santang Mulitama Tbk" className="h-12 w-full max-w-61 justify-self-start object-contain object-left" />
          </div>
          <p className="text-sm font-medium text-secondary-foreground">Manajemen program konversi BBM ke BBG</p>
        </div>
        <div className="space-y-3"><h1 className="text-balance text-3xl font-semibold tracking-tight text-foreground sm:text-4xl">Masuk ke dashboard</h1><p className="max-w-prose text-sm leading-6 text-muted-foreground">Kelola jadwal kabupaten, data penerima, dokumen BAST, dan laporan program dalam satu ruang kerja.</p></div>
        <form className="grid gap-4" method="post" action="/login" aria-label="Masuk ke dashboard" onSubmit={() => setSubmitting(true)}>
          {errorMessage ? <Alert variant="destructive" id="login-error"><AlertDescription>{errorMessage}</AlertDescription></Alert> : null}
          <div className="grid gap-2"><Label htmlFor="identity">Email atau username</Label><Input id="identity" name="identity" type="text" autoComplete="username" placeholder="admin@konkit.local" maxLength={254} aria-describedby={errorMessage ? 'login-error' : undefined} required /></div>
          <div className="grid gap-2"><Label htmlFor="password">Password</Label><Input id="password" name="password" type="password" autoComplete="current-password" placeholder="Masukkan password" maxLength={256} aria-describedby={errorMessage ? 'login-error' : undefined} required /></div>
          <div className="flex items-center gap-3"><Checkbox id="remember" name="remember" /><Label htmlFor="remember" className="font-normal text-muted-foreground">Ingat perangkat ini</Label></div>
          <Button type="submit" size="lg" className="w-full" disabled={submitting}>{submitting ? 'Memproses...' : 'Masuk'}</Button>
        </form>
        <p className="text-sm text-muted-foreground">Akses hanya untuk pengguna internal yang berwenang.</p>
      </div>
    </section>
  </main>;
}
