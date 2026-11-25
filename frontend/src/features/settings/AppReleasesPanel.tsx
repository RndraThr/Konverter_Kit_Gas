import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Download, Smartphone, Trash2, Upload } from 'lucide-react';
import { FormEvent, useState } from 'react';
import { toast } from 'sonner';
import { DataState } from '../../components/DataState';
import { FormField } from '../../components/FormField';
import { Badge } from '../../components/ui/badge';
import { Button } from '../../components/ui/button';
import { Checkbox } from '../../components/ui/checkbox';
import { Label } from '../../components/ui/label';
import { apiRequest } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { uploadRequest } from '../../lib/upload';

export type AppRelease = {
  id: string;
  version_name: string;
  version_code: number;
  notes: string;
  mandatory: boolean;
  size_bytes: number;
  uploaded_by_name?: string;
  created_at: string;
  /** Delta updates to this release from earlier ones (made by the server). */
  patches?: { from_version_name: string; from_version_code: number; size_bytes: number; ready: boolean }[];
  /** The server is still making its patches: phones are not offered it yet. */
  patches_pending?: boolean;
  /** APKs for other phone types: 32-bit phones (armeabi-v7a) get theirs. */
  variants?: { abi: string; size_bytes: number }[];
};

const abiLabels: Record<string, string> = { 'armeabi-v7a': '32-bit', x86_64: 'x86_64' };

const queryKey = ['app-releases'];

function megabytes(bytes: number) {
  return `${(bytes / (1024 * 1024)).toLocaleString('id-ID', { maximumFractionDigits: 1 })} MB`;
}

/**
 * Rilis aplikasi mobile di luar Play Store: admin mengunggah APK bertanda
 * tangan rilis beserta versinya; HP petugas menawarkan pemasangan, dan rilis
 * wajib harus dipasang sebelum aplikasi bisa dipakai.
 */
export function AppReleasesPanel() {
  const canManage = useCan('settings.manage');
  const client = useQueryClient();
  const releases = useQuery({
    queryKey,
    queryFn: () => apiRequest<{ data: AppRelease[] }>('/api/v1/app/releases'),
    // Follow the patches being made until the release is offered.
    refetchInterval: (query) => (query.state.data?.data.some((release) => release.patches_pending) ? 10_000 : false),
  });
  const latest = releases.data?.data[0];

  const [versionName, setVersionName] = useState('');
  const [versionCode, setVersionCode] = useState('');
  const [notes, setNotes] = useState('');
  const [mandatory, setMandatory] = useState(false);
  const [file, setFile] = useState<File | null>(null);
  const [variantFile, setVariantFile] = useState<File | null>(null);
  const [progress, setProgress] = useState<number | null>(null);
  const [fileKey, setFileKey] = useState(0);

  const upload = useMutation({
    mutationFn: () => {
      const form = new FormData();
      form.append('version_name', versionName.trim());
      form.append('version_code', versionCode.trim());
      form.append('notes', notes.trim());
      form.append('mandatory', mandatory ? 'true' : 'false');
      form.append('apk', file as File);
      if (variantFile) form.append('apk_variant', variantFile);
      setProgress(0);
      return uploadRequest<{ data: AppRelease }>('/api/v1/app/releases', form, { onProgress: setProgress });
    },
    onSuccess: ({ data }) => {
      toast.success(`Versi ${data.version_name} dirilis. HP petugas akan menerima pemberitahuan update.`);
      setVersionName('');
      setVersionCode('');
      setNotes('');
      setMandatory(false);
      setFile(null);
      setVariantFile(null);
      setFileKey((key) => key + 1);
      void client.invalidateQueries({ queryKey });
    },
    onError: (error: Error) => toast.error(error.message || 'Rilis belum dapat diunggah.'),
    onSettled: () => setProgress(null),
  });

  const toggleMandatory = useMutation({
    mutationFn: (release: AppRelease) => apiRequest(`/api/v1/app/releases/${release.id}`, { method: 'PATCH', body: JSON.stringify({ mandatory: !release.mandatory }) }),
    onSuccess: () => void client.invalidateQueries({ queryKey }),
    onError: (error: Error) => toast.error(error.message),
  });

  const addVariant = useMutation({
    mutationFn: ({ release, apk }: { release: AppRelease; apk: File }) => {
      const form = new FormData();
      form.append('apk', apk);
      return uploadRequest<{ data: { abi: string } }>(`/api/v1/app/releases/${release.id}/variants`, form, {});
    },
    onSuccess: ({ data }, { release }) => {
      toast.success(`APK ${abiLabels[data.abi] ?? data.abi} versi ${release.version_name} ditambahkan.`);
      void client.invalidateQueries({ queryKey });
    },
    onError: (error: Error) => toast.error(error.message || 'APK belum dapat diunggah.'),
  });

  const remove = useMutation({
    mutationFn: (release: AppRelease) => apiRequest(`/api/v1/app/releases/${release.id}`, { method: 'DELETE' }),
    onSuccess: () => {
      toast.success('Rilis dihapus.');
      void client.invalidateQueries({ queryKey });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const codeNumber = Number(versionCode);
  const codeTooLow = latest != null && versionCode !== '' && codeNumber <= latest.version_code;
  const ready = versionName.trim() !== '' && Number.isInteger(codeNumber) && codeNumber > 0 && !codeTooLow && file != null;

  return <section className="overflow-hidden rounded-lg border bg-card" aria-labelledby="app-releases-title">
    <div className="flex items-center gap-3 border-b px-5 py-4">
      <span className="grid size-9 place-items-center rounded-lg bg-primary/10 text-primary"><Smartphone className="size-4" aria-hidden="true" /></span>
      <div>
        <h2 id="app-releases-title" className="font-semibold">Rilis aplikasi mobile</h2>
        <p className="mt-0.5 text-sm text-muted-foreground">Unggah APK baru; HP petugas menawarkan pemasangan. Rilis wajib harus dipasang sebelum aplikasi dipakai.</p>
      </div>
    </div>

    {canManage ? <form className="grid gap-4 border-b p-5 sm:grid-cols-2" onSubmit={(event: FormEvent) => { event.preventDefault(); if (ready) upload.mutate(); }}>
      <FormField label="Nama versi" name="version_name" placeholder="1.1.0" value={versionName} maxLength={40} onChange={(event) => setVersionName(event.target.value)} hint="Sama dengan versi di pubspec.yaml (sebelum tanda +)." />
      <FormField
        label="Kode versi"
        name="version_code"
        type="number"
        min={1}
        placeholder={latest ? String(latest.version_code + 1) : '2'}
        value={versionCode}
        onChange={(event) => setVersionCode(event.target.value)}
        hint={codeTooLow ? `Harus lebih dari ${latest?.version_code}.` : 'Angka setelah tanda + di pubspec.yaml; selalu naik.'}
      />
      <div className="grid gap-2 sm:col-span-2">
        <Label htmlFor="release-notes">Catatan perubahan</Label>
        <textarea id="release-notes" rows={3} maxLength={4000} className="min-h-20 rounded-lg border border-input bg-transparent px-3 py-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50" value={notes} onChange={(event) => setNotes(event.target.value)} placeholder="Apa yang baru di versi ini (tampil di HP petugas)." />
      </div>
      <div className="grid gap-2">
        <Label htmlFor="release-apk">Berkas APK (64-bit)</Label>
        <input key={fileKey} id="release-apk" type="file" accept=".apk,application/vnd.android.package-archive" className="text-sm" onChange={(event) => setFile(event.target.files?.[0] ?? null)} />
        <p className="text-xs text-muted-foreground">app-arm64-v8a-release.apk, untuk hampir semua HP.</p>
      </div>
      <div className="grid gap-2">
        <Label htmlFor="release-apk-32">APK 32-bit (opsional)</Label>
        <input key={fileKey} id="release-apk-32" type="file" accept=".apk,application/vnd.android.package-archive" className="text-sm" onChange={(event) => setVariantFile(event.target.files?.[0] ?? null)} />
        <p className="text-xs text-muted-foreground">app-armeabi-v7a-release.apk, untuk HP Android Go yang hanya bisa aplikasi 32-bit. HP memilih otomatis.</p>
      </div>
      <label className="flex items-start gap-2 text-sm sm:self-end">
        <Checkbox checked={mandatory} onCheckedChange={(value) => setMandatory(value === true)} />
        <span><strong className="font-medium">Wajib dipasang</strong><br /><span className="text-muted-foreground">Aplikasi terkunci sampai versi ini dipasang.</span></span>
      </label>
      <div className="flex items-center justify-end gap-3 sm:col-span-2">
        {progress != null ? <span className="text-sm text-muted-foreground">Mengunggah {progress}%</span> : null}
        <Button type="submit" disabled={!ready || upload.isPending}><Upload aria-hidden="true" />{upload.isPending ? 'Mengunggah…' : 'Rilis versi'}</Button>
      </div>
    </form> : null}

    {releases.isError
      ? <div className="p-5"><DataState kind="error" title="Daftar rilis belum dapat dimuat" description="Periksa koneksi lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => releases.refetch() }} /></div>
      : releases.isPending
        ? <div className="p-5"><DataState kind="loading" title="Memuat rilis" description="Mengambil daftar versi." /></div>
        : releases.data.data.length === 0
          ? <p className="p-5 text-sm text-muted-foreground">Belum ada rilis. HP petugas tidak menerima tawaran update.</p>
          : <ul className="divide-y">{releases.data.data.map((release, index) => <li key={release.id} className="flex flex-col gap-2 px-5 py-4 sm:flex-row sm:items-start sm:justify-between">
            <div className="min-w-0">
              <p className="flex flex-wrap items-center gap-2 font-medium">
                Versi {release.version_name} <span className="text-sm font-normal text-muted-foreground">(kode {release.version_code})</span>
                {index === 0 ? <Badge>Terbaru</Badge> : null}
                {release.mandatory ? <Badge variant="destructive">Wajib</Badge> : null}
              </p>
              {release.notes ? <p className="mt-1 whitespace-pre-line text-sm text-muted-foreground">{release.notes}</p> : null}
              <p className="mt-1 text-xs text-muted-foreground">{megabytes(release.size_bytes)} · {new Date(release.created_at).toLocaleString('id-ID')}{release.uploaded_by_name ? ` · ${release.uploaded_by_name}` : ''}</p>
              {release.patches_pending ? <p className="mt-1 text-xs text-muted-foreground">Menyiapkan update delta. Versi ini ditawarkan ke HP setelah selesai (sekitar 1–3 menit).</p> : null}
              {release.patches?.some((patch) => patch.ready) ? <p className="mt-1 text-xs text-muted-foreground">
                Update delta: {release.patches.filter((patch) => patch.ready).map((patch) => `dari ${patch.from_version_name} ${megabytes(patch.size_bytes)}`).join(' · ')}
              </p> : null}
              <p className="mt-1 text-xs text-muted-foreground">
                {release.variants?.length
                  ? <>HP 32-bit: {release.variants.map((variant) => <a key={variant.abi} className="underline-offset-2 hover:underline" href={`/api/v1/app/releases/${release.id}/apk?abi=${variant.abi}`}>APK {abiLabels[variant.abi] ?? variant.abi} {megabytes(variant.size_bytes)}</a>)}</>
                  : 'Belum ada APK 32-bit: HP 32-bit tetap di versi lamanya.'}
              </p>
            </div>
            <div className="flex shrink-0 gap-2">
              <Button variant="outline" size="sm" nativeButton={false} render={<a href={`/api/v1/app/releases/${release.id}/apk`} />}><Download aria-hidden="true" />APK</Button>
              {canManage ? <>
                <Button variant="outline" size="sm" disabled={addVariant.isPending} nativeButton={false} render={<label />} title="Tambah atau ganti APK 32-bit (app-armeabi-v7a-release.apk)">
                  <Upload aria-hidden="true" />{release.variants?.length ? 'Ganti 32-bit' : 'APK 32-bit'}
                  <input type="file" accept=".apk,application/vnd.android.package-archive" className="sr-only" aria-label={`APK 32-bit versi ${release.version_name}`}
                    onChange={(event) => { const apk = event.target.files?.[0]; event.target.value = ''; if (apk) addVariant.mutate({ release, apk }); }} />
                </Button>
                <Button variant="outline" size="sm" disabled={toggleMandatory.isPending} onClick={() => toggleMandatory.mutate(release)}>{release.mandatory ? 'Jadikan opsional' : 'Jadikan wajib'}</Button>
                <Button variant="outline" size="sm" aria-label={`Hapus versi ${release.version_name}`} disabled={remove.isPending} onClick={() => { if (window.confirm(`Hapus rilis versi ${release.version_name}?`)) remove.mutate(release); }}><Trash2 aria-hidden="true" /></Button>
              </> : null}
            </div>
          </li>)}</ul>}
  </section>;
}
