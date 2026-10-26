import {
  ArrowRight, BadgeCheck, Building2, Camera, CheckCircle2, ClipboardCheck, CloudOff, FileCheck2, FileSpreadsheet, Files,
  Fingerprint, Images, KeyRound, LogIn, MapPinned, PackageCheck, PenLine, Route, ScanBarcode, ShieldCheck, Smartphone,
  Sparkles, UserRoundCheck, Users, Workflow, type LucideIcon,
} from 'lucide-react';
import type { CSSProperties, ReactNode } from 'react';
import shots from '../guideShots.json';
import { glossary, guideIntro, guideSteps, type GuideStep } from '../guideContent';
import styles from './Presentation.module.css';

/**
 * Isi presentasi alur Konkit. Tahap alur, screenshot bertanda, dan istilah
 * diambil dari Panduan (guideContent / guideShots.json) agar keduanya tidak
 * pernah berbeda; slide ini hanya menyajikannya dengan tampilan presentasi.
 */

type Mark = { n: number; x: number; y: number; w: number; h: number };
type Shot = { src: string; width: number; height: number; marks: Mark[] };
const manifest = shots as Record<string, Shot>;

export type Slide = { id: string; title: string; render: () => ReactNode };

const reveal = (i: number) => ({ 'data-reveal': true, style: { '--i': i } as CSSProperties });

const stepIcons: Record<string, LucideIcon> = {
  persiapan: Building2,
  akses: ShieldCheck,
  dcp3: FileSpreadsheet,
  penerima: UserRoundCheck,
  distribusi: PackageCheck,
  kegiatan: Images,
  'berita-acara': Files,
  laporan: MapPinned,
};

/** Poin singkat per tahap untuk presentasi (detail lengkap ada di Panduan). */
const stepPoints: Record<string, string[]> = {
  persiapan: [
    'Kabupaten & kode dokumen, program Petani/Nelayan per tahun',
    'Zona tender dan No. PO per zona',
    'Template paket (mesin, selang, konkit, TKDN) dan foto wajib',
    'Jadwal per kabupaten: dipilih di semua menu lain',
  ],
  akses: [
    'Role berisi hak akses sesuai tugas tim',
    'Setiap role dibatasi ke kabupaten tim',
    'Akun pengguna dengan role & password awal',
  ],
  dcp3: [
    'Unggah Excel DCP3 ke jadwal kabupaten',
    'Cocokkan kolom: Nama, NIK, kartu tani, alamat, HP',
    'Tinjau baris valid, peringatan, konflik, lalu import',
  ],
  penerima: [
    'Cari dan filter penerima lintas jadwal',
    'Periksa NIK, nama, alamat sebelum hari distribusi',
    'Tambah penerima manual bila belum ada di DCP3',
  ],
  distribusi: [
    'POS Mesin: nomor bagi baru, scan serial, foto mesin',
    'POS Dokumen: hubungkan penerima, tanggal, peralatan, tanda tangan',
    'POS Penyerahan: foto serah terima, Selesaikan distribusi',
    'Revisi tercatat dengan alasan',
  ],
  kegiatan: [
    'Ceremony & sosialisasi, pelatihan teknis, rakor',
    'Training 10% / 100% dan unloading barang',
    'Foto & video dari kamera atau galeri, per kabupaten',
  ],
  'berita-acara': [
    'DP3, BA Perorangan, Rekap Harian per tanggal distribusi',
    'Penandatangan digital atau tanda tangan basah',
    'Preview langsung, finalisasi, PDF tersinkron ke Drive',
  ],
  laporan: [
    'Progres distribusi per kabupaten di peta',
    'Laporan per penerima, export Excel / PDF',
    'Pantau dari web atau HP (Monitoring)',
  ],
};

/** Pelaksana singkat untuk kartu alur (versi lengkap di slide tiap tahap). */
const shortDoers: Record<string, string> = {
  persiapan: 'Admin pusat',
  akses: 'Admin pusat',
  dcp3: 'Operator data',
  penerima: 'Operator data, koordinator',
  distribusi: 'Petugas POS Mesin, Dokumen, Penyerahan',
  kegiatan: 'Tim dokumentasi',
  'berita-acara': 'Admin BA, koordinator',
  laporan: 'Koordinator, pimpinan',
};

const phases = [
  { label: 'Persiapan di kantor', color: '#c89b39', steps: ['persiapan', 'akses', 'dcp3', 'penerima'] },
  { label: 'Di lapangan', color: '#4fad42', steps: ['distribusi', 'kegiatan'] },
  { label: 'Dokumen & laporan', color: '#2f7d4a', steps: ['berita-acara', 'laporan'] },
];

function ShotView({ id, maxHeight }: { id: string; maxHeight: number }) {
  const shot = manifest[id];
  if (!shot) return null;
  const width = Math.min(shot.width, (maxHeight * shot.width) / shot.height, 640);
  return <div className={styles.shotFrame} style={{ width }}>
    <img src={shot.src} alt="" width={shot.width} height={shot.height} className="block h-auto w-full rounded-[13px]" />
    {shot.marks.map((mark, index) => <span key={`${mark.n}-${index}`} className={styles.mark} style={{ left: `${mark.x}%`, top: `${mark.y}%`, width: `${mark.w}%`, height: `${mark.h}%` }}>
      <span className={styles.markNumber}>{mark.n}</span>
    </span>)}
  </div>;
}

function SlideFooter({ label }: { label: string }) {
  return <div className="absolute right-10 bottom-6 left-10 flex items-center justify-between text-[13px] font-semibold text-[#8a978d]">
    <span className="flex items-center gap-2"><img src="/static/images/favicon.svg" alt="" className="size-5" />Konkit · Konverter Kit Gas</span>
    <span>{label}</span>
  </div>;
}

function CoverSlide() {
  return <>
    <div className={styles.cover} style={{ backgroundImage: 'url(/static/images/konkit-field-1.jpg)' }} />
    <div className={styles.coverShade} />
    <div className="relative flex h-full flex-col justify-between p-16 text-white">
      <div {...reveal(0)} className="flex items-center gap-4">
        <img src="/static/images/logo-ksm-transparent.png" alt="" className="h-14 w-auto rounded-lg bg-white/90 p-1.5" />
        <img src="/static/images/logo-ergas.png" alt="" className="h-14 w-auto rounded-lg bg-white/90 p-1.5" />
      </div>
      <div className="max-w-[720px]">
        <p {...reveal(1)} className="mb-4 inline-flex items-center gap-2 rounded-full bg-white/12 px-4 py-1.5 text-sm font-bold tracking-[0.18em] text-[#d7b764] uppercase"><Sparkles className="size-4" />Alur kerja aplikasi</p>
        <h1 {...reveal(2)} className="text-[76px] leading-[1.02] font-extrabold tracking-tight">Konkit</h1>
        <p {...reveal(3)} className="mt-4 text-[26px] leading-snug font-semibold text-[#eef6f0]">Penyaluran paket konverter kit gas, dari persiapan program sampai Berita Acara.</p>
        <div {...reveal(4)} className="mt-8 flex gap-3">
          <span className={`${styles.glass} flex items-center gap-2 px-4 py-2.5 text-base font-semibold`}><Workflow className="size-5 text-[#d7b764]" />Website untuk kantor</span>
          <span className={`${styles.glass} flex items-center gap-2 px-4 py-2.5 text-base font-semibold`}><Smartphone className="size-5 text-[#d7b764]" />Aplikasi HP untuk lapangan</span>
        </div>
      </div>
      <p {...reveal(5)} className="text-sm text-white/70">Tekan → atau spasi untuk lanjut</p>
    </div>
  </>;
}

function PurposeSlide() {
  const pillars: [LucideIcon, string, string][] = [
    [Fingerprint, 'Satu data penerima', 'NIK, kartu tani, dan alamat dari DCP3 dipakai sampai BA.'],
    [Camera, 'Bukti foto bertanda', 'Setiap pos punya foto wajib dengan cap waktu & lokasi.'],
    [CloudOff, 'Tetap jalan offline', 'Petugas lapangan bekerja tanpa sinyal; data terkirim otomatis.'],
    [FileCheck2, 'BA otomatis', 'Berita Acara dibuat dari data distribusi, siap tanda tangan.'],
  ];
  return <div className="flex h-full flex-col justify-center px-16 pt-10 pb-24">
    <p {...reveal(0)} className={styles.eyebrow}><Sparkles className="size-4" />Pengantar</p>
    <h2 {...reveal(1)} className={`${styles.title} mt-3`}>{guideIntro.title}</h2>
    <p {...reveal(2)} className={`${styles.lead} mt-5 max-w-[1040px]`}>{guideIntro.body}</p>
    <div className="mt-12 grid grid-cols-4 gap-5">
      {pillars.map(([Icon, title, text], index) => <div key={title} {...reveal(3 + index)} className={`${styles.card} p-6`}>
        <span className="grid size-12 place-items-center rounded-2xl bg-[#edf3eb] text-[#2f7d4a]"><Icon className="size-6" /></span>
        <h3 className="mt-4 text-[19px] font-extrabold text-[#173d2b]">{title}</h3>
        <p className="mt-1.5 text-[15px] leading-relaxed text-[#5d6b61]">{text}</p>
      </div>)}
    </div>
    <SlideFooter label="Pengantar" />
  </div>;
}

function FlowSlide() {
  return <div className="flex h-full flex-col p-16 pb-20">
    <p {...reveal(0)} className={styles.eyebrow}><Route className="size-4" />Alur besar</p>
    <h2 {...reveal(1)} className={`${styles.title} mt-3`}>Delapan tahap, tiga fase</h2>
    <p {...reveal(2)} className={`${styles.lead} mt-3`}>Setiap tahap memakai hasil tahap sebelumnya. Jadwal kabupaten menjadi pegangan di semua menu.</p>
    {/* Every card has the same width: 2 columns for the office phase, one
        each for the field and document phases. */}
    <div className="mt-8 grid flex-1 grid-cols-[2fr_1fr_1fr] gap-6">
      {phases.map((phase, p) => <div key={phase.label} {...reveal(3 + p)} className="grid min-h-0 grid-rows-[auto_1fr] gap-3">
        <div className="flex items-center gap-2 text-[13px] font-extrabold tracking-wide uppercase" style={{ color: phase.color }}>
          <span className="h-1.5 w-7 rounded-full" style={{ background: phase.color }} />{phase.label}
        </div>
        <div className={`grid auto-rows-fr gap-3 ${phase.steps.length > 2 ? 'grid-cols-2' : 'grid-cols-1'}`}>
          {phase.steps.map((id) => {
            const step = guideSteps.find((item) => item.id === id)!;
            const Icon = stepIcons[id] ?? Workflow;
            return <div key={id} className={`${styles.card} relative flex items-center gap-4 px-5 py-4`}>
              <span className="grid size-12 shrink-0 place-items-center rounded-xl text-white" style={{ background: phase.color }}><Icon className="size-6" /></span>
              <span className="min-w-0 flex-1">
                <span className="block text-[17px] leading-snug font-extrabold text-[#173d2b]">{step.title}</span>
                <span className="mt-0.5 block text-[13px] leading-snug text-[#6b786f]">{shortDoers[id] ?? step.doers}</span>
              </span>
              <span className="absolute top-2.5 right-3.5 text-[13px] font-extrabold text-[#b9c4bb]">{guideSteps.indexOf(step) + 1}</span>
            </div>;
          })}
        </div>
      </div>)}
    </div>
    <div {...reveal(7)} className="mt-6 flex items-center gap-3 text-[15px] font-semibold text-[#435248]">
      <span className={`${styles.chip} bg-[#f5ecd8] text-[#76591a]`}>Kantor</span><ArrowRight className="size-4" />
      <span className={`${styles.chip} bg-[#edf3eb] text-[#28733a]`}>Lapangan</span><ArrowRight className="size-4" />
      <span className={`${styles.chip} bg-[#2f7d4a] text-white`}>Dokumen resmi & laporan</span>
    </div>
    <SlideFooter label="Alur besar" />
  </div>;
}

function StepSlide({ step }: { step: GuideStep }) {
  const index = guideSteps.indexOf(step);
  const Icon = stepIcons[step.id] ?? Workflow;
  const phase = phases.find((item) => item.steps.includes(step.id))!;
  const shot = step.shots?.[0];
  return <div className="grid h-full grid-cols-[1fr_auto] items-center gap-12 px-14 pt-10 pb-24">
    <div className="flex min-w-0 flex-col">
      <div {...reveal(0)} className="flex items-start gap-4">
        <span className="mt-1 grid size-14 shrink-0 place-items-center rounded-2xl text-white shadow-lg" style={{ background: phase.color }}><Icon className="size-7" /></span>
        <div>
          <p className="text-[13px] font-extrabold tracking-[0.14em] uppercase" style={{ color: phase.color }}>Tahap {index + 1} · {phase.label}</p>
          <h2 className="text-[38px] leading-tight font-extrabold tracking-tight text-[#173d2b]">{step.title}</h2>
        </div>
      </div>
      <p {...reveal(1)} className="mt-5 text-[19px] leading-relaxed text-[#435248]">{step.summary}</p>
      <ul className="mt-6 grid gap-3">
        {(stepPoints[step.id] ?? []).map((point, i) => <li key={point} {...reveal(2 + i)} className="flex items-start gap-3 text-[17px] font-medium text-[#24352a]">
          <CheckCircle2 className="mt-0.5 size-5 shrink-0 text-[#4fad42]" />{point}
        </li>)}
      </ul>
      <div className="mt-8 grid gap-3">
        <div {...reveal(7)} className="flex items-center gap-2 text-[15px]"><Users className="size-4 text-[#2f7d4a]" /><span className="text-[#6b786f]">Dikerjakan oleh</span><b className="text-[#173d2b]">{step.doers}</b></div>
        <div {...reveal(8)} className="flex items-start gap-2 rounded-xl bg-[#edf3eb] px-4 py-3 text-[15px] text-[#28733a]"><BadgeCheck className="mt-0.5 size-5 shrink-0" /><span><b>Selesai bila:</b> {step.doneWhen}</span></div>
      </div>
    </div>
    {shot && <div {...reveal(3)} className="grid content-center gap-5 p-3">
      <ShotView id={shot.id} maxHeight={430} />
      <ol className="grid max-w-[560px] gap-1.5">
        {shot.legend.slice(0, 4).map((text, i) => <li key={text} className="flex items-start gap-2 text-[13.5px] leading-snug text-[#435248]">
          <span className="grid size-5 shrink-0 place-items-center rounded-full bg-[#2f7d4a] text-[11px] font-bold text-white">{i + 1}</span>{text}
        </li>)}
      </ol>
    </div>}
    <SlideFooter label={`Tahap ${index + 1} dari ${guideSteps.length}`} />
  </div>;
}

function MobileSlide() {
  const pos: [LucideIcon, string, string][] = [
    [ScanBarcode, 'POS Mesin', 'Nomor bagi baru, scan serial, foto mesin'],
    [ClipboardCheck, 'POS Dokumen', 'Hubungkan penerima, tanggal, tanda tangan'],
    [PackageCheck, 'POS Penyerahan', 'Foto serah terima, Selesaikan distribusi'],
  ];
  const features: [LucideIcon, string, string][] = [
    [CloudOff, 'Offline dulu', 'Semua tersimpan di HP, terkirim otomatis saat ada sinyal.'],
    [Camera, 'Kamera watermark', 'Logo tender, waktu, dan lokasi tercetak di setiap foto.'],
    [PenLine, 'Tanda tangan di HP', 'Penerima tanda tangan di layar; langsung masuk BA.'],
    [Files, 'BA dari HP', 'Preview, penandatangan, dan finalisasi Berita Acara.'],
  ];
  return <div className="grid h-full grid-cols-[auto_1fr] items-center gap-14 px-14 pt-10 pb-24">
    <div {...reveal(0)} className="grid place-items-center">
      <div className={styles.phone}>
        <div className={styles.phoneScreen}>
          <div className="bg-gradient-to-br from-[#173d2b] to-[#245139] px-4 pt-6 pb-5 text-white">
            <p className="text-[11px] font-bold tracking-widest text-[#d7b764] uppercase">Nomor bagi #128</p>
            <p className="mt-1 text-[17px] font-extrabold">Penerima Contoh</p>
            <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-white/15"><div className="h-full w-2/3 rounded-full bg-[#4fad42]" /></div>
          </div>
          <div className="grid gap-2.5 p-3">
            {pos.map(([Icon, title, text], i) => <div key={title} className="flex items-center gap-2.5 rounded-xl border border-[#dde4da] bg-white p-2.5">
              <span className={`grid size-8 shrink-0 place-items-center rounded-lg ${i < 2 ? 'bg-[#2f7d4a] text-white' : 'bg-[#f5ecd8] text-[#76591a]'}`}><Icon className="size-4" /></span>
              <span className="min-w-0"><b className="block text-[12.5px] text-[#173d2b]">{title}</b><span className="block text-[10.5px] leading-tight text-[#6b786f]">{text}</span></span>
              {i < 2 && <CheckCircle2 className="ml-auto size-4 shrink-0 text-[#4fad42]" />}
            </div>)}
            <div className="mt-1 rounded-xl bg-[#2f7d4a] py-2.5 text-center text-[12.5px] font-bold text-white">Selesaikan distribusi</div>
          </div>
        </div>
      </div>
    </div>
    <div className="flex min-w-0 flex-col justify-center">
      <p {...reveal(1)} className={styles.eyebrow}><Smartphone className="size-4" />Aplikasi HP</p>
      <h2 {...reveal(2)} className={`${styles.title} mt-3`}>Di lapangan, cukup dengan HP</h2>
      <p {...reveal(3)} className={`${styles.lead} mt-4`}>Satu paket melewati tiga pos berurutan. Setiap pos dikerjakan petugasnya, dan kemajuannya terlihat oleh semua.</p>
      <div className="mt-5 flex items-center gap-2" {...reveal(4)}>
        {pos.map(([, title], i) => <span key={title} className="flex items-center gap-2">
          <span className={`${styles.chip} bg-[#173d2b] text-white`}>{title}</span>{i < pos.length - 1 && <ArrowRight className="size-4 text-[#8a978d]" />}
        </span>)}
      </div>
      <div className="mt-10 grid grid-cols-2 gap-4">
        {features.map(([Icon, title, text], i) => <div key={title} {...reveal(5 + i)} className={`${styles.card} flex gap-3 p-4`}>
          <span className="grid size-10 shrink-0 place-items-center rounded-xl bg-[#edf3eb] text-[#2f7d4a]"><Icon className="size-5" /></span>
          <span><b className="block text-[16px] text-[#173d2b]">{title}</b><span className="text-[14px] leading-snug text-[#5d6b61]">{text}</span></span>
        </div>)}
      </div>
    </div>
    <SlideFooter label="Aplikasi HP" />
  </div>;
}

function BeritaAcaraSlide() {
  const groups: [string, string, string[]][] = [
    ['Persiapan', '#c89b39', ['Rakorda', 'BA Pemeriksaan']],
    ['Sosialisasi', '#8c7a2e', ['Sosialisasi', 'Training 10%']],
    ['Distribusi', '#4fad42', ['DP3', 'BA Perorangan', 'Rekap Harian', 'Training 100%']],
    ['Penutupan', '#2f7d4a', ['Closing Titik Serah', 'Closing Kabupaten', 'Servis Berkala', 'TKDN']],
  ];
  return <div className="flex h-full flex-col justify-center px-16 pt-10 pb-24">
    <p {...reveal(0)} className={styles.eyebrow}><Files className="size-4" />Berita Acara</p>
    <h2 {...reveal(1)} className={`${styles.title} mt-3`}>Semua dokumen resmi dalam satu tempat</h2>
    <p {...reveal(2)} className={`${styles.lead} mt-3 max-w-[1000px]`}>Dibuat dari data distribusi yang sudah tercatat: preview langsung, penandatangan digital atau basah, lalu finalisasi menjadi PDF yang tersinkron.</p>
    <div className="mt-9 grid grid-cols-4 gap-5">
      {groups.map(([label, color, docs], i) => <div key={label} {...reveal(3 + i)} className={`${styles.card} overflow-hidden`}>
        <div className="px-5 py-3 text-[15px] font-extrabold text-white" style={{ background: color }}>{label}</div>
        <ul className="grid gap-2.5 p-5">
          {docs.map((doc) => <li key={doc} className="flex items-center gap-2 text-[16px] font-semibold text-[#24352a]"><FileCheck2 className="size-4 shrink-0" style={{ color }} />{doc}</li>)}
        </ul>
      </div>)}
    </div>
    <div className="mt-8 flex flex-wrap gap-3" {...reveal(8)}>
      <span className={`${styles.chip} bg-[#edf3eb] text-[#28733a]`}><CheckCircle2 className="size-4" />Per tanggal distribusi</span>
      <span className={`${styles.chip} bg-[#edf3eb] text-[#28733a]`}><CheckCircle2 className="size-4" />Nomor urut berlanjut antar hari</span>
      <span className={`${styles.chip} bg-[#edf3eb] text-[#28733a]`}><CheckCircle2 className="size-4" />Data berubah → ditandai "Perlu dibuat ulang"</span>
    </div>
    <SlideFooter label="Berita Acara" />
  </div>;
}

function RolesSlide() {
  const roles: [LucideIcon, string, string][] = [
    [KeyRound, 'Admin pusat', 'Persiapan program, role & pengguna'],
    [FileSpreadsheet, 'Operator data', 'Import DCP3, periksa data penerima'],
    [ScanBarcode, 'Petugas POS Mesin', 'Nomor bagi, serial, foto mesin'],
    [ClipboardCheck, 'Petugas POS Dokumen', 'Penerima, tanggal, peralatan, tanda tangan'],
    [PackageCheck, 'Petugas POS Penyerahan', 'Foto serah terima, selesaikan distribusi'],
    [Images, 'Tim dokumentasi', 'Foto & video kegiatan'],
    [Files, 'Admin BA', 'Penandatangan, finalisasi Berita Acara'],
    [MapPinned, 'Koordinator / pimpinan', 'Monitoring, peta, laporan'],
  ];
  return <div className="flex h-full flex-col justify-center px-16 pt-10 pb-24">
    <p {...reveal(0)} className={styles.eyebrow}><Users className="size-4" />Peran</p>
    <h2 {...reveal(1)} className={`${styles.title} mt-3`}>Siapa mengerjakan apa</h2>
    <p {...reveal(2)} className={`${styles.lead} mt-3`}>Menu yang tampil mengikuti role. Setiap role hanya melihat kabupaten timnya.</p>
    <div className="mt-9 grid grid-cols-4 gap-4">
      {roles.map(([Icon, title, text], i) => <div key={title} {...reveal(3 + i)} className={`${styles.card} p-5`}>
        <Icon className="size-7 text-[#2f7d4a]" />
        <b className="mt-3 block text-[17px] text-[#173d2b]">{title}</b>
        <span className="mt-1 block text-[14px] leading-snug text-[#5d6b61]">{text}</span>
      </div>)}
    </div>
    <SlideFooter label="Peran" />
  </div>;
}

function GlossarySlide() {
  return <div className="flex h-full flex-col justify-center px-16 pt-10 pb-24">
    <p {...reveal(0)} className={styles.eyebrow}><BadgeCheck className="size-4" />Istilah</p>
    <h2 {...reveal(1)} className={`${styles.title} mt-3`}>Istilah yang sering dipakai</h2>
    <dl className="mt-10 grid grid-cols-4 gap-4">
      {glossary.slice(0, 8).map((item, i) => <div key={item.term} {...reveal(2 + i)} className={`${styles.card} p-5`}>
        <dt className="text-[17px] leading-snug font-extrabold text-[#173d2b]">{item.term}</dt>
        <dd className="mt-1.5 text-[14px] leading-snug text-[#5d6b61]">{item.meaning}</dd>
      </div>)}
    </dl>
    <SlideFooter label="Istilah" />
  </div>;
}

function ClosingSlide() {
  const start: [LucideIcon, string][] = [
    [LogIn, 'Login dengan akun dari admin'],
    [Building2, 'Pilih jadwal kabupaten Anda'],
    [Workflow, 'Kerjakan tahap sesuai peran'],
    [BadgeCheck, 'Butuh bantuan? Tombol ? di setiap halaman'],
  ];
  return <>
    <div className={styles.cover} style={{ backgroundImage: 'url(/static/images/konkit-field-2.jpeg)', backgroundSize: '135%', backgroundPosition: '62% 22%' }} />
    <div className={styles.coverShade} />
    <div className="relative flex h-full flex-col justify-center p-16 text-white">
      <p {...reveal(0)} className="text-sm font-bold tracking-[0.18em] text-[#d7b764] uppercase">Mulai sekarang</p>
      <h2 {...reveal(1)} className="mt-3 max-w-[760px] text-[60px] leading-[1.05] font-extrabold tracking-tight">Satu alur, dari kantor sampai penerima.</h2>
      <div className="mt-10 grid max-w-[760px] grid-cols-2 gap-4">
        {start.map(([Icon, text], i) => <div key={text} {...reveal(2 + i)} className={`${styles.glass} flex items-center gap-3 px-5 py-4 text-[17px] font-semibold`}>
          <span className="grid size-9 shrink-0 place-items-center rounded-xl bg-[#d7b764] text-[#173d2b]"><Icon className="size-5" /></span>{text}
        </div>)}
      </div>
    </div>
  </>;
}

export const slides: Slide[] = [
  { id: 'sampul', title: 'Konkit', render: () => <CoverSlide /> },
  { id: 'pengantar', title: guideIntro.title, render: () => <PurposeSlide /> },
  { id: 'alur', title: 'Alur besar', render: () => <FlowSlide /> },
  ...guideSteps.slice(0, 4).map((step) => ({ id: step.id, title: step.title, render: () => <StepSlide step={step} /> })),
  ...guideSteps.slice(4, 5).map((step) => ({ id: step.id, title: step.title, render: () => <StepSlide step={step} /> })),
  { id: 'aplikasi-hp', title: 'Aplikasi HP', render: () => <MobileSlide /> },
  ...guideSteps.slice(5, 7).map((step) => ({ id: step.id, title: step.title, render: () => <StepSlide step={step} /> })),
  { id: 'dokumen-ba', title: 'Berita Acara', render: () => <BeritaAcaraSlide /> },
  ...guideSteps.slice(7).map((step) => ({ id: step.id, title: step.title, render: () => <StepSlide step={step} /> })),
  { id: 'peran', title: 'Siapa mengerjakan apa', render: () => <RolesSlide /> },
  { id: 'istilah', title: 'Istilah', render: () => <GlossarySlide /> },
  { id: 'mulai', title: 'Mulai sekarang', render: () => <ClosingSlide /> },
];
