import { useQuery } from '@tanstack/react-query';
import { FileText } from 'lucide-react';
import { useSearchParams } from 'react-router-dom';
import { DataState } from '@/components/DataState';
import { PageHeader } from '@/components/PageHeader';
import { Badge } from '@/components/ui/badge';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { apiRequest } from '@/lib/api';
import type { DataResponse, ProgramType, Schedule } from '../programs/types';
import { programTypeLabel } from '../programs/types';
import { BAIndividualPanel } from './BAIndividualPanel';
import { LogoTenderPanel } from './LogoTenderPanel';

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
        <Select disabled={schedules.isPending || schedules.isError} value={scheduleID} onValueChange={(value) => updateParam('schedule_id', value ?? '')}>
          <SelectTrigger className="w-full" aria-labelledby="bast-schedule-label"><SelectValue placeholder={schedules.isPending ? 'Memuat jadwal...' : schedules.isError ? 'Jadwal tidak tersedia' : 'Pilih kabupaten dan jadwal'} /></SelectTrigger>
          <SelectContent>{activeSchedules.map((schedule) => <SelectItem key={schedule.id} value={schedule.id}>{schedule.regency?.name} / {schedule.name}</SelectItem>)}</SelectContent>
        </Select>
      </div>
      {selectedSchedule && programType && <div className="flex min-h-11 flex-wrap items-center gap-2 rounded-lg border bg-muted/30 px-3 py-2">
        <Badge>{programTypeLabel(programType)}</Badge>
        <span className="text-sm font-medium">{selectedSchedule.regency?.name}</span>
        <span aria-hidden="true" className="text-muted-foreground">·</span>
        <span className="text-sm text-muted-foreground">{selectedSchedule.program?.name}</span>
      </div>}
    </section>

    {schedules.isError && <DataState kind="error" title="Jadwal berita acara belum dapat dimuat" description="Periksa koneksi atau hak akses, lalu coba kembali." action={{ label: 'Coba lagi', onClick: () => schedules.refetch() }} />}

    {selectedSchedule && <LogoTenderPanel programID={selectedSchedule.program_id} />}

    <Tabs value={activeTab} onValueChange={(value) => updateParam('tab', value)}>
      <div className="overflow-x-auto pb-2" role="presentation">
        <TabsList activateOnFocus variant="line" className="h-auto min-w-max justify-start" aria-label="Jenis berita acara">
          {documentTypes.map((document) => <TabsTrigger className="min-h-11 px-3" key={document.value} value={document.value}>{document.label}</TabsTrigger>)}
        </TabsList>
      </div>
      {documentTypes.map((document) => <TabsContent className="pt-3" key={document.value} value={document.value}>
        {document.value === 'ba-perorangan' && selectedSchedule && programType
          ? <section aria-label={`BA Perorangan ${programTypeLabel(programType)}`}><BAIndividualPanel programID={selectedSchedule.program_id} regencyID={selectedSchedule.regency_id} regencyName={selectedSchedule.regency?.name ?? 'Kabupaten/Kota'} programType={programType} /></section>
          : <DocumentWorkspace document={document} programType={programType} />}
      </TabsContent>)}
    </Tabs>
  </div>;
}
