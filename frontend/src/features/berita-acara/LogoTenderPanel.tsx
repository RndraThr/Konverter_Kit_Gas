import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowDown, ArrowUp, Eye, EyeOff, ImagePlus, Images } from 'lucide-react';
import { toast } from 'sonner';
import { DataState } from '@/components/DataState';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { apiRequest } from '@/lib/api';
import { useCan } from '@/lib/permissions';
import type { DataResponse } from '../programs/types';
import type { BaLogo } from './types';

type BrandingLogoResponse = Omit<BaLogo, 'content_url'>;

function contentURL(programID: string, logoID: string) {
  return `/api/v1/bast/branding/logos/${logoID}/content?program_id=${encodeURIComponent(programID)}`;
}

export function LogoTenderPanel({ programID }: { programID: string }) {
  const canManage = useCan('bast.manage');
  const queryClient = useQueryClient();
  const queryKey = ['bast', 'branding', programID];
  const logos = useQuery({
    queryKey,
    queryFn: async () => {
      const response = await apiRequest<DataResponse<BrandingLogoResponse[]>>(`/api/v1/bast/branding?program_id=${encodeURIComponent(programID)}`);
      return response.data
        .map((logo): BaLogo => ({ ...logo, content_url: contentURL(programID, logo.id) }))
        .sort((left, right) => left.sort_order - right.sort_order || left.id.localeCompare(right.id));
    },
    enabled: Boolean(programID),
  });
  const refresh = () => queryClient.invalidateQueries({ queryKey });
  const patchLogo = (logo: BaLogo, patch: Partial<Pick<BaLogo, 'sort_order' | 'max_width_mm' | 'max_height_mm' | 'is_visible'>>) => apiRequest(`/api/v1/bast/branding/logos/${logo.id}`, {
    method: 'PATCH',
    body: JSON.stringify({
      program_id: programID,
      sort_order: patch.sort_order ?? logo.sort_order,
      max_width_mm: patch.max_width_mm ?? logo.max_width_mm,
      max_height_mm: patch.max_height_mm ?? logo.max_height_mm,
      is_visible: patch.is_visible ?? logo.is_visible,
    }),
  });
  const upload = useMutation({
    mutationFn: (file: File) => {
      const items = logos.data ?? [];
      const nextOrder = items.reduce((maximum, item) => Math.max(maximum, item.sort_order), -1) + 1;
      const body = new FormData();
      body.set('program_id', programID);
      body.set('slot_code', `logo_${items.length + 1}`);
      body.set('sort_order', String(nextOrder));
      body.set('max_width_mm', '35');
      body.set('max_height_mm', '18');
      body.set('file', file);
      return apiRequest('/api/v1/bast/branding/logos', { method: 'POST', body });
    },
    onSuccess: () => { refresh(); toast.success('Logo tender berhasil diunggah.'); },
    onError: () => toast.error('Logo tender belum dapat diunggah.'),
  });
  const update = useMutation({
    mutationFn: ({ logo, patch }: { logo: BaLogo; patch: Partial<Pick<BaLogo, 'sort_order' | 'is_visible'>> }) => patchLogo(logo, patch),
    onSuccess: refresh,
    onError: () => toast.error('Logo tender belum dapat diperbarui.'),
  });
  const move = useMutation({
    mutationFn: async ({ logo, sibling }: { logo: BaLogo; sibling: BaLogo }) => {
      await Promise.all([patchLogo(logo, { sort_order: sibling.sort_order }), patchLogo(sibling, { sort_order: logo.sort_order })]);
    },
    onSuccess: refresh,
    onError: () => toast.error('Urutan logo tender belum dapat diperbarui.'),
  });

  return <Card aria-label="Logo Tender" className="mb-5">
    <CardHeader className="border-b sm:grid-cols-[1fr_auto]">
      <div>
        <CardTitle className="flex items-center gap-2"><Images className="size-5 text-primary" aria-hidden="true" />Logo Tender</CardTitle>
        <CardDescription>Logo berlaku untuk seluruh jenis Berita Acara dalam program yang dipilih.</CardDescription>
      </div>
      {canManage && <label className="mt-3 inline-flex h-10 cursor-pointer items-center justify-center gap-2 rounded-lg border bg-background px-4 text-sm font-medium shadow-xs hover:bg-muted sm:mt-0">
        <ImagePlus className="size-4" aria-hidden="true" />Unggah logo
        <input className="sr-only" aria-label="Unggah logo tender" type="file" accept="image/png,image/jpeg" disabled={upload.isPending} onChange={(event) => { const file = event.target.files?.[0]; if (file) upload.mutate(file); event.currentTarget.value = ''; }} />
      </label>}
    </CardHeader>
    <CardContent>
      {logos.isPending ? <DataState kind="loading" title="Memuat logo tender" description="Mengambil susunan branding Berita Acara." />
        : logos.isError ? <DataState kind="error" title="Logo tender belum dapat dimuat" description="Periksa koneksi atau hak akses, lalu coba kembali." action={{ label: 'Coba lagi', onClick: () => logos.refetch() }} />
          : logos.data.length === 0 ? <DataState kind="empty" title="Belum ada logo tender" description={canManage ? 'Unggah minimal satu logo aktif agar Berita Acara dapat dibuat.' : 'Logo tender belum dikonfigurasi oleh pengelola.'} />
            : <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
              {logos.data.map((logo, index) => <article key={logo.id} className="flex min-w-0 items-center gap-3 rounded-lg border bg-background p-3">
                <div className="grid size-20 shrink-0 place-items-center rounded-md bg-muted/60 p-2">
                  <img className={`max-h-full max-w-full object-contain ${logo.is_visible ? '' : 'opacity-35 grayscale'}`} src={logo.content_url} alt={logo.original_filename} />
                </div>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium" title={logo.original_filename}>{logo.original_filename}</p>
                  <div className="mt-1 flex flex-wrap items-center gap-1.5"><Badge variant="outline">Urutan {index + 1}</Badge><Badge variant={logo.is_visible ? 'default' : 'secondary'}>{logo.is_visible ? 'Aktif' : 'Tersembunyi'}</Badge></div>
                  {canManage && <div className="mt-3 flex gap-1">
                    <Button size="icon-sm" variant="outline" aria-label={`Naikkan ${logo.original_filename}`} disabled={index === 0 || move.isPending} onClick={() => move.mutate({ logo, sibling: logos.data[index - 1] })}><ArrowUp /></Button>
                    <Button size="icon-sm" variant="outline" aria-label={`Turunkan ${logo.original_filename}`} disabled={index === logos.data.length - 1 || move.isPending} onClick={() => move.mutate({ logo, sibling: logos.data[index + 1] })}><ArrowDown /></Button>
                    <Button size="icon-sm" variant="outline" aria-label={`${logo.is_visible ? 'Sembunyikan' : 'Tampilkan'} ${logo.original_filename}`} disabled={update.isPending} onClick={() => update.mutate({ logo, patch: { is_visible: !logo.is_visible } })}>{logo.is_visible ? <EyeOff /> : <Eye />}</Button>
                  </div>}
                </div>
              </article>)}
            </div>}
    </CardContent>
  </Card>;
}
