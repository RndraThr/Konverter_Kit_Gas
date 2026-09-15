import { Layers3, MapPinned, SlidersHorizontal } from 'lucide-react';
import { PageHeader } from '../../components/PageHeader';
import { Badge } from '../../components/ui/badge';
import { Card, CardContent, CardHeader } from '../../components/ui/card';
import { Separator } from '../../components/ui/separator';

const mapPoints = [
  { className: 'left-[18%] top-[58%]', size: 'size-3' },
  { className: 'left-[47%] top-[34%]', size: 'size-4' },
  { className: 'right-[22%] top-[62%]', size: 'size-3' },
];

export function DistributionMapPage() {
  return <div className="space-y-6">
    <PageHeader title="Map Distribusi" description="Pemetaan sebaran penerima dan progres distribusi berdasarkan wilayah." />

    <section aria-label="Ruang kerja Map Distribusi" className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_300px]">
      <Card className="gap-0 overflow-hidden py-0 shadow-none">
        <CardHeader className="flex-row items-center justify-between border-b py-4">
          <div className="flex items-center gap-3"><MapPinned className="size-5 text-primary" aria-hidden="true" /><h2 className="text-base leading-snug font-medium">Area peta</h2></div>
          <Badge variant="secondary">Modul sedang disiapkan</Badge>
        </CardHeader>
        <CardContent className="p-0">
          <div className="relative grid min-h-[420px] place-items-center overflow-hidden bg-[linear-gradient(to_right,var(--border)_1px,transparent_1px),linear-gradient(to_bottom,var(--border)_1px,transparent_1px)] bg-[size:48px_48px]">
            <div className="absolute inset-8 rounded-[42%_58%_46%_54%/55%_38%_62%_45%] border-2 border-primary/25 bg-primary/6" aria-hidden="true" />
            {mapPoints.map((point) => <span key={point.className} className={`absolute ${point.className} ${point.size} rounded-full bg-primary ring-4 ring-primary/15`} aria-hidden="true" />)}
            <div className="relative mx-6 max-w-md rounded-xl border bg-background/95 p-6 text-center shadow-sm backdrop-blur">
              <span className="mx-auto grid size-12 place-items-center rounded-full bg-secondary text-secondary-foreground"><Layers3 className="size-6" aria-hidden="true" /></span>
              <h2 className="mt-4 text-lg font-semibold">Peta distribusi akan tampil di sini</h2>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">Titik penerima, cakupan kabupaten, dan status penyaluran akan dipetakan pada tahap pengembangan berikutnya.</p>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card className="gap-0 py-0 shadow-none">
        <CardHeader className="border-b py-4"><div className="flex items-center gap-3"><SlidersHorizontal className="size-5 text-primary" aria-hidden="true" /><h2 className="text-base leading-snug font-medium">Kontrol peta</h2></div></CardHeader>
        <CardContent className="space-y-5 py-5">
          <div><p className="text-sm font-medium">Wilayah</p><p className="mt-1 text-sm leading-6 text-muted-foreground">Filter provinsi, kabupaten, dan kecamatan.</p></div>
          <Separator />
          <div><p className="text-sm font-medium">Program</p><p className="mt-1 text-sm leading-6 text-muted-foreground">Bandingkan sebaran berdasarkan jenis bantuan.</p></div>
          <Separator />
          <div><p className="text-sm font-medium">Status distribusi</p><p className="mt-1 text-sm leading-6 text-muted-foreground">Pantau paket yang belum dan sudah disalurkan.</p></div>
        </CardContent>
      </Card>
    </section>
  </div>;
}
