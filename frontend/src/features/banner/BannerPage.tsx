import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Download, ImageUp, Info, RotateCcw, Save } from 'lucide-react';
import { lazy, Suspense, useState, type ReactNode } from 'react';
import { toast } from 'sonner';
import { DataState } from '@/components/DataState';
import { FormField } from '@/components/FormField';
import { PageHeader } from '@/components/PageHeader';
import { SearchableSelect } from '@/components/SearchableSelect';
import { Badge } from '@/components/ui/badge';
import { Button, buttonVariants } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Label } from '@/components/ui/label';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useCan } from '@/lib/permissions';
import { ApiError, apiBlobRequest, apiRequest } from '../../lib/api';
import type { Schedule } from '../programs/types';

const PdfCanvasPreview = lazy(() => import('@/features/berita-acara/PdfCanvasPreview'));

type Kind = 'backdrop' | 'pelatihan' | 'seremoni' | 'plakat';
type ServiceCenter = { name: string; address: string; phone: string };
type Settings = {
  service_center_name: string; service_center_address: string; service_center_phone: string;
  seremoni_name: string; plakat_name: string; plakat_place: string; plakat_date: string;
};
type KindInfo = { kind: Kind; label: string; size: string; filename: string };
type Overview = {
  schedule_id: string; schedule_name: string; regency_name: string; display_name: string; short_name: string;
  supported: boolean; brand: string; has_logo: boolean; logo_updated_at?: string;
  master_service_center: ServiceCenter; service_center: ServiceCenter; settings: Settings;
  kinds: KindInfo[];
};

/** The fields each banner prints; Pelatihan Teknis has none besides the logo. */
const kindFields: Record<Kind, (keyof Settings)[]> = {
  backdrop: ['service_center_name', 'service_center_address', 'service_center_phone'],
  pelatihan: [],
  seremoni: ['seremoni_name'],
  plakat: ['plakat_name', 'plakat_place', 'plakat_date'],
};

/** The form shows the service center actually printed: the typed value, or the master data. */
const formFrom = (o: Overview): Settings => ({
  ...o.settings,
  service_center_name: o.service_center.name, service_center_address: o.service_center.address, service_center_phone: o.service_center.phone,
});

// Banner kabupaten dari template resmi. Tiap banner punya tab sendiri berisi
// isiannya dan pratinjau ukuran asli; logo kabupaten dipakai semua banner.
export function BannerPage() {
  const canManage = useCan('banner.manage');
  const [scheduleID, setScheduleID] = useState('');
  const [kind, setKind] = useState<Kind>('backdrop');
  const schedules = useQuery({ queryKey: ['program-setup', 'schedules'], queryFn: () => apiRequest<{ data: Schedule[] }>('/api/v1/program-setup/schedules') });
  const overview = useQuery({
    queryKey: ['banners', scheduleID],
    queryFn: () => apiRequest<{ data: Overview }>(`/api/v1/banners?schedule_id=${encodeURIComponent(scheduleID)}`),
    enabled: Boolean(scheduleID),
  });
  const data = overview.data?.data;
  const info = data?.kinds.find((item) => item.kind === kind) ?? data?.kinds[0];

  return <div className="space-y-6">
    <PageHeader title="Banner" description="Backdrop, pelatihan teknis, seremoni, dan plakat per kabupaten, dicetak dari template resmi dalam ukuran asli." />
    <section className="grid max-w-xl gap-2" aria-label="Pilih jadwal">
      <Label id="banner-schedule-label">Jadwal</Label>
      <SearchableSelect value={scheduleID} onValueChange={setScheduleID} aria-labelledby="banner-schedule-label" placeholder="Cari kabupaten atau jadwal" emptyText="Jadwal tidak ditemukan."
        options={(schedules.data?.data ?? []).map((schedule) => ({ value: schedule.id, label: `${schedule.regency?.name} / ${schedule.name}` }))} />
    </section>
    {!scheduleID ? <DataState kind="empty" title="Pilih jadwal" description="Banner disusun untuk kabupaten pada jadwal yang dipilih." />
      : overview.isPending ? <DataState kind="loading" title="Memuat banner" description="Mengambil logo dan data service center kabupaten." />
      : overview.isError || !data || !info ? <DataState kind="error" title="Banner belum dapat dimuat" description="Periksa koneksi lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => void overview.refetch() }} />
      : !data.supported ? <DataState kind="empty" title="Banner Nelayan belum tersedia" description="Template banner saat ini baru untuk program Petani." />
      : <div className="space-y-4">
        <RegencyLogo data={data} canManage={canManage} />
        <Tabs value={info.kind} onValueChange={(value) => setKind(value as Kind)}>
          <TabsList aria-label="Jenis banner" className="w-full justify-start overflow-x-auto sm:w-auto">{data.kinds.map((item) => <TabsTrigger key={item.kind} value={item.kind}>{item.label}</TabsTrigger>)}</TabsList>
        </Tabs>
        <div className="grid items-start gap-4 xl:grid-cols-[minmax(300px,360px)_minmax(0,1fr)]">
          <BannerSettings key={`${data.schedule_id}-${info.kind}`} data={data} info={info} canManage={canManage} />
          <BannerPreview data={data} info={info} />
        </div>
      </div>}
  </div>;
}

function RegencyLogo({ data, canManage }: { data: Overview; canManage: boolean }) {
  const queryClient = useQueryClient();
  const upload = useMutation({
    mutationFn: (file: File) => {
      const body = new FormData();
      body.append('file', file);
      return apiRequest<{ data: Overview }>(`/api/v1/banners/logo?schedule_id=${encodeURIComponent(data.schedule_id)}`, { method: 'PUT', body });
    },
    onSuccess: (response) => { queryClient.setQueryData(['banners', data.schedule_id], response); toast.success('Logo kabupaten diganti.'); },
    onError: (error) => toast.error(error instanceof ApiError ? error.message : 'Logo belum dapat diunggah.'),
  });
  return <Card>
    <CardContent className="flex flex-wrap items-center gap-4 py-3">
      <div className="grid size-16 shrink-0 place-items-center overflow-hidden rounded-lg border bg-muted/40 p-1.5">
        {data.has_logo
          ? <img src={`/api/v1/banners/logo?schedule_id=${encodeURIComponent(data.schedule_id)}&v=${encodeURIComponent(data.logo_updated_at ?? '')}`} alt={`Logo ${data.display_name}`} className="max-h-full max-w-full object-contain" />
          : <span className="text-center text-[10px] leading-tight text-muted-foreground">Belum ada logo</span>}
      </div>
      <div className="min-w-0 flex-1">
        <p className="font-semibold">{data.display_name}</p>
        <p className="text-sm text-muted-foreground">Logo kabupaten dipakai di semua banner. PNG atau JPG resolusi tinggi, tanpa dikompres.</p>
      </div>
      {canManage && <label className={buttonVariants({ variant: 'outline', size: 'sm', className: 'cursor-pointer' })}>
        <ImageUp />{upload.isPending ? 'Mengunggah...' : data.has_logo ? 'Ganti logo' : 'Unggah logo'}
        <input className="sr-only" type="file" accept="image/png,image/jpeg" aria-label="Pilih logo kabupaten" disabled={upload.isPending} onChange={(event) => { const file = event.target.files?.[0]; if (file) upload.mutate(file); event.target.value = ''; }} />
      </label>}
    </CardContent>
  </Card>;
}

function BannerSettings({ data, info, canManage }: { data: Overview; info: KindInfo; canManage: boolean }) {
  const queryClient = useQueryClient();
  const fields = kindFields[info.kind];
  const [form, setForm] = useState<Settings>(() => formFrom(data));
  const set = (field: keyof Settings) => (event: { target: { value: string } }) => setForm((current) => ({ ...current, [field]: event.target.value }));
  const saved = formFrom(data);
  const dirty = fields.some((field) => form[field] !== saved[field]);
  const master = data.master_service_center;
  const hasMaster = Boolean(master.name);
  const fromMaster = hasMaster && form.service_center_name === master.name && form.service_center_address === master.address && form.service_center_phone === master.phone;

  const save = useMutation({
    // The other banners' text is sent as saved, so only this banner changes.
    mutationFn: () => apiRequest<{ data: Overview }>(`/api/v1/banners/settings?schedule_id=${encodeURIComponent(data.schedule_id)}`, {
      method: 'PUT', body: JSON.stringify({ ...saved, ...Object.fromEntries(fields.map((field) => [field, form[field]])) }),
    }),
    onSuccess: (response) => { queryClient.setQueryData(['banners', data.schedule_id], response); toast.success(`${info.label} tersimpan.`); },
    onError: (error) => toast.error(error instanceof ApiError ? error.message : 'Data banner belum tersimpan.'),
  });

  let body: ReactNode;
  if (info.kind === 'backdrop') {
    body = <>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm font-medium">Lokasi servis</p>
        {fromMaster ? <Badge variant="outline">Data master {data.brand === 'loncin' ? 'Loncin' : 'Shark'}</Badge>
          : hasMaster && canManage ? <Button type="button" variant="ghost" size="sm" onClick={() => setForm((current) => ({ ...current, service_center_name: master.name, service_center_address: master.address, service_center_phone: master.phone }))}><RotateCcw />Pakai data master</Button>
          : null}
      </div>
      {!hasMaster && <Notice>Belum ada data master service center untuk kabupaten ini. Isi manual.</Notice>}
      <FormField label="Nama service center" name="service_center_name" value={form.service_center_name} onChange={set('service_center_name')} />
      <FormField label="Alamat" name="service_center_address" value={form.service_center_address} onChange={set('service_center_address')} />
      <FormField label="Telepon" name="service_center_phone" value={form.service_center_phone} onChange={set('service_center_phone')} />
    </>;
  } else if (info.kind === 'seremoni') {
    body = <FormField label="Nama" name="seremoni_name" value={form.seremoni_name} onChange={set('seremoni_name')} hint="Misalnya anggota Komisi XII DPR RI yang hadir. Dicetak huruf besar." />;
  } else if (info.kind === 'plakat') {
    body = <>
      <FormField label="Nama" name="plakat_name" value={form.plakat_name} onChange={set('plakat_name')} hint="Dicetak huruf besar." />
      <FormField label="Tempat" name="plakat_place" value={form.plakat_place} placeholder={data.short_name} onChange={set('plakat_place')} hint={`Kosongkan untuk memakai "${data.short_name}".`} />
      <FormField label="Tanggal" name="plakat_date" type="date" value={form.plakat_date} onChange={set('plakat_date')} />
    </>;
  } else {
    body = <Notice>Tidak ada isian. Banner ini memakai template resmi dengan logo kabupaten.</Notice>;
  }

  return <Card>
    <CardHeader className="border-b"><CardTitle className="text-base">Isian {info.label}</CardTitle></CardHeader>
    <CardContent className="pt-4">
      <fieldset className="grid gap-4" disabled={!canManage}>{body}</fieldset>
      {canManage && fields.length > 0 && <div className="mt-5 flex justify-end">
        <Button disabled={!dirty || save.isPending} onClick={() => save.mutate()}><Save />{save.isPending ? 'Menyimpan...' : 'Simpan'}</Button>
      </div>}
    </CardContent>
  </Card>;
}

function Notice({ children }: { children: ReactNode }) {
  return <p className="flex gap-2 rounded-md bg-muted/60 px-3 py-2 text-sm text-muted-foreground"><Info className="mt-0.5 size-4 shrink-0" aria-hidden="true" />{children}</p>;
}

function BannerPreview({ data, info }: { data: Overview; info: KindInfo }) {
  const path = `/api/v1/banners/${info.kind}.pdf?schedule_id=${encodeURIComponent(data.schedule_id)}`;
  // Keyed on the saved data, so the preview follows every save.
  const pdf = useQuery({
    queryKey: ['banners', data.schedule_id, 'pdf', info.kind, data.settings, data.logo_updated_at],
    queryFn: () => apiBlobRequest(`${path}&inline=1`),
    staleTime: Infinity,
  });
  const missing = !data.has_logo ? 'Logo kabupaten belum ada, slot logo dibiarkan kosong.'
    : info.kind === 'seremoni' && !data.settings.seremoni_name ? 'Nama seremoni belum diisi.'
    : info.kind === 'plakat' && (!data.settings.plakat_name || !data.settings.plakat_date) ? 'Nama atau tanggal plakat belum diisi.'
    : info.kind === 'backdrop' && !data.service_center.name ? 'Service center belum diisi.' : '';

  return <Card className="min-w-0 xl:sticky xl:top-20">
    <CardHeader className="gap-2 border-b">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="min-w-0">
          <CardTitle className="text-base">Pratinjau {info.label}</CardTitle>
          <p className="truncate text-sm text-muted-foreground">{info.size} · {info.filename}</p>
        </div>
        <a className={buttonVariants({ size: 'sm' })} href={path} download={info.filename}><Download />Unduh PDF</a>
      </div>
      {missing && <p className="rounded-md bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-400">{missing}</p>}
    </CardHeader>
    <CardContent className="pt-4">
      {pdf.isPending ? <DataState kind="loading" title="Menyusun banner" description="Template ukuran asli, mohon tunggu sebentar." />
        : pdf.isError ? <DataState kind="error" title="Banner belum dapat disusun" description={pdf.error instanceof ApiError ? pdf.error.message : 'Periksa koneksi lalu coba lagi.'} action={{ label: 'Coba lagi', onClick: () => void pdf.refetch() }} />
        : <Suspense fallback={<DataState kind="loading" title="Membuka pratinjau" description="Menyiapkan penampil PDF." />}>
          <PdfCanvasPreview blob={pdf.data} label={`${info.label} ${data.display_name}`} className="grid gap-3 rounded-lg border bg-muted/30 p-2" />
        </Suspense>}
    </CardContent>
  </Card>;
}
