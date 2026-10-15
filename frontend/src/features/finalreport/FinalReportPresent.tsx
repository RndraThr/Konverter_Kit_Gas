import { useQuery } from '@tanstack/react-query';
import { ChevronLeft, ChevronRight, LoaderCircle, X } from 'lucide-react';
import { useEffect, useState, type ReactNode } from 'react';
import { apiRequest } from '../../lib/api';
import { SLIDE_KEYS, SLIDE_KEY_MAX_PICKS, type FinalReportPreview, type SlideKey } from './types';

type DataResponse<T> = { data: T };

const iconButton = 'rounded p-2 text-white hover:bg-white/10 focus-visible:outline focus-visible:outline-2 focus-visible:outline-white disabled:opacity-30 disabled:hover:bg-transparent';

export default function FinalReportPresent({ programId, regencyId, onClose }: { programId: string; regencyId: string; onClose: () => void }) {
  const [index, setIndex] = useState(0);
  const preview = useQuery({
    queryKey: ['final-report', 'preview', programId, regencyId],
    queryFn: () => apiRequest<DataResponse<FinalReportPreview>>(`/api/v1/final-report/preview?program_id=${encodeURIComponent(programId)}&regency_id=${encodeURIComponent(regencyId)}`),
  });

  const slides = preview.data ? buildSlides(preview.data.data) : [];
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
    <div className="flex flex-1 items-center justify-center overflow-y-auto p-10">
      {preview.isPending ? <LoaderCircle className="size-10 animate-spin" aria-label="Memuat" role="status" />
        : preview.isError ? <div className="space-y-4 text-center">
          <p>Gagal memuat tampilan laporan.</p>
          <button type="button" className="rounded border border-white/60 px-4 py-2 hover:bg-white/10" onClick={() => { void preview.refetch(); }}>Coba lagi</button>
        </div>
          : slides[index]}
    </div>
    <div className="flex items-center justify-center gap-6 p-4">
      <button type="button" aria-label="Slide sebelumnya" className={iconButton} disabled={index === 0} onClick={() => setIndex((i) => Math.max(i - 1, 0))}><ChevronLeft aria-hidden="true" /></button>
      <span>{index + 1} / {total || 1}</span>
      <button type="button" aria-label="Slide berikutnya" className={iconButton} disabled={index >= total - 1} onClick={() => setIndex((i) => Math.min(i + 1, total - 1))}><ChevronRight aria-hidden="true" /></button>
    </div>
  </div>;
}

function Attendees({ data }: { data: FinalReportPreview }) {
  return <>
    <p className="pt-2 font-semibold">Dihadiri oleh:</p>
    <ul className="list-disc space-y-1 pl-6">
      <li>Tim ESDM</li>
      <li>Tim Pertamina</li>
      <li>{data.agriculture_office_name}</li>
      <li>PT Kian Santang Muliatama Tbk (Penyedia Barang)</li>
      <li>{data.consultant_company_name} (Konsultan Pengawas)</li>
      <li>{data.machine_manufacturer_name} (Pabrikan Mesin)</li>
    </ul>
  </>;
}

function docSlide(key: SlideKey, data: FinalReportPreview): { heading: string; lines: ReactNode[] } {
  switch (key) {
    case 'pengiriman_penerimaan':
      return { heading: 'Dokumentasi Pengiriman dan Penerimaan Barang', lines: [data.handover_location, `Tanggal ${data.delivery_range_start} s.d. ${data.delivery_range_end}`] };
    case 'rapat_koordinasi':
      return { heading: 'Dokumentasi Rapat Koordinasi', lines: [data.agriculture_office_name, `Tanggal ${data.rakorda_date}`] };
    case 'sosialisasi':
      return { heading: 'Dokumentasi Seremoni dan Sosialisasi', lines: [data.handover_location, `Tanggal ${data.sosialisasi_date}`] };
  }
}

function buildSlides(data: FinalReportPreview): ReactNode[] {
  return [
    <div key="title" className="max-w-3xl space-y-4 text-center">
      <h1 className="text-3xl font-bold">LAPORAN KEGIATAN</h1>
      <p>PENYEDIAAN DAN PENDISTRIBUSIAN PAKET PERDANA LIQUEFIED PETROLEUM GAS (LPG) UNTUK MESIN POMPA AIR BAGI PETANI SASARAN TAHUN ANGGARAN {data.fiscal_year} DI PT PERTAMINA PATRA NIAGA</p>
      <p>PT KIAN SANTANG MULIATAMA TBK</p>
      <p className="text-2xl">{data.regency_label}</p>
    </div>,
    <div key="pengiriman" className="max-w-2xl space-y-2">
      <h2 className="text-xl font-semibold">Pengiriman dan Penerimaan Barang</h2>
      <p>{data.agriculture_office_name}</p>
      <p>Tanggal {data.delivery_hose_date} (Selang Buang)</p>
      <p>Tanggal {data.delivery_machine_group_date} (Mesin Pompa), (Konkit), (Oli), (Bracket), (Name Plate) dan (Selang Hisap)</p>
      <p>Tanggal {data.delivery_gas_cylinder_date} (Tabung Gas 3 Kg)</p>
      <h2 className="pt-4 text-xl font-semibold">Rapat Koordinasi, {data.rakorda_date}</h2>
      <Attendees data={data} />
    </div>,
    <div key="sosialisasi" className="max-w-2xl space-y-2">
      <h2 className="text-xl font-semibold">Sosialisasi, {data.sosialisasi_date}</h2>
      <Attendees data={data} />
      <h3 className="pt-4 font-semibold">Anggota Tim</h3>
      <ul className="list-disc space-y-1 pl-6">
        <li>Leader ({data.leader_count} orang)</li>
        <li>Administrasi ({data.admin_count} orang)</li>
        <li>Teknisi ({data.technician_count} orang)</li>
        <li>Helper ({data.helper_count} orang)</li>
      </ul>
    </div>,
    <div key="pendistribusian" className="max-w-2xl space-y-2">
      <h2 className="text-xl font-semibold">Pendistribusian</h2>
      <p>{data.handover_location}</p>
      <p>Tanggal Mulai {data.distribution_start_date}</p>
      <p>Tanggal Selesai {data.distribution_end_date}</p>
      <p>Total Paket {data.distribution_total} Unit</p>
    </div>,
    ...SLIDE_KEYS.map((key) => {
      const { heading, lines } = docSlide(key, data);
      const picks = (data.photo_picks[key] ?? []).slice(0, SLIDE_KEY_MAX_PICKS[key]);
      return <div key={`docs-${key}`} className="w-full max-w-4xl space-y-4">
        <div className="space-y-1 text-center">
          <h2 className="text-xl font-semibold">{heading}</h2>
          {lines.map((line, i) => <p key={i} className="text-white/80">{line}</p>)}
        </div>
        {picks.length === 0
          ? <p className="text-center text-white/70">Belum ada foto dipilih.</p>
          : <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
            {picks.map((pick) => <img key={pick.id} src={pick.content_url} alt={pick.display_name} className="aspect-video w-full rounded object-cover" />)}
          </div>}
      </div>;
    }),
    <div key="docs-pendistribusian" className="max-w-2xl space-y-1 text-center">
      <h2 className="text-xl font-semibold">Dokumentasi Pendistribusian</h2>
      <p className="text-white/80">{data.handover_location}</p>
    </div>,
    <p key="thanks" className="text-3xl font-bold">Terima Kasih</p>,
  ];
}
