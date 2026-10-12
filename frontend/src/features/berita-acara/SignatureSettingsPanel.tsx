import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Eraser, ImageUp, Save, Signature, Trash2, Undo2 } from 'lucide-react';
import { useEffect, useId, useState } from 'react';
import { toast } from 'sonner';
import { DataState } from '@/components/DataState';
import { useSignatureCanvas } from '@/components/SignaturePad';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { ApiError, apiRequest } from '@/lib/api';
import { useCan } from '@/lib/permissions';
import { uppercaseBusinessText } from '@/lib/text';
import {
  imageFileToPNGBase64, signatoryHeadings, signatoryPath,
  type SignatoryCard as Card_, type SignatureRule, type SignatureRules, type SignatureSettings,
} from './signatures';

type Props = { scheduleID: string; document: string; documentLabel: string };
type DataResponse<T> = { data: T };

/**
 * Panel "Penandatangan" yang sama di setiap BA: satu kartu per pihak yang
 * menyerupai kolom tanda tangan BA (kotak tanda tangan yang langsung bisa
 * dicoret, Nama, NIP), dengan opsi cetak tanda tangan digital dan stempel.
 * Nama, tanda tangan, dan stempel berlaku untuk semua BA jadwal ini. Switch
 * "Atur khusus" mati: pilihan cetak disimpan sebagai pengaturan global jadwal;
 * nyala: hanya untuk BA ini.
 */
export function SignatureSettingsPanel({ scheduleID, document, documentLabel }: Props) {
  const canManage = useCan('bast.manage');
  const queryClient = useQueryClient();
  const switchID = useId();
  const wetSwitchID = useId();
  const settings = useQuery({
    queryKey: ['bast', 'signatures', scheduleID, document],
    queryFn: () => apiRequest<DataResponse<SignatureSettings>>(`/api/v1/bast/signatures?schedule_id=${encodeURIComponent(scheduleID)}&document=${encodeURIComponent(document)}`),
  });

  // Preview BA mana pun bisa terpengaruh (nama, gambar, dan aturan global).
  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ['bast', 'signatures', scheduleID] });
    queryClient.invalidateQueries({ queryKey: ['bast', 'schedule-settings', scheduleID] });
    queryClient.invalidateQueries({ predicate: (query) => ['preview', 'summary', 'dates'].some((key) => query.queryKey.includes(key)) });
  };
  const saveRules = useMutation({
    mutationFn: (input: { override: boolean; rules: SignatureRules; wet_signature: boolean }) => apiRequest<DataResponse<SignatureSettings>>('/api/v1/bast/signatures', { method: 'PUT', body: JSON.stringify({ schedule_id: scheduleID, document, ...input }) }),
    onSuccess: refresh,
    onError: (error) => toast.error(errorMessage(error)),
  });

  if (settings.isPending) return <DataState kind="loading" title="Memuat penandatangan" description="Mengambil nama, tanda tangan, dan stempel." />;
  const data = settings.data?.data;
  if (settings.isError || !data) return <DataState kind="error" title="Penandatangan belum dapat dimuat" description="Coba lagi." action={{ label: 'Coba lagi', onClick: () => settings.refetch() }} />;

  // Saat diaktifkan, aturan khusus dimulai dari aturan global; saat dimatikan
  // BA kembali memakai global (aturan khusus tetap tersimpan, tidak aktif).
  const toggleOverride = () => saveRules.mutate({ override: !data.override, rules: data.global_rules, wet_signature: data.wet_signature });
  const toggleWet = () => saveRules.mutate({ override: data.override, rules: data.rules, wet_signature: !data.wet_signature });

  return <Card>
    <CardHeader className="border-b"><CardTitle className="flex items-center gap-2 text-base"><Signature className="size-4 text-primary" aria-hidden="true" />Penandatangan</CardTitle></CardHeader>
    <CardContent className="grid gap-4 pt-5">
      <div className={`grid gap-3 ${data.wet_supported ? 'lg:grid-cols-2' : ''}`}>
        <SwitchRow
          id={switchID}
          checked={data.override}
          disabled={!canManage || saveRules.isPending}
          onToggle={toggleOverride}
          label={`Atur khusus untuk ${documentLabel}`}
          description={`${data.override
            ? `Pilihan cetak tanda tangan dan stempel hanya berlaku untuk ${documentLabel}.`
            : 'Pilihan cetak tanda tangan dan stempel adalah pengaturan global jadwal ini, berlaku untuk semua BA yang tidak diatur khusus.'} Nama, tanda tangan, dan stempel selalu dipakai bersama di semua BA kabupaten ini.`}
        />
        {data.wet_supported && <SwitchRow
          id={wetSwitchID}
          checked={data.wet_signature}
          disabled={!canManage || saveRules.isPending}
          onToggle={toggleWet}
          label="Tanda tangan basah"
          description={`Halaman tanda tangan dicetak, ditandatangani dan distempel basah, lalu hasil pindainya diunggah dan menggantikan halaman tanda tangan di PDF. ${data.override ? `Hanya untuk ${documentLabel}.` : 'Berlaku untuk semua BA jadwal ini yang tidak diatur khusus.'}`}
        />}
      </div>

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {data.signatories.map((card) => <SignatoryCardView
          key={card.role}
          scheduleID={scheduleID}
          card={card}
          rule={data.rules[card.role] ?? { signature: false, stamp: false }}
          canManage={canManage}
          saveRule={(rule) => saveRules.mutateAsync({ override: data.override, rules: { ...data.rules, [card.role]: rule }, wet_signature: data.wet_signature })}
          onSaved={refresh}
        />)}
      </div>
      <p className="text-xs text-muted-foreground">Kotak tanpa tanda tangan, atau yang tidak dicentang, dibiarkan kosong untuk ditandatangani basah.</p>
    </CardContent>
  </Card>;
}

type SwitchRowProps = { id: string; checked: boolean; disabled: boolean; onToggle: () => void; label: string; description: string };

function SwitchRow({ id, checked, disabled, onToggle, label, description }: SwitchRowProps) {
  return <div className="flex items-start gap-3 rounded-lg border p-3">
    <button
      id={id}
      type="button"
      role="switch"
      aria-checked={checked}
      disabled={disabled}
      onClick={onToggle}
      className="relative mt-0.5 inline-flex h-5 w-9 shrink-0 items-center rounded-full border transition-colors disabled:opacity-50 aria-checked:bg-primary aria-[checked=false]:bg-muted"
    >
      <span className={`inline-block size-4 rounded-full bg-background shadow transition-transform ${checked ? 'translate-x-4' : 'translate-x-0.5'}`} />
    </button>
    <div className="grid gap-0.5">
      <Label htmlFor={id}>{label}</Label>
      <p className="text-xs text-muted-foreground">{description}</p>
    </div>
  </div>;
}

type CardProps = {
  scheduleID: string;
  card: Card_;
  rule: SignatureRule;
  canManage: boolean;
  saveRule: (rule: SignatureRule) => Promise<unknown>;
  onSaved: () => void;
};

type PendingStamp = { image: string; preview: string } | 'remove' | null;

const canvasHeight = 150;

/**
 * Satu kartu penandatangan. Kotak tanda tangan adalah kanvas: tanda tangan
 * tersimpan tampil di dalamnya dan langsung bisa dicoret ulang. Semua
 * perubahan (nama, NIP, tanda tangan, pilihan cetak, stempel) baru disimpan
 * saat tombol Simpan di bawah kartu ditekan.
 */
function SignatoryCardView({ scheduleID, card, rule, canManage, saveRule, onSaved }: CardProps) {
  const id = useId();
  const [name, setName] = useState(card.name);
  const [nip, setNIP] = useState(card.nip);
  const [draftRule, setDraftRule] = useState(rule);
  // keep: tanda tangan tersimpan dipertahankan; drawn: diganti coretan baru;
  // cleared: dihapus.
  const [signatureState, setSignatureState] = useState<'keep' | 'drawn' | 'cleared'>('keep');
  const [pendingStamp, setPendingStamp] = useState<PendingStamp>(null);
  const [removeWhite, setRemoveWhite] = useState(true);
  const [saving, setSaving] = useState(false);
  const pad = useSignatureCanvas(canvasHeight, () => setSignatureState('drawn'));
  const { clear } = pad;

  const reset = () => {
    setName(card.name);
    setNIP(card.nip);
    setDraftRule(rule);
    setSignatureState('keep');
    setPendingStamp(null);
    clear();
  };
  // Data tersimpan berubah (mis. setelah Simpan): kartu kembali ke keadaan bersih.
  useEffect(() => {
    setName(card.name);
    setNIP(card.nip);
    setDraftRule({ signature: rule.signature, stamp: rule.stamp });
    setSignatureState('keep');
    setPendingStamp(null);
    clear();
  }, [card.name, card.nip, card.signature_updated_at, card.stamp_updated_at, rule.signature, rule.stamp, clear]);

  const hasNIP = card.role === 'agriculture_office';
  const heading = signatoryHeadings[card.role];
  const signatureURL = card.signature_updated_at ? signatoryPath(scheduleID, card.role, 'signature', card.signature_updated_at) : undefined;
  const storedStampURL = card.stamp_updated_at ? signatoryPath(scheduleID, card.role, 'stamp', card.stamp_updated_at) : undefined;
  const stampURL = pendingStamp === 'remove' ? undefined : pendingStamp ? pendingStamp.preview : storedStampURL;
  const showStoredSignature = signatureState === 'keep' && Boolean(signatureURL);
  const nameDirty = name.trim() !== card.name.trim() || (hasNIP && nip.trim() !== card.nip.trim());
  const ruleDirty = draftRule.signature !== rule.signature || draftRule.stamp !== rule.stamp;
  const dirty = nameDirty || ruleDirty || signatureState !== 'keep' || pendingStamp !== null;

  const save = async () => {
    setSaving(true);
    try {
      if (nameDirty) await apiRequest(signatoryPath(scheduleID, card.role), { method: 'PUT', body: JSON.stringify({ name, nip }) });
      if (signatureState === 'drawn' && !pad.empty) await apiRequest(signatoryPath(scheduleID, card.role, 'signature'), { method: 'PUT', body: JSON.stringify({ image_png: pad.exportPNG() }) });
      if (signatureState === 'cleared' && signatureURL) await apiRequest(signatoryPath(scheduleID, card.role, 'signature'), { method: 'DELETE' });
      if (pendingStamp === 'remove' && storedStampURL) await apiRequest(signatoryPath(scheduleID, card.role, 'stamp'), { method: 'DELETE' });
      else if (pendingStamp && pendingStamp !== 'remove') await apiRequest(signatoryPath(scheduleID, card.role, 'stamp'), { method: 'PUT', body: JSON.stringify({ image_png: pendingStamp.image }) });
      if (ruleDirty) await saveRule(draftRule);
      onSaved();
      toast.success(`${heading} tersimpan.`);
    } catch (error) {
      toast.error(errorMessage(error));
    } finally {
      setSaving(false);
    }
  };

  const chooseStamp = async (file: File) => {
    try {
      const image = await imageFileToPNGBase64(file, removeWhite);
      setPendingStamp({ image, preview: `data:image/png;base64,${image}` });
    } catch (error) {
      toast.error(errorMessage(error));
    }
  };
  const clearSignature = () => {
    clear();
    setSignatureState(signatureURL ? 'cleared' : 'keep');
  };

  return <section aria-label={heading} className="flex flex-col overflow-hidden rounded-lg border bg-background">
    <div className="flex min-h-12 items-center justify-center border-b px-3 py-2 text-center text-xs font-medium uppercase tracking-wide">{heading}</div>

    <div className="relative overflow-hidden border-b bg-white" style={{ height: canvasHeight }}>
      {showStoredSignature && <img alt={`Tanda tangan ${heading}`} src={signatureURL} className="pointer-events-none absolute top-1/2 left-1/2 max-h-[85%] max-w-[86%] -translate-x-1/2 -translate-y-1/2 object-contain" />}
      {!showStoredSignature && pad.empty && <span className="pointer-events-none absolute inset-0 grid place-items-center px-3 text-center text-xs text-muted-foreground">{canManage ? 'Coret di sini untuk tanda tangan' : 'Belum ada tanda tangan'}</span>}
      <canvas
        {...pad.canvasProps}
        aria-label={`Area tanda tangan ${heading}`}
        className={`absolute inset-0 block h-full w-full touch-none ${canManage ? 'cursor-crosshair' : 'pointer-events-none'}`}
      />
      {draftRule.stamp && stampURL && <img alt={`Stempel ${heading}`} src={stampURL} className="pointer-events-none absolute top-1/2 left-[8%] h-[78%] -translate-y-1/2 rotate-[-8deg] object-contain opacity-90 mix-blend-multiply" />}
    </div>

    <div className="grid gap-2 border-b px-3 py-2">
      <div className="flex items-center gap-2 text-sm">
        <Label htmlFor={`${id}-name`} className="w-12 shrink-0 whitespace-nowrap font-normal">Nama :</Label>
        <Input id={`${id}-name`} className="h-8" value={name} disabled={!canManage} onChange={(event) => setName(uppercaseBusinessText(event.target.value))} />
      </div>
      {hasNIP && <div className="flex items-center gap-2 text-sm">
        <Label htmlFor={`${id}-nip`} className="w-12 shrink-0 whitespace-nowrap font-normal">NIP :</Label>
        <Input id={`${id}-nip`} className="h-8" inputMode="numeric" value={nip} disabled={!canManage} onChange={(event) => setNIP(event.target.value)} />
      </div>}
    </div>

    <div className="grid gap-2 p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <Checkbox id={`${id}-signature`} checked={draftRule.signature} disabled={!canManage} onCheckedChange={(value) => setDraftRule({ ...draftRule, signature: value === true })} />
          <Label htmlFor={`${id}-signature`} className="text-sm font-normal">Cetak tanda tangan digital</Label>
        </div>
        {canManage && (showStoredSignature || !pad.empty) && <Button type="button" size="sm" variant="ghost" onClick={clearSignature}><Eraser aria-hidden="true" />Ulangi</Button>}
      </div>
      <div className="flex items-center gap-2">
        <Checkbox id={`${id}-stamp`} checked={draftRule.stamp} disabled={!canManage} onCheckedChange={(value) => setDraftRule({ ...draftRule, stamp: value === true })} />
        <Label htmlFor={`${id}-stamp`} className="text-sm font-normal">Tambah stempel</Label>
      </div>
      {draftRule.stamp && canManage && <div className="grid gap-2 rounded-md bg-muted/40 p-2">
        <div className="flex flex-wrap items-center gap-2">
          <Button type="button" size="sm" variant="outline" nativeButton={false} render={<label htmlFor={`${id}-stamp-file`} />}><ImageUp aria-hidden="true" />{stampURL ? 'Ganti stempel' : 'Unggah stempel'}</Button>
          <input id={`${id}-stamp-file`} type="file" accept="image/png,image/jpeg" className="sr-only" onChange={(event) => { const file = event.target.files?.[0]; event.target.value = ''; if (file) void chooseStamp(file); }} />
          {stampURL && <Button type="button" size="sm" variant="ghost" onClick={() => setPendingStamp(storedStampURL ? 'remove' : null)}><Trash2 aria-hidden="true" />Hapus</Button>}
        </div>
        <div className="flex items-center gap-2">
          <Checkbox id={`${id}-white`} checked={removeWhite} onCheckedChange={(value) => setRemoveWhite(value === true)} />
          <Label htmlFor={`${id}-white`} className="text-xs font-normal">Hapus latar putih hasil pindai</Label>
        </div>
        {!stampURL && <p className="text-xs text-muted-foreground">Unggah PNG/JPG; posisinya otomatis di sisi kiri tanda tangan.</p>}
      </div>}
    </div>

    {canManage && <div className="mt-auto flex gap-2 border-t bg-muted/20 p-3">
      <Button type="button" className="flex-1" disabled={!dirty || saving} onClick={() => void save()}><Save aria-hidden="true" />{saving ? 'Menyimpan...' : 'Simpan'}</Button>
      {dirty && <Button type="button" variant="ghost" disabled={saving} onClick={reset}><Undo2 aria-hidden="true" />Batal</Button>}
    </div>}
  </section>;
}

function errorMessage(error: unknown) {
  return error instanceof ApiError || error instanceof Error ? error.message : 'Permintaan belum dapat diproses.';
}
