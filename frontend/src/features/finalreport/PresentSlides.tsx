import type { CSSProperties, ReactNode } from 'react';
import isoStrip from '@/assets/finalreport/iso-strip.png';
import ksmLogo from '@/assets/finalreport/ksm-logo.png';
import { SLIDE_KEY_MAX_PICKS, type FinalReportPhotoPick, type FinalReportPreview, type SlideKey } from './types';

/*
 * The web view of the Laporan Akhir deck. It mirrors
 * internal/finalreport/template/design/build.py: the same 16:9 slides,
 * positions (inches) and type sizes (pt), drawn on a 1333x750 px canvas
 * (100 px per inch) that the presenter scales to the screen.
 */

export const SLIDE_W = 1333.3;
export const SLIDE_H = 750;

const NAVY = '#0F2A43';
const NAVY_2 = '#1C3D5E';
const GOLD = '#B8893A';
const INK = '#1F2937';
const MUTED = '#6B7280';
const PANEL = '#F6F4EF';
const LINE = '#E5E0D5';
const SOFT = '#D6DEE8';

const IN = 100;
const pt = (size: number) => (size * IN) / 72;
const box = (x: number, y: number, w: number, h: number): CSSProperties => ({ position: 'absolute', left: x * IN, top: y * IN, width: w * IN, height: h * IN });
const text = (size: number, color = INK, bold = false, extra: CSSProperties = {}): CSSProperties => ({ fontSize: pt(size), color, fontWeight: bold ? 700 : 400, lineHeight: 1.15, margin: 0, ...extra });
const eyebrow = (size: number, color: string, spacing = 2): CSSProperties => text(size, color, true, { letterSpacing: spacing });

function Rect({ x, y, w, h, fill, line }: { x: number; y: number; w: number; h: number; fill: string; line?: string }) {
  return <div style={{ ...box(x, y, w, h), background: fill, boxSizing: 'border-box', border: line ? `1px solid ${line}` : undefined }} />;
}

function Slide({ label, children, background = '#FFFFFF' }: { label: string; children: ReactNode; background?: string }) {
  return <div role="group" aria-label={label} style={{ position: 'relative', width: SLIDE_W, height: SLIDE_H, background, overflow: 'hidden', fontFamily: 'Calibri, Carlito, "Segoe UI", sans-serif', textAlign: 'left' }}>
    {children}
  </div>;
}

function Chrome({ number, data }: { number: number; data: FinalReportPreview }) {
  return <>
    <img src={ksmLogo} alt="PT Kian Santang Muliatama Tbk" style={box(0.5, 0.36, 3.3, 3.3 / 6.736)} />
    <div style={{ ...box(7.3, 0.42, 5.53, 0.5), textAlign: 'right' }}>
      <p style={eyebrow(10, GOLD, 1.5)}>LAPORAN AKHIR KEGIATAN</p>
      <p style={text(10, MUTED)}>{data.regency_label}  ·  TA {data.fiscal_year}</p>
    </div>
    <Rect x={0.5} y={1.0} w={12.333} h={0.025} fill={GOLD} />
    <img src={isoStrip} alt="" style={box(0.5, 6.98, 2.3, 2.3 / 8.196)} />
    <p style={{ ...box(8.3, 6.98, 4.53, 0.3), ...text(9, MUTED), display: 'flex', alignItems: 'center', justifyContent: 'flex-end', gap: 6 }}>
      PT Kian Santang Muliatama Tbk <b style={{ color: NAVY }}>{String(number).padStart(2, '0')}</b>
    </p>
  </>;
}

function Heading({ eyebrow: label, title }: { eyebrow: string; title: string }) {
  return <>
    <p style={{ ...box(0.5, 1.3, 12.3, 0.3), ...eyebrow(11, GOLD) }}>{label}</p>
    <h2 style={{ ...box(0.5, 1.6, 12.3, 0.6), ...text(28, NAVY, true) }}>{title}</h2>
  </>;
}

function Card({ x, y, w, h }: { x: number; y: number; w: number; h: number }) {
  return <><Rect x={x} y={y} w={w} h={h} fill={PANEL} /><Rect x={x} y={y} w={0.07} h={h} fill={GOLD} /></>;
}

function Bullets({ items }: { items: string[] }) {
  return <ul style={{ listStyle: 'none', margin: 0, padding: 0 }}>
    {items.map((item) => <li key={item} style={{ ...text(13), display: 'flex', gap: 0.12 * IN, marginBottom: pt(5) }}>
      <span aria-hidden="true" style={{ color: GOLD, width: 0.16 * IN, flexShrink: 0 }}>•</span>{item}
    </li>)}
  </ul>;
}

const attendees = (data: FinalReportPreview) => [
  'Tim ESDM',
  'Tim Pertamina',
  data.agriculture_office_name,
  'PT Kian Santang Muliatama Tbk (Penyedia Barang)',
  `${data.consultant_company_name} (Konsultan Pengawas)`,
  `${data.machine_manufacturer_name} (Pabrikan Mesin)`,
];

function Meeting({ x, y, w, title, date, place, data }: { x: number; y: number; w: number; title: string; date: string; place: string; data: FinalReportPreview }) {
  return <div style={box(x + 0.35, y + 0.25, w - 0.6, 3.9)}>
    <h3 style={{ ...text(17, NAVY, true), marginBottom: pt(3) }}>{title}</h3>
    <p style={{ ...text(14, GOLD, true), marginBottom: pt(2) }}>{date}</p>
    <p style={{ ...text(12, MUTED), marginBottom: pt(12) }}>{place}</p>
    <p style={{ ...eyebrow(10, MUTED, 1.5), marginBottom: pt(6) }}>DIHADIRI OLEH</p>
    <Bullets items={attendees(data)} />
  </div>;
}

function Cover({ data }: { data: FinalReportPreview }) {
  return <Slide label="Sampul">
    <Rect x={0} y={0} w={8.1} h={7.5} fill={NAVY} />
    <Rect x={8.1} y={0} w={0.08} h={7.5} fill={GOLD} />
    <Rect x={0.75} y={0.9} w={0.9} h={0.05} fill={GOLD} />
    <p style={{ ...box(0.75, 1.1, 6.9, 0.4), ...eyebrow(14, GOLD, 3) }}>LAPORAN AKHIR KEGIATAN</p>
    <h1 style={{ ...box(0.75, 1.65, 6.9, 2.6), ...text(28, '#FFFFFF', true, { lineHeight: 1.1 }) }}>
      Penyediaan dan Pendistribusian Paket Perdana Liquefied Petroleum Gas (LPG) untuk {data.program_target}
    </h1>
    <div style={box(0.75, 4.25, 6.9, 0.8)}>
      <p style={text(16, SOFT)}>Tahun Anggaran {data.fiscal_year}</p>
      <p style={text(16, SOFT)}>di PT Pertamina Patra Niaga</p>
    </div>
    <Rect x={0.75} y={5.35} w={6.6} h={0.85} fill={NAVY_2} />
    <Rect x={0.75} y={5.35} w={0.07} h={0.85} fill={GOLD} />
    <p style={{ ...box(1.05, 5.35, 6.2, 0.85), ...text(22, GOLD, true), display: 'flex', alignItems: 'center' }}>{data.regency_label}</p>
    <p style={{ ...box(8.75, 2.55, 4.1, 0.3), ...eyebrow(10, MUTED) }}>PENYEDIA BARANG</p>
    <img src={ksmLogo} alt="PT Kian Santang Muliatama Tbk" style={box(8.75, 2.9, 4.1, 4.1 / 6.736)} />
    <Rect x={8.75} y={3.75} w={4.1} h={0.015} fill={LINE} />
    <p style={{ ...box(8.75, 3.9, 4.1, 0.9), ...text(12, MUTED) }}>Pendistribusian paket perdana LPG untuk {data.program_target}.</p>
    <img src={isoStrip} alt="Sertifikat ISO" style={box(8.75, 6.75, 3.2, 3.2 / 8.196)} />
  </Slide>;
}

function Pengiriman({ data }: { data: FinalReportPreview }) {
  const timeline = [
    [data.delivery_hose_date, 'Selang Buang'],
    [data.delivery_machine_group_date, 'Mesin Pompa, Konkit, Oli, Bracket, Name Plate dan Selang Hisap'],
    [data.delivery_gas_cylinder_date, 'Tabung Gas 3 Kg'],
  ];
  return <Slide label="Pengiriman Barang & Rapat Koordinasi">
    <Chrome number={2} data={data} />
    <Heading eyebrow="01  ·  PERSIAPAN" title="Pengiriman Barang & Rapat Koordinasi" />
    <Card x={0.5} y={2.4} w={6.05} h={4.3} />
    <div style={box(0.85, 2.65, 5.45, 0.85)}>
      <h3 style={{ ...text(17, NAVY, true), marginBottom: pt(3) }}>Pengiriman dan Penerimaan Barang</h3>
      <p style={text(12, MUTED)}>{data.agriculture_office_name}</p>
    </div>
    {timeline.map(([date, items], i) => <div key={items}>
      <Rect x={0.88} y={3.73 + i * 0.98} w={0.14} h={0.14} fill={GOLD} />
      <div style={box(1.2, 3.65 + i * 0.98, 5.05, 0.95)}>
        <p style={{ ...text(14, NAVY, true), marginBottom: pt(1) }}>{date}</p>
        <p style={text(12)}>{items}</p>
      </div>
    </div>)}
    <Card x={6.78} y={2.4} w={6.05} h={4.3} />
    <Meeting x={6.78} y={2.4} w={6.05} title="Rapat Koordinasi" date={data.rakorda_date} place={data.rakorda_location} data={data} />
  </Slide>;
}

function Sosialisasi({ data }: { data: FinalReportPreview }) {
  const tiles = [[data.leader_count, 'Leader'], [data.admin_count, 'Administrasi'], [data.technician_count, 'Teknisi'], [data.helper_count, 'Helper']];
  const tw = (6.05 - 0.25) / 2;
  return <Slide label="Sosialisasi & Tim Pelaksana">
    <Chrome number={3} data={data} />
    <Heading eyebrow="02  ·  SOSIALISASI" title="Sosialisasi & Tim Pelaksana" />
    <Card x={0.5} y={2.4} w={6.05} h={4.3} />
    <Meeting x={0.5} y={2.4} w={6.05} title="Sosialisasi" date={data.sosialisasi_date} place={data.sosialisasi_location} data={data} />
    <h3 style={{ ...box(6.78, 2.4, 6.05, 0.5), ...text(17, NAVY, true) }}>Anggota Tim Pelaksana</h3>
    {tiles.map(([count, label], i) => {
      const x = 6.78 + (i % 2) * (tw + 0.25);
      const y = 2.95 + Math.floor(i / 2) * 2;
      return <div key={label}>
        <Rect x={x} y={y} w={tw} h={1.75} fill="#FFFFFF" line={LINE} />
        <Rect x={x} y={y} w={tw} h={0.06} fill={i === 0 ? GOLD : NAVY} />
        <div style={box(x + 0.3, y + 0.25, tw - 0.5, 1.35)}>
          <p style={{ ...text(40, NAVY, true), marginBottom: pt(2) }}>{count}<span style={text(13, MUTED)}>  orang</span></p>
          <p style={text(14, INK, true)}>{label}</p>
        </div>
      </div>;
    })}
  </Slide>;
}

function Pendistribusian({ data }: { data: FinalReportPreview }) {
  const tw = (12.333 - 0.5) / 3;
  const tile = (label: string, value: string, i: number) => <div key={label}>
    <Rect x={0.5 + i * (tw + 0.25)} y={3.95} w={tw} h={2.7} fill="#FFFFFF" line={LINE} />
    <Rect x={0.5 + i * (tw + 0.25)} y={3.95} w={tw} h={0.06} fill={NAVY} />
    <div style={{ ...box(0.85 + i * (tw + 0.25), 3.95, tw - 0.6, 2.7), display: 'flex', flexDirection: 'column', justifyContent: 'center' }}>
      <p style={{ ...eyebrow(10, MUTED, 1.5), marginBottom: pt(8) }}>{label}</p>
      <p style={text(24, NAVY, true)}>{value}</p>
    </div>
  </div>;
  const x = 0.5 + 2 * (tw + 0.25);
  return <Slide label="Pendistribusian Paket">
    <Chrome number={4} data={data} />
    <Heading eyebrow="03  ·  PENDISTRIBUSIAN" title="Pendistribusian Paket" />
    <Card x={0.5} y={2.4} w={12.333} h={1.25} />
    <div style={{ ...box(0.85, 2.4, 11.8, 1.25), display: 'flex', flexDirection: 'column', justifyContent: 'center' }}>
      <p style={{ ...eyebrow(10, MUTED, 1.5), marginBottom: pt(4) }}>TITIK SERAH</p>
      <p style={text(18, NAVY, true)}>{data.handover_location}</p>
    </div>
    {tile('TANGGAL MULAI', data.distribution_start_date, 0)}
    {tile('TANGGAL SELESAI', data.distribution_end_date, 1)}
    <Rect x={x} y={3.95} w={tw} h={2.7} fill={NAVY} />
    <Rect x={x} y={3.95} w={tw} h={0.06} fill={GOLD} />
    <div style={{ ...box(x + 0.35, 3.95, tw - 0.6, 2.7), display: 'flex', flexDirection: 'column', justifyContent: 'center' }}>
      <p style={{ ...eyebrow(10, SOFT, 1.5), marginBottom: pt(4) }}>TOTAL PAKET TERSALURKAN</p>
      <p style={text(54, GOLD, true)}>{data.distribution_total}<span style={text(18, '#FFFFFF', true)}>  Unit</span></p>
    </div>
  </Slide>;
}

type Frame = [number, number, number, number];

/** Photo frames (all 4:3) on the right of the info panel, as in build.py. */
function frames(layout: 'big+2' | '2' | '2x2'): Frame[] {
  const [ax, ay, aw, ah, g] = [4.15, 1.3, 8.68, 5.4, 0.2];
  if (layout === 'big+2') {
    const hb = (aw - g + (2 / 3) * g) / 2;
    const wb = (4 / 3) * hb;
    const hs = (hb - g) / 2;
    const ws = (4 / 3) * hs;
    const oy = ay + (ah - hb) / 2;
    return [[ax, oy, wb, hb], [ax + wb + g, oy, ws, hs], [ax + wb + g, oy + hs + g, ws, hs]];
  }
  if (layout === '2') {
    const fw = (aw - g) / 2;
    const fh = (fw * 3) / 4;
    const oy = ay + (ah - fh) / 2;
    return [[ax, oy, fw, fh], [ax + fw + g, oy, fw, fh]];
  }
  const fh = (ah - g) / 2;
  const fw = (fh * 4) / 3;
  const ox = ax + (aw - 2 * fw - g) / 2;
  return [0, 1].flatMap((r) => [0, 1].map((c): Frame => [ox + c * (fw + g), ay + r * (fh + g), fw, fh]));
}

function DocSlide({ number, title, place, dates, layout, picks, data, emptyLabel = 'Foto belum dipilih' }: {
  number: number; title: string; place: string; dates: string[]; layout: 'big+2' | '2' | '2x2'; picks: FinalReportPhotoPick[]; data: FinalReportPreview; emptyLabel?: string;
}) {
  return <Slide label={`Dokumentasi ${title}`}>
    <Chrome number={number} data={data} />
    <Rect x={0.5} y={1.3} w={3.4} h={5.4} fill={NAVY} />
    <Rect x={0.5} y={1.3} w={3.4} h={0.06} fill={GOLD} />
    <div style={box(0.8, 1.65, 2.8, 4.8)}>
      <p style={{ ...eyebrow(11, GOLD), marginBottom: pt(10) }}>DOKUMENTASI</p>
      <h2 style={{ ...text(22, '#FFFFFF', true, { lineHeight: 1.1 }), marginBottom: pt(16) }}>{title}</h2>
      <p style={{ ...text(13, SOFT), marginBottom: pt(10) }}>{place}</p>
      {dates.length > 0 && <>
        <p style={{ ...eyebrow(9, '#A9B6C6', 1.5), marginBottom: pt(2) }}>TANGGAL</p>
        {dates.map((date) => <p key={date} style={{ ...text(14, GOLD, true), marginBottom: pt(1) }}>{date}</p>)}
      </>}
    </div>
    {frames(layout).map(([x, y, w, h], i) => {
      const pick = picks[i];
      return pick
        ? <img key={pick.id} src={pick.content_url} alt={pick.display_name} style={{ ...box(x, y, w, h), objectFit: 'cover' }} />
        : <div key={i} style={{ ...box(x, y, w, h), background: '#E2E5E9', display: 'flex', alignItems: 'center', justifyContent: 'center', ...text(12, '#969CA5') }}>{emptyLabel}</div>;
    })}
  </Slide>;
}

function Closing({ data }: { data: FinalReportPreview }) {
  return <Slide label="Penutup" background={NAVY}>
    <Rect x={0} y={7.5 - 1.55} w={13.333} h={1.55} fill="#FFFFFF" />
    <Rect x={0} y={7.5 - 1.6} w={13.333} h={0.05} fill={GOLD} />
    <Rect x={13.333 / 2 - 0.45} y={2.0} w={0.9} h={0.05} fill={GOLD} />
    <p style={{ ...box(1, 2.25, 11.333, 1.2), ...text(54, '#FFFFFF', true), textAlign: 'center' }}>Terima Kasih</p>
    <div style={{ ...box(1, 3.45, 11.333, 0.9), textAlign: 'center' }}>
      <p style={text(16, SOFT)}>Laporan Akhir Kegiatan  ·  {data.regency_label}</p>
      <p style={text(16, SOFT)}>Tahun Anggaran {data.fiscal_year}</p>
    </div>
    <img src={ksmLogo} alt="PT Kian Santang Muliatama Tbk" style={box(13.333 / 2 - 1.9, 7.5 - 1.2, 3.8, 3.8 / 6.736)} />
  </Slide>;
}

const picksOf = (data: FinalReportPreview, key: SlideKey) => (data.photo_picks[key] ?? []).slice(0, SLIDE_KEY_MAX_PICKS[key]);

/** The nine slides of the deck, in order. */
export function buildDeck(data: FinalReportPreview): ReactNode[] {
  return [
    <Cover key="cover" data={data} />,
    <Pengiriman key="pengiriman" data={data} />,
    <Sosialisasi key="sosialisasi" data={data} />,
    <Pendistribusian key="pendistribusian" data={data} />,
    <DocSlide key="doc-pengiriman" number={5} title="Pengiriman dan Penerimaan Barang" place={data.agriculture_office_name}
      dates={[data.delivery_range_start, `s.d. ${data.delivery_range_end}`]} layout="big+2" picks={picksOf(data, 'pengiriman_penerimaan')} data={data} />,
    <DocSlide key="doc-rakor" number={6} title="Rapat Koordinasi" place={data.rakorda_location} dates={[data.rakorda_date]} layout="2" picks={picksOf(data, 'rapat_koordinasi')} data={data} />,
    <DocSlide key="doc-sosialisasi" number={7} title="Seremoni dan Sosialisasi" place={data.sosialisasi_location} dates={[data.sosialisasi_date]} layout="2x2" picks={picksOf(data, 'sosialisasi')} data={data} />,
    <DocSlide key="doc-pendistribusian" number={8} title="Pendistribusian" place={data.handover_location} dates={[]} layout="big+2" picks={[]} data={data} emptyLabel="Foto dokumentasi" />,
    <Closing key="closing" data={data} />,
  ];
}
