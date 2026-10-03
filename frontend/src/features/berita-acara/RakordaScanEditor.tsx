import { ArrowDown, ArrowUp, Camera, Check, FileImage, RotateCw, ScanLine, Trash2, WandSparkles } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import { buildJpegPdf, defaultScanCorners, processScanImage, type ScanColorMode, type ScanCorners } from './rakordaScan';

export type ScanDraftPage = {
  id: string;
  file: File;
  sourceUrl: string;
  corners: ScanCorners;
  rotation: 0 | 90 | 180 | 270;
  colorMode: ScanColorMode;
  processed?: { blob: Blob; url: string; width: number; height: number };
};

type Props = { disabled?: boolean; onPdfReady: (file: File) => Promise<void> };
const colorModes: { value: ScanColorMode; label: string; description: string }[] = [
  { value: 'original', label: 'Asli', description: 'Warna kamera tanpa koreksi.' },
  { value: 'enhanced', label: 'Warna ditingkatkan', description: 'Kontras dan warna lebih bersih, tinta tetap berwarna.' },
  { value: 'grayscale', label: 'Abu-abu', description: 'Tampilan dokumen netral dan ringan.' },
  { value: 'black-white', label: 'Hitam-putih', description: 'Kontras tinggi untuk teks dan tanda tangan.' },
];

export function RakordaScanEditor({ disabled, onPdfReady }: Props) {
  const cameraRef = useRef<HTMLInputElement>(null);
  const galleryRef = useRef<HTMLInputElement>(null);
  const [pages, setPages] = useState<ScanDraftPage[]>([]);
  const [selectedID, setSelectedID] = useState('');
  const [processing, setProcessing] = useState(false);
  const pagesRef = useRef<ScanDraftPage[]>([]);
  const selected = pages.find((page) => page.id === selectedID) ?? pages[0];

  useEffect(() => { pagesRef.current = pages; }, [pages]);
  useEffect(() => () => pagesRef.current.forEach((page) => { URL.revokeObjectURL(page.sourceUrl); if (page.processed) URL.revokeObjectURL(page.processed.url); }), []);

  const addFiles = (files: FileList | null) => {
    const additions = Array.from(files ?? []).filter((file) => file.type === 'image/jpeg' || file.type === 'image/png').map((file) => ({
      id: crypto.randomUUID(), file, sourceUrl: URL.createObjectURL(file), corners: defaultScanCorners(), rotation: 0 as const, colorMode: 'enhanced' as const,
    }));
    if (!additions.length) return;
    setPages((current) => [...current, ...additions]);
    setSelectedID((current) => current || additions[0].id);
  };
  const updateSelected = (change: Partial<ScanDraftPage>) => setPages((current) => current.map((page) => page.id === selected?.id ? { ...page, ...change, processed: change.processed ?? undefined } : page));
  const move = (index: number, direction: -1 | 1) => setPages((current) => {
    const target = index + direction;
    if (target < 0 || target >= current.length) return current;
    const next = [...current]; [next[index], next[target]] = [next[target], next[index]]; return next;
  });
  const remove = (id: string) => setPages((current) => {
    const removed = current.find((page) => page.id === id);
    if (removed) { URL.revokeObjectURL(removed.sourceUrl); if (removed.processed) URL.revokeObjectURL(removed.processed.url); }
    const next = current.filter((page) => page.id !== id);
    if (id === selectedID) setSelectedID(next[0]?.id ?? '');
    return next;
  });
  const applySelected = async () => {
    if (!selected) return;
    setProcessing(true);
    try {
      const output = await processScanImage(selected.file, selected);
      if (selected.processed) URL.revokeObjectURL(selected.processed.url);
      updateSelected({ processed: { ...output, url: URL.createObjectURL(output.blob) } });
    } finally { setProcessing(false); }
  };
  const assemble = async () => {
    if (!pages.length) return;
    setProcessing(true);
    try {
      const rendered = [];
      for (const page of pages) {
        const output = page.processed ?? { ...(await processScanImage(page.file, page)), url: '' };
        rendered.push({ bytes: new Uint8Array(await output.blob.arrayBuffer()), width: output.width, height: output.height });
      }
      const pdf = buildJpegPdf(rendered);
      const pdfBuffer = new ArrayBuffer(pdf.byteLength);
      new Uint8Array(pdfBuffer).set(pdf);
      await onPdfReady(new File([pdfBuffer], `RAKORDA-SCAN-${new Date().toISOString().slice(0, 10)}.pdf`, { type: 'application/pdf' }));
    } finally { setProcessing(false); }
  };

  return <Card className="overflow-hidden">
    <CardHeader className="border-b bg-muted/20"><div className="flex flex-wrap items-center justify-between gap-3"><div><CardTitle className="flex items-center gap-2 text-base"><ScanLine className="size-4 text-primary" aria-hidden="true" />Pindai dokumen selesai</CardTitle><p className="mt-1 text-sm text-muted-foreground">Rapikan foto, pilih warna, urutkan halaman, lalu gabungkan menjadi PDF.</p></div><Badge variant="outline">{pages.length} halaman</Badge></div></CardHeader>
    <CardContent className="grid gap-4 pt-5">
      <div className="flex flex-wrap gap-2">
        <input ref={cameraRef} className="sr-only" type="file" accept="image/jpeg,image/png" capture="environment" onChange={(event) => { addFiles(event.target.files); event.target.value = ''; }} />
        <input ref={galleryRef} className="sr-only" type="file" accept="image/jpeg,image/png" multiple onChange={(event) => { addFiles(event.target.files); event.target.value = ''; }} />
        <Button type="button" variant="outline" disabled={disabled} onClick={() => cameraRef.current?.click()}><Camera aria-hidden="true" />Buka kamera</Button>
        <Button type="button" variant="outline" disabled={disabled} onClick={() => galleryRef.current?.click()}><FileImage aria-hidden="true" />Pilih galeri</Button>
      </div>

      {pages.length === 0 ? <div className="rounded-xl border border-dashed bg-muted/20 px-5 py-10 text-center"><ScanLine className="mx-auto size-8 text-muted-foreground" aria-hidden="true" /><strong className="mt-3 block text-sm">Belum ada halaman</strong><p className="mt-1 text-sm text-muted-foreground">Ambil foto tiap halaman daftar hadir atau pilih beberapa foto sekaligus.</p></div> : <div className="grid min-w-0 gap-4 xl:grid-cols-[12rem_minmax(0,1fr)]">
        <div className="flex gap-2 overflow-x-auto pb-2 xl:grid xl:max-h-[42rem] xl:overflow-y-auto xl:overflow-x-hidden xl:pr-1">
          {pages.map((page, index) => <div key={page.id} className={cn('min-w-40 rounded-lg border p-2 transition-colors xl:min-w-0', page.id === selected?.id && 'border-primary bg-primary/5')}>
            <button type="button" className="block w-full text-left" onClick={() => setSelectedID(page.id)} aria-pressed={page.id === selected?.id}>
              <img alt={`Halaman ${index + 1}`} src={page.processed?.url || page.sourceUrl} className="aspect-[3/4] w-full rounded-md bg-muted object-cover" />
              <span className="mt-2 flex items-center justify-between text-xs"><strong>Halaman {index + 1}</strong>{page.processed && <Check className="size-3.5 text-emerald-600" aria-label="Sudah diproses" />}</span>
            </button>
            <div className="mt-2 grid grid-cols-3 gap-1">
              <Button size="icon-sm" variant="ghost" aria-label="Naikkan halaman" disabled={index === 0} onClick={() => move(index, -1)}><ArrowUp /></Button>
              <Button size="icon-sm" variant="ghost" aria-label="Turunkan halaman" disabled={index === pages.length - 1} onClick={() => move(index, 1)}><ArrowDown /></Button>
              <Button size="icon-sm" variant="ghost" aria-label="Hapus halaman" onClick={() => remove(page.id)}><Trash2 /></Button>
            </div>
          </div>)}
        </div>
        {selected && <div className="grid min-w-0 gap-4">
          <CornerEditor page={selected} onCornersChange={(corners) => updateSelected({ corners })} />
          <div className="grid gap-3">
            <div><strong className="text-sm">Mode warna</strong><p className="text-xs text-muted-foreground">Warna ditingkatkan direkomendasikan untuk foto dokumen.</p></div>
            <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">{colorModes.map((mode) => <button key={mode.value} type="button" aria-pressed={selected.colorMode === mode.value} onClick={() => updateSelected({ colorMode: mode.value })} className="min-h-20 rounded-lg border p-3 text-left transition-colors hover:bg-muted aria-pressed:border-primary aria-pressed:bg-primary/5"><span className="flex items-center gap-2 text-sm font-semibold">{mode.value === 'enhanced' && <WandSparkles className="size-4 text-primary" aria-hidden="true" />}{mode.label}</span><span className="mt-1 block text-xs leading-5 text-muted-foreground">{mode.description}</span></button>)}</div>
          </div>
          <div className="flex flex-wrap gap-2"><Button type="button" variant="outline" onClick={() => updateSelected({ rotation: ((selected.rotation + 90) % 360) as ScanDraftPage['rotation'] })}><RotateCw aria-hidden="true" />Putar 90°</Button><Button type="button" disabled={processing} onClick={applySelected}><WandSparkles aria-hidden="true" />{processing ? 'Memproses...' : 'Terapkan ke halaman'}</Button></div>
        </div>}
      </div>}
      {pages.length > 0 && <div className="flex justify-end border-t pt-4"><Button type="button" disabled={processing || disabled} onClick={assemble}><ScanLine aria-hidden="true" />{processing ? 'Menyusun PDF...' : 'Gabungkan & unggah PDF'}</Button></div>}
    </CardContent>
  </Card>;
}

function CornerEditor({ page, onCornersChange }: { page: ScanDraftPage; onCornersChange: (corners: ScanCorners) => void }) {
  const frameRef = useRef<HTMLDivElement>(null);
  const moveCorner = (index: number, clientX: number, clientY: number) => {
    const rect = frameRef.current?.getBoundingClientRect(); if (!rect) return;
    const corners = [...page.corners] as ScanCorners;
    corners[index] = { x: Math.max(0, Math.min(1, (clientX - rect.left) / rect.width)), y: Math.max(0, Math.min(1, (clientY - rect.top) / rect.height)) };
    onCornersChange(corners);
  };
  return <div><div className="mb-2 flex items-center justify-between gap-3"><div><strong className="text-sm">Atur empat sudut kertas</strong><p className="text-xs text-muted-foreground">Geser titik hijau mengikuti tepi halaman.</p></div>{page.processed && <Badge className="bg-emerald-600 text-white">Diterapkan</Badge>}</div>
    <div ref={frameRef} className="relative mx-auto max-h-[34rem] w-fit overflow-hidden rounded-xl bg-neutral-950 shadow-inner">
      <img alt="Halaman yang sedang disunting" src={page.sourceUrl} className="block max-h-[34rem] max-w-full select-none object-contain opacity-85" draggable={false} />
      <svg className="pointer-events-none absolute inset-0 size-full" viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true"><polygon points={page.corners.map((point) => `${point.x * 100},${point.y * 100}`).join(' ')} fill="rgba(34,197,94,.12)" stroke="rgb(34,197,94)" strokeWidth=".6" strokeDasharray="2 1" vectorEffect="non-scaling-stroke" /></svg>
      {page.corners.map((point, index) => <button key={index} type="button" aria-label={`Sudut ${index + 1}; gunakan tombol panah untuk menyesuaikan`} className="absolute size-7 -translate-x-1/2 -translate-y-1/2 touch-none rounded-full border-4 border-white bg-emerald-600 shadow-lg focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-ring" style={{ left: `${point.x * 100}%`, top: `${point.y * 100}%` }} onPointerDown={(event) => { event.currentTarget.setPointerCapture(event.pointerId); }} onPointerMove={(event) => { if (event.currentTarget.hasPointerCapture(event.pointerId)) moveCorner(index, event.clientX, event.clientY); }} onKeyDown={(event) => { const step = event.shiftKey ? .02 : .005; const delta = { ArrowLeft: [-step, 0], ArrowRight: [step, 0], ArrowUp: [0, -step], ArrowDown: [0, step] }[event.key]; if (!delta) return; event.preventDefault(); const corners = [...page.corners] as ScanCorners; corners[index] = { x: Math.max(0, Math.min(1, point.x + delta[0])), y: Math.max(0, Math.min(1, point.y + delta[1])) }; onCornersChange(corners); }} />)}
    </div>
  </div>;
}
