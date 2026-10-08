import { useQuery } from '@tanstack/react-query';
import { FileText } from 'lucide-react';
import { useSearchParams } from 'react-router-dom';
import { DataState } from '@/components/DataState';
import { PageHeader } from '@/components/PageHeader';
import { Badge } from '@/components/ui/badge';
import { Label } from '@/components/ui/label';
import { Combobox, ComboboxContent, ComboboxEmpty, ComboboxInput, ComboboxItem, ComboboxList } from '@/components/ui/combobox';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { apiRequest } from '@/lib/api';
import type { DataResponse, ProgramType, Schedule } from '../programs/types';
import { programTypeLabel } from '../programs/types';
import { BAIndividualPanel } from './BAIndividualPanel';
import { ClosingKabupatenPanel } from './ClosingKabupatenPanel';
import { ClosingTitikSerahPanel } from './ClosingTitikSerahPanel';
import { DailyRecapPanel } from './DailyRecapPanel';
import { DP3Panel } from './DP3Panel';
import { PemeriksaanPanel } from './PemeriksaanPanel';
import { RakordaPanel } from './RakordaPanel';
import { ServisBerkalaPanel } from './ServisBerkalaPanel';
import { SosialisasiPanel } from './SosialisasiPanel';
import { TKDNPanel } from './TKDNPanel';
import { Training10Panel } from './Training10Panel';
import { Training100Panel } from './Training100Panel';

const documentTypes = [
  { value: 'dp3', label: 'DP3' },
  { value: 'ba-perorangan', label: 'BA Perorangan' },
  { value: 'rekap-harian', label: 'Rekap Harian' },
  { value: 'closing-titik-serah', label: 'Closing Titik Serah' },
  { value: 'closing-kabupaten', label: 'Closing Kabupaten' },
  { value: 'rakorda', label: 'Rakorda' },
  { value: 'sosialisasi', label: 'Sosialisasi' },
  { value: 'training-10', label: 'Training 10%' },
  { value: 'training-100', label: 'Training 100%' },
  { value: 'ba-pemeriksaan', label: 'BA Pemeriksaan' },
  { value: 'servis-berkala', label: 'Servis Berkala' },
  { value: 'tkdn', label: 'TKDN' },
] as const;

type DocumentType = (typeof documentTypes)[number];

function DocumentWorkspace({ document, programType }: { document: DocumentType; programType?: ProgramType }) {
  if (!programType) return <div className="rounded-xl border border-dashed bg-muted/20 px-5 py-10 text-center text-sm text-muted-foreground">
    Pilih jadwal program untuk menyiapkan dokumen {document.label}.
  </div>;

  const programLabel = programTypeLabel(programType);
  return <section
    aria-label={`${document.label} ${programLabel}`}
    className="rounded-xl border bg-card px-5 py-10 text-center shadow-xs sm:px-8"
    data-workspace-key={`${programType}:${document.value}`}
  >
    <span className="mx-auto mb-4 grid size-11 place-items-center rounded-lg bg-primary/10 text-primary"><FileText aria-hidden="true" /></span>
    <h2 className="text-lg font-semibold">{document.label} · {programLabel}</h2>
    <p className="mx-auto mt-2 max-w-xl text-sm leading-6 text-muted-foreground">
      Format {document.label} untuk program {programLabel} akan disiapkan pada tahap berikutnya.
    </p>
  </section>;
}

export function BeritaAcaraPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const schedules = useQuery({
    queryKey: ['program-setup', 'schedules'],
    queryFn: () => apiRequest<DataResponse<Schedule[]>>('/api/v1/program-setup/schedules'),
  });
  const requestedTab = searchParams.get('tab');
  const activeTab = documentTypes.some((item) => item.value === requestedTab) ? requestedTab! : documentTypes[0].value;
  const scheduleID = searchParams.get('schedule_id') ?? '';
  const activeSchedules = schedules.data?.data.filter((schedule) => schedule.status === 'active') ?? [];
  const selectedSchedule = activeSchedules.find((schedule) => schedule.id === scheduleID);
  const scheduleOptions = activeSchedules.map((schedule) => ({ value: schedule.id, label: `${schedule.regency?.name ?? ''} / ${schedule.name}` }));
  const selectedOption = scheduleOptions.find((option) => option.value === scheduleID) ?? null;
  const programType = selectedSchedule?.program?.program_type;

  const updateParam = (name: 'schedule_id' | 'tab', value: string) => {
    const next = new URLSearchParams(searchParams);
    if (value) next.set(name, value);
    else next.delete(name);
    setSearchParams(next, { replace: true });
  };

  return <div className="page">
    <PageHeader title="Berita Acara" description="Kelola seluruh jenis berita acara dalam satu ruang kerja. Varian dokumen Petani atau Nelayan mengikuti jadwal yang dipilih." />

    <section className="grid gap-4 py-5 lg:grid-cols-[minmax(0,28rem)_1fr] lg:items-end" aria-label="Konteks berita acara">
      <div className="grid gap-1.5">
        <Label id="bast-schedule-label">Jadwal program</Label>
        <Combobox
          items={scheduleOptions}
          value={selectedOption}
          onValueChange={(option) => updateParam('schedule_id', option?.value ?? '')}
          itemToStringLabel={(option) => option.label}
          isItemEqualToValue={(option, value) => option.value === value.value}
        >
          <ComboboxInput
            className="w-full"
            aria-labelledby="bast-schedule-label"
            disabled={schedules.isPending || schedules.isError}
            placeholder={schedules.isPending ? 'Memuat jadwal...' : schedules.isError ? 'Jadwal tidak tersedia' : 'Cari kabupaten atau jadwal'}
          />
          <ComboboxContent>
            <ComboboxEmpty>Jadwal tidak ditemukan.</ComboboxEmpty>
            <ComboboxList>{(option: { value: string; label: string }) => <ComboboxItem key={option.value} value={option}>{option.label}</ComboboxItem>}</ComboboxList>
          </ComboboxContent>
        </Combobox>
      </div>
      {selectedSchedule && programType && <div className="flex min-h-11 flex-wrap items-center gap-2 rounded-lg border bg-muted/30 px-3 py-2">
        <Badge>{programTypeLabel(programType)}</Badge>
        <span className="text-sm font-medium">{selectedSchedule.regency?.name}</span>
        <span aria-hidden="true" className="text-muted-foreground">·</span>
        <span className="text-sm text-muted-foreground">{selectedSchedule.program?.name}</span>
      </div>}
    </section>

    {schedules.isError && <DataState kind="error" title="Jadwal berita acara belum dapat dimuat" description="Periksa koneksi atau hak akses, lalu coba kembali." action={{ label: 'Coba lagi', onClick: () => schedules.refetch() }} />}

    <Tabs value={activeTab} onValueChange={(value) => updateParam('tab', value)}>
      {/* Tab mengisi lebar penuh dan turun ke baris berikut di layar sempit (tanpa scroll). */}
      <div className="pb-2" role="presentation">
        <TabsList activateOnFocus variant="line" className="h-auto w-full flex-wrap justify-start gap-y-1" aria-label="Jenis berita acara">
          {documentTypes.map((document) => <TabsTrigger className="min-h-11 flex-auto px-3" key={document.value} value={document.value}>{document.label}</TabsTrigger>)}
        </TabsList>
      </div>
      {documentTypes.map((document) => <TabsContent className="pt-3" key={document.value} value={document.value}>
        {document.value === 'ba-perorangan' && selectedSchedule && programType
          ? <section aria-label={`BA Perorangan ${programTypeLabel(programType)}`}><BAIndividualPanel programID={selectedSchedule.program_id} regencyID={selectedSchedule.regency_id} regencyName={selectedSchedule.regency?.name ?? 'Kabupaten/Kota'} programType={programType} /></section>
          : document.value === 'dp3' && selectedSchedule && programType
            ? <section aria-label="DP3"><DP3Panel scheduleID={selectedSchedule.id} regencyName={selectedSchedule.regency?.name ?? 'Kabupaten/Kota'} programType={programType} defaultDate={selectedSchedule.start_date.slice(0, 10)} /></section>
            : document.value === 'rekap-harian' && selectedSchedule && programType
              ? <section aria-label="Rekap Harian"><DailyRecapPanel scheduleID={selectedSchedule.id} regencyName={selectedSchedule.regency?.name ?? 'Kabupaten/Kota'} programType={programType} /></section>
              : document.value === 'closing-titik-serah' && selectedSchedule && programType
                ? <section aria-label="Closing Titik Serah"><ClosingTitikSerahPanel scheduleID={selectedSchedule.id} regencyName={selectedSchedule.regency?.name ?? 'Kabupaten/Kota'} programType={programType} defaultDate={selectedSchedule.start_date.slice(0, 10)} /></section>
                : document.value === 'closing-kabupaten' && selectedSchedule && programType
                  ? <section aria-label="Closing Kabupaten"><ClosingKabupatenPanel scheduleID={selectedSchedule.id} regencyName={selectedSchedule.regency?.name ?? 'Kabupaten/Kota'} programType={programType} defaultDate={selectedSchedule.start_date.slice(0, 10)} /></section>
                  : document.value === 'rakorda' && selectedSchedule && programType
                    ? <section aria-label="RAKORDA"><RakordaPanel scheduleID={selectedSchedule.id} regencyName={selectedSchedule.regency?.name ?? 'Kabupaten/Kota'} programType={programType} defaultDate={selectedSchedule.start_date.slice(0, 10)} /></section>
                    : document.value === 'sosialisasi' && selectedSchedule && programType
                      ? <section aria-label="Sosialisasi"><SosialisasiPanel scheduleID={selectedSchedule.id} regencyName={selectedSchedule.regency?.name ?? 'Kabupaten/Kota'} programType={programType} defaultDate={selectedSchedule.start_date.slice(0, 10)} /></section>
                      : document.value === 'training-10' && selectedSchedule && programType
                        ? <section aria-label="Training 10%"><Training10Panel scheduleID={selectedSchedule.id} regencyName={selectedSchedule.regency?.name ?? 'Kabupaten/Kota'} programType={programType} defaultDate={selectedSchedule.start_date.slice(0, 10)} /></section>
                        : document.value === 'training-100' && selectedSchedule && programType
                          ? <section aria-label="Training 100%"><Training100Panel scheduleID={selectedSchedule.id} regencyName={selectedSchedule.regency?.name ?? 'Kabupaten/Kota'} programType={programType} defaultDate={selectedSchedule.start_date.slice(0, 10)} /></section>
                          : document.value === 'servis-berkala' && selectedSchedule && programType
                            ? <section aria-label="Servis Berkala"><ServisBerkalaPanel scheduleID={selectedSchedule.id} regencyName={selectedSchedule.regency?.name ?? 'Kabupaten/Kota'} programType={programType} defaultDate={selectedSchedule.start_date.slice(0, 10)} /></section>
                            : document.value === 'tkdn' && selectedSchedule && programType
                              ? <section aria-label="TKDN"><TKDNPanel scheduleID={selectedSchedule.id} programID={selectedSchedule.program_id} regencyName={selectedSchedule.regency?.name ?? 'Kabupaten/Kota'} programType={programType} defaultDate={selectedSchedule.start_date.slice(0, 10)} /></section>
                              : document.value === 'ba-pemeriksaan' && selectedSchedule && programType
                                ? <section aria-label="BA Pemeriksaan"><PemeriksaanPanel scheduleID={selectedSchedule.id} programID={selectedSchedule.program_id} regencyName={selectedSchedule.regency?.name ?? 'Kabupaten/Kota'} programType={programType} defaultDate={selectedSchedule.start_date.slice(0, 10)} /></section>
                                : <DocumentWorkspace document={document} programType={programType} />}
      </TabsContent>)}
    </Tabs>
  </div>;
}
