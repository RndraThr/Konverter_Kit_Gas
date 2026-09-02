import { useState } from 'react';
import styles from './LoginPage.module.css';

const slides = [
  {
    imagePath: '/static/images/konkit-field-1.jpg',
    alt: 'Foto lapangan penerima program Konkit',
    label: 'Serah Terima',
    caption: 'Paket konkit, nomor penerima, dan dokumen lapangan terhubung dalam satu alur kerja.',
    focus: 'center 48%',
  },
  {
    imagePath: '/static/images/konkit-field-2.jpeg',
    alt: 'Foto kegiatan program Konkit kabupaten',
    label: 'Program Kabupaten',
    caption: 'Setiap kegiatan kabupaten dapat dipantau tanpa memisahkan database program.',
    focus: 'center 50%',
  },
  {
    imagePath: '/static/images/konkit-training.jpeg',
    alt: 'Foto pelatihan teknis Konkit',
    label: 'Pelatihan Teknis',
    caption: 'Data penerima, pemasangan, dan BAST disiapkan untuk operasional yang lebih tertib.',
    focus: 'center 54%',
  },
  {
    imagePath: '/static/images/konkit-aceh-socialization.jpg',
    alt: 'Sosialisasi teknis program Konkit di Aceh Tengah',
    label: 'Sosialisasi Teknis',
    caption: 'Petani menerima penjelasan program dan penggunaan paket konkit sebelum pelaksanaan lapangan.',
    focus: 'center 48%',
  },
  {
    imagePath: '/static/images/konkit-aceh-demonstration.jpg',
    alt: 'Demonstrasi penggunaan konkit bagi petani Aceh Tengah',
    label: 'Demonstrasi Lapangan',
    caption: 'Penggunaan mesin dan perangkat konkit diperagakan langsung bersama peserta program.',
    focus: 'center 52%',
  },
  {
    imagePath: '/static/images/konkit-aceh-recipient.jpg',
    alt: 'Penerima paket konkit di Aceh Tengah',
    label: 'Penerima Paket',
    caption: 'Penerima dan kelengkapan paket didokumentasikan sebagai bagian dari proses serah terima.',
    focus: 'center 52%',
  },
];

export function LoginPage() {
  const [submitting, setSubmitting] = useState(false);
  const errorCode = new URLSearchParams(window.location.search).get('error');
  const errorMessage = errorCode === 'throttled'
    ? 'Terlalu banyak percobaan. Tunggu 15 menit lalu coba lagi.'
    : errorCode === 'invalid'
      ? 'Email/username atau password tidak sesuai.'
      : null;

  return (
    <main className={styles.loginShell}>
      <section className={styles.fieldPanel} aria-label="Foto lapangan Konkit">
        <div className={styles.carousel} aria-roledescription="carousel">
          <div className={styles.carouselTrack}>
            {slides.map((slide, index) => (
              <article
                className={styles.slide}
                key={slide.imagePath}
                style={{ animationDelay: `${index * 6}s` }}
              >
                <img src={slide.imagePath} alt={slide.alt} style={{ objectPosition: slide.focus }} />
                <div className={styles.slideCopy}>
                  <span>{slide.label}</span>
                  <p>{slide.caption}</p>
                </div>
              </article>
            ))}
          </div>
        </div>

        <div
          className={styles.carouselRail}
          aria-hidden="true"
          style={{ gridTemplateColumns: `repeat(${slides.length}, 1fr)` }}
        >
          {slides.map((slide, index) => (
            <span key={slide.imagePath} style={{ animationDelay: `${index * 6}s` }} />
          ))}
        </div>

        <div className={styles.fieldPanelFooter}>
          <span className={styles.statusDot} aria-hidden="true" />
          <span>Operasional lintas kabupaten</span>
        </div>
      </section>

      <section className={styles.loginPanel} aria-label="Form login">
        <div className={styles.loginContent}>
          <header className={styles.productIdentity}>
            <span aria-hidden="true" />
            <p>Sistem Konkit Gas</p>
          </header>

          <div className={styles.brandLockup}>
            <div className={styles.brandMarks}>
              <img src="/static/images/logo-ergas.png" alt="Ergas" className={`${styles.brandLogo} ${styles.ergasLogo}`} />
              <span className={styles.brandDivider} aria-hidden="true" />
              <img
                src="/static/images/logo-ksm-cropped.png"
                alt="PT Kian Santang Mulitama Tbk"
                className={`${styles.brandLogo} ${styles.ksmLogo}`}
              />
            </div>
            <p>Manajemen program konversi BBM ke BBG</p>
          </div>

          <div className={styles.formHeading}>
            <h1>Masuk ke dashboard</h1>
            <p>Kelola jadwal kabupaten, data penerima, dokumen BAST, dan laporan program dalam satu ruang kerja.</p>
          </div>

          <form
            className={styles.loginForm}
            method="post"
            action="/login"
            onSubmit={() => setSubmitting(true)}
          >
            {errorMessage && (
              <p id="login-error" className={styles.formError} role="alert">
                {errorMessage}
              </p>
            )}

            <label htmlFor="identity">Email atau username</label>
            <input
              id="identity"
              name="identity"
              type="text"
              autoComplete="username"
              placeholder="admin@konkit.local"
              maxLength={254}
              aria-describedby={errorMessage ? 'login-error' : undefined}
              required
            />

            <div className={styles.passwordRow}>
              <label htmlFor="password">Password</label>
            </div>
            <input
              id="password"
              name="password"
              type="password"
              autoComplete="current-password"
              placeholder="Masukkan password"
              maxLength={256}
              aria-describedby={errorMessage ? 'login-error' : undefined}
              required
            />

            <label className={styles.rememberOption}>
              <input type="checkbox" name="remember" />
              <span>Ingat perangkat ini</span>
            </label>

            <button type="submit" disabled={submitting}>
              {submitting ? 'Memproses...' : 'Masuk'}
            </button>
          </form>

          <p className={styles.securityNote}>Akses hanya untuk pengguna internal yang berwenang.</p>
        </div>
      </section>
    </main>
  );
}
