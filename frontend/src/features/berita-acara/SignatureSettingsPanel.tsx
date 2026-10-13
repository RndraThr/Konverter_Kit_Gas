import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Eraser, ImageUp, PenLine, RotateCcw, Save, Signature, Trash2, Undo2, ZoomIn, ZoomOut } from 'lucide-react';
import { useEffect, useId, useMemo, useState } from 'react';
import { toast } from 'sonner';
import { DataState } from '@/components/DataState';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { ApiError, apiRequest } from '@/lib/api';
import { useCan } from '@/lib/permissions';
import { uppercaseBusinessText } from '@/lib/text';
import { InlineSignatureCanvas } from './InlineSignatureCanvas';
import { SignatureTableEditor, type EditorDrawing, type EditorItem, type EditorSelection } from './SignatureTableEditor';
import {
  defaultSignaturePlacement, defaultStampPlacement, imageFileToPNGBase64, samePlacement, signatoryHeadings, signatoryPath, sizeLimits,
  type MarkPlacement, type SignatoryCard, type SignatoryKind, type SignatoryRole, type SignatureLayout, type SignatureRule, type SignatureRules, type SignatureSettings,
} from './signatures';

type Props = { scheduleID: string; document: string; documentLabel: string; date: string };
type DataResponse<T> = { data: T };

type PendingImage = { image: string; preview: string };
type Draft = {
  name: string;
  nip: string;
  rule: SignatureRule;
  // keep: gambar tersimpan dipertahankan; removed: dihapus saat Simpan.
  signature: 'keep' | 'removed' | PendingImage;
  stamp: 'keep' | 'removed' | PendingImage;
};

const emptyRule: SignatureRule = { signature: false, stamp: false };
const zoomSteps = [0.75, 1, 1.25, 1.5, 2];

function initialDraft(card: SignatoryCard, rule: SignatureRule | undefined): Draft {
  return { name: card.name, nip: card.nip, rule: { ...emptyRule, ...rule }, signature: 'keep', stamp: 'keep' };
}

function sameRule(a: SignatureRule, b: SignatureRule | undefined) {
  const other = { ...emptyRule, ...b };
  return a.signature === other.signature && a.stamp === other.stamp
    && samePlacement(a.signature_placement, other.signature_placement) && samePlacement(a.stamp_placement, other.stamp_placement);
}

/**
 * Panel "Penandatangan" di setiap BA: tabel tanda tangan BA ditampilkan
 * persis seperti cetak, setiap kolom adalah area satu pihak. Tanda tangan
 * (dicoret di popup) dan stempel diletakkan langsung di tabel: digeser,
 * diubah ukurannya, dan diputar. Nama, NIP, dan pilihan cetak diatur di panel
 * samping untuk pihak yang dipilih. Semua perubahan disimpan sekaligus.
 * Nama, tanda tangan, dan stempel berlaku untuk semua BA jadwal ini; pilihan
 * cetak dan posisi global jadwal, atau khusus BA ini bila "Atur khusus" nyala.
 */
export function SignatureSettingsPanel({ scheduleID, document, documentLabel, date }: Props) {
  const canManage = useCan('bast.manage');
  const queryClient = useQueryClient();
  const switchID = useId();
  const wetSwitchID = useId();
  const settings = useQuery({
    queryKey: ['bast', 'signatures', scheduleID, document],
    queryFn: () => apiRequest<DataResponse<SignatureSettings>>(`/api/v1/bast/signatures?schedule_id=${encodeURIComponent(scheduleID)}&document=${encodeURIComponent(document)}`),
  });
  const layout = useQuery({
    queryKey: ['bast', 'signatures', 'layout', 'preview', scheduleID, document, date],
    queryFn: () => apiRequest<DataResponse<SignatureLayout>>(`/api/v1/bast/signatures/layout?schedule_id=${encodeURIComponent(scheduleID)}&document=${encodeURIComponent(document)}&date=${encodeURIComponent(date)}`),
    enabled: date !== '',
    retry: false,
  });

  const data = settings.data?.data;
  // Draf disusun ulang hanya bila data tersimpan berubah, bukan setiap refetch.
  const fingerprint = data ? JSON.stringify([data.override, data.rules, data.signatories]) : '';
  const [drafts, setDrafts] = useState<Partial<Record<SignatoryRole, Draft>>>({});
  const [selection, setSelection] = useState<EditorSelection | null>(null);
  const [aspects, setAspects] = useState<Record<string, number>>({});
  const [zoom, setZoom] = useState(1);
  const [saving, setSaving] = useState(false);
  const [drawing, setDrawing] = useState<SignatoryRole | null>(null);
  useEffect(() => {
    if (!fingerprint) return;
    const saved = JSON.parse(fingerprint) as [boolean, SignatureRules, SignatoryCard[]];
    setDrafts(Object.fromEntries(saved[2].map((card) => [card.role, initialDraft(card, saved[1][card.role])])));
    setSelection((current) => current && saved[2].some((card) => card.role === current.role) ? current : saved[2][0] ? { role: saved[2][0].role } : null);
  }, [fingerprint]);

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

  const cards = useMemo(() => data?.signatories ?? [], [data]);
  const layoutData = layout.data?.data;

  const sources = (card: SignatoryCard, draft: Draft) => ({
    signature: typeof draft.signature === 'object' ? draft.signature.preview : draft.signature === 'keep' && card.signature_updated_at ? signatoryPath(scheduleID, card.role, 'signature', card.signature_updated_at) : undefined,
    stamp: typeof draft.stamp === 'object' ? draft.stamp.preview : draft.stamp === 'keep' && card.stamp_updated_at ? signatoryPath(scheduleID, card.role, 'stamp', card.stamp_updated_at) : undefined,
  });
  const placementFor = (role: SignatoryRole, kind: SignatoryKind, rule: SignatureRule): MarkPlacement => {
    if (kind === 'stamp') return rule.stamp_placement ?? defaultStampPlacement;
    if (rule.signature_placement) return rule.signature_placement;
    const box = layoutData?.boxes.find((item) => item.role === role);
    return defaultSignaturePlacement(aspects[`${role}:signature`] ?? 3, box?.w ?? 47.5, box?.h ?? 28);
  };

  const items: Partial<Record<SignatoryRole, EditorItem>> = {};
  for (const card of cards) {
    const draft = drafts[card.role];
    if (!draft) continue;
    const src = sources(card, draft);
    items[card.role] = {
      signature: draft.rule.signature && src.signature ? { src: src.signature, placement: placementFor(card.role, 'signature', draft.rule) } : undefined,
      stamp: draft.rule.stamp && src.stamp ? { src: src.stamp, placement: placementFor(card.role, 'stamp', draft.rule) } : undefined,
    };
  }

  if (settings.isPending) return <DataState kind="loading" title="Memuat penandatangan" description="Mengambil nama, tanda tangan, dan stempel." />;
  if (settings.isError || !data) return <DataState kind="error" title="Penandatangan belum dapat dimuat" description="Coba lagi." action={{ label: 'Coba lagi', onClick: () => settings.refetch() }} />;

  const updateDraft = (role: SignatoryRole, change: Partial<Draft>) => setDrafts((current) => {
    const draft = current[role];
    return draft ? { ...current, [role]: { ...draft, ...change } } : current;
  });
  const updateRule = (role: SignatoryRole, change: Partial<SignatureRule>) => setDrafts((current) => {
    const draft = current[role];
    return draft ? { ...current, [role]: { ...draft, rule: { ...draft.rule, ...change } } } : current;
  });
  const setPlacement = (role: SignatoryRole, kind: SignatoryKind, placement: MarkPlacement | null) =>
    updateRule(role, kind === 'signature' ? { signature_placement: placement } : { stamp_placement: placement });

  const nameChanged = (card: SignatoryCard, draft: Draft) =>
    draft.name.trim() !== card.name.trim() || (card.role === 'agriculture_office' && draft.nip.trim() !== card.nip.trim());
  const dirtyRoles = cards.filter((card) => {
    const draft = drafts[card.role];
    return Boolean(draft) && (nameChanged(card, draft!) || draft!.signature !== 'keep' || draft!.stamp !== 'keep' || !sameRule(draft!.rule, data.rules[card.role]));
  });
  const reset = () => setDrafts(Object.fromEntries(cards.map((card) => [card.role, initialDraft(card, data.rules[card.role])])));
  const save = async () => {
    setSaving(true);
    try {
      let rulesDirty = false;
      for (const card of dirtyRoles) {
        const draft = drafts[card.role];
        if (!draft) continue;
        if (nameChanged(card, draft)) await apiRequest(signatoryPath(scheduleID, card.role), { method: 'PUT', body: JSON.stringify({ name: draft.name, nip: draft.nip }) });
        for (const kind of ['signature', 'stamp'] as const) {
          const value = draft[kind];
          const stored = kind === 'signature' ? card.signature_updated_at : card.stamp_updated_at;
          if (typeof value === 'object') await apiRequest(signatoryPath(scheduleID, card.role, kind), { method: 'PUT', body: JSON.stringify({ image_png: value.image }) });
          else if (value === 'removed' && stored) await apiRequest(signatoryPath(scheduleID, card.role, kind), { method: 'DELETE' });
        }
        rulesDirty ||= !sameRule(draft.rule, data.rules[card.role]);
      }
      if (rulesDirty) {
        const rules: SignatureRules = { ...data.rules };
        for (const card of cards) {
          const draft = drafts[card.role];
          if (draft) rules[card.role] = draft.rule;
        }
        await saveRules.mutateAsync({ override: data.override, rules, wet_signature: data.wet_signature });
      }
      refresh();
      toast.success('Penandatangan tersimpan.');
    } catch (error) {
      toast.error(errorMessage(error));
    } finally {
      setSaving(false);
    }
  };

  const finishDrawing = (role: SignatoryRole, image: string, placement: MarkPlacement) => {
    updateDraft(role, { signature: { image, preview: `data:image/png;base64,${image}` } });
    // Tanda tangan ditaruh tepat di tempat dicoret.
    updateRule(role, { signature: true, signature_placement: placement });
    setSelection({ role, kind: 'signature' });
    setDrawing(null);
  };
  const editorDrawing: EditorDrawing | null = drawing
    ? { role: drawing, onDone: (image, placement) => finishDrawing(drawing, image, placement), onCancel: () => setDrawing(null) }
    : null;

  // Saat diaktifkan, aturan khusus dimulai dari aturan global; saat dimatikan
  // BA kembali memakai global (aturan khusus tetap tersimpan, tidak aktif).
  const toggleOverride = () => saveRules.mutate({ override: !data.override, rules: data.global_rules, wet_signature: data.wet_signature });
  const toggleWet = () => saveRules.mutate({ override: data.override, rules: data.rules, wet_signature: !data.wet_signature });

  const selectedCard = cards.find((card) => card.role === selection?.role) ?? cards[0];
  const selectedDraft = selectedCard ? drafts[selectedCard.role] : undefined;
  const dirty = dirtyRoles.length > 0;
  const zoomIndex = zoomSteps.indexOf(zoom);

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
            ? `Pilihan cetak serta posisi tanda tangan dan stempel hanya berlaku untuk ${documentLabel}.`
            : 'Pilihan cetak serta posisi tanda tangan dan stempel adalah pengaturan global jadwal ini, berlaku untuk semua BA yang tidak diatur khusus.'} Nama, tanda tangan, dan stempel selalu dipakai bersama di semua BA kabupaten ini.`}
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

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_22rem]">
        <div className="grid min-w-0 content-start gap-3">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="flex flex-wrap gap-1.5" role="tablist" aria-label="Pihak penandatangan">
              {cards.map((card) => <Button
                key={card.role}
                type="button"
                size="sm"
                role="tab"
                aria-selected={selectedCard?.role === card.role}
                variant={selectedCard?.role === card.role ? 'default' : 'outline'}
                onClick={() => setSelection({ role: card.role })}
              >{signatoryHeadings[card.role]}</Button>)}
            </div>
            {layoutData && <div className="flex items-center gap-1">
              <Button type="button" size="icon-sm" variant="ghost" aria-label="Perkecil" disabled={zoomIndex <= 0 || drawing !== null} onClick={() => setZoom(zoomSteps[zoomIndex - 1])}><ZoomOut aria-hidden="true" /></Button>
              <span className="w-11 text-center text-xs tabular-nums">{Math.round(zoom * 100)}%</span>
              <Button type="button" size="icon-sm" variant="ghost" aria-label="Perbesar" disabled={zoomIndex >= zoomSteps.length - 1 || drawing !== null} onClick={() => setZoom(zoomSteps[zoomIndex + 1])}><ZoomIn aria-hidden="true" /></Button>
            </div>}
          </div>
          {layoutData
            ? <SignatureTableEditor
              layout={layoutData}
              items={items}
              selection={selection}
              editable={canManage}
              zoom={zoom}
              drawing={editorDrawing}
              onSelect={setSelection}
              onPlacement={(role, kind, placement) => setPlacement(role, kind, placement)}
              onAspect={(role, kind, aspect) => setAspects((current) => current[`${role}:${kind}`] === aspect ? current : { ...current, [`${role}:${kind}`]: aspect })}
            />
            : <div className="rounded-lg border border-dashed p-4 text-sm text-muted-foreground">
              {date === '' ? 'Pilih tanggal dokumen untuk menampilkan tabel tanda tangan.'
                : layout.isPending ? 'Menyiapkan tabel tanda tangan...'
                  : `Tabel tanda tangan belum dapat ditampilkan untuk tanggal ini${layout.error ? `: ${errorMessage(layout.error)}` : '.'} Nama, tanda tangan, dan stempel tetap bisa diatur di samping.`}
            </div>}
          {!layoutData && editorDrawing && <div className="relative mx-auto mb-10" style={{ width: 288, height: 181 }}>
            <InlineSignatureCanvas width={288} height={181} label={`Kanvas tanda tangan ${signatoryHeadings[editorDrawing.role]}`} onDone={editorDrawing.onDone} onCancel={editorDrawing.onCancel} />
          </div>}
          {layoutData && canManage && <p className="text-xs text-muted-foreground">{drawing ? `Coret tanda tangan langsung di kolom ${signatoryHeadings[drawing]}, lalu klik Selesai. Tanda tangan dicetak tepat di tempat dicoret.` : <>Klik tanda tangan atau stempel untuk memilih, lalu geser, tarik sudut untuk ukuran, atau tarik tombol bulat untuk memutar. Tombol panah menggeser sedikit demi sedikit. Kolom kosong dibiarkan untuk ditandatangani basah.</>}</p>}
        </div>

        {selectedCard && selectedDraft && <SignatoryEditor
          key={selectedCard.role}
          card={selectedCard}
          draft={selectedDraft}
          sources={sources(selectedCard, selectedDraft)}
          canManage={canManage}
          placement={(kind) => placementFor(selectedCard.role, kind, selectedDraft.rule)}
          onDraft={(change) => updateDraft(selectedCard.role, change)}
          onRule={(change) => updateRule(selectedCard.role, change)}
          onPlacement={(kind, placement) => setPlacement(selectedCard.role, kind, placement)}
          drawing={drawing === selectedCard.role}
          onDraw={() => { setSelection({ role: selectedCard.role }); setDrawing(selectedCard.role); }}
          onSelectMark={(kind) => setSelection({ role: selectedCard.role, kind })}
        />}
      </div>

      {canManage && <div className="flex flex-wrap items-center justify-between gap-2 border-t pt-4">
        <p className="text-xs text-muted-foreground">{dirty ? `Perubahan belum disimpan: ${dirtyRoles.map((card) => signatoryHeadings[card.role]).join(', ')}.` : 'Semua perubahan tersimpan.'}</p>
        <div className="flex gap-2">
          {dirty && <Button type="button" variant="ghost" disabled={saving} onClick={reset}><Undo2 aria-hidden="true" />Batal</Button>}
          <Button type="button" disabled={!dirty || saving} onClick={() => void save()}><Save aria-hidden="true" />{saving ? 'Menyimpan...' : 'Simpan'}</Button>
        </div>
      </div>}
    </CardContent>

  </Card>;
}

type EditorProps = {
  card: SignatoryCard;
  draft: Draft;
  sources: { signature?: string; stamp?: string };
  canManage: boolean;
  placement: (kind: SignatoryKind) => MarkPlacement;
  onDraft: (change: Partial<Draft>) => void;
  onRule: (change: Partial<SignatureRule>) => void;
  onPlacement: (kind: SignatoryKind, placement: MarkPlacement | null) => void;
  drawing: boolean;
  onDraw: () => void;
  onSelectMark: (kind: SignatoryKind) => void;
};

/** Panel samping pihak terpilih: nama, NIP, tanda tangan, stempel, dan posisinya. */
function SignatoryEditor({ card, draft, sources, canManage, placement, onDraft, onRule, onPlacement, drawing, onDraw, onSelectMark }: EditorProps) {
  const id = useId();
  const [removeWhite, setRemoveWhite] = useState(true);
  const heading = signatoryHeadings[card.role];

  const chooseStamp = async (file: File) => {
    try {
      const image = await imageFileToPNGBase64(file, removeWhite);
      onDraft({ stamp: { image, preview: `data:image/png;base64,${image}` } });
      onRule({ stamp: true });
      onSelectMark('stamp');
    } catch (error) {
      toast.error(errorMessage(error));
    }
  };

  return <section aria-label={heading} className="grid content-start gap-4 rounded-lg border p-4">
    <h3 className="text-sm font-semibold">{heading}</h3>
    <div className="grid gap-2">
      <div className="grid gap-1">
        <Label htmlFor={`${id}-name`}>Nama</Label>
        <Input id={`${id}-name`} value={draft.name} disabled={!canManage} onChange={(event) => onDraft({ name: uppercaseBusinessText(event.target.value) })} />
      </div>
      {card.role === 'agriculture_office' && <div className="grid gap-1">
        <Label htmlFor={`${id}-nip`}>NIP</Label>
        <Input id={`${id}-nip`} inputMode="numeric" value={draft.nip} disabled={!canManage} onChange={(event) => onDraft({ nip: event.target.value })} />
      </div>}
    </div>

    <div className="grid gap-2 border-t pt-3">
      <div className="flex items-center gap-2">
        <Checkbox id={`${id}-signature`} checked={draft.rule.signature} disabled={!canManage} onCheckedChange={(value) => onRule({ signature: value === true })} />
        <Label htmlFor={`${id}-signature`} className="font-normal">Cetak tanda tangan digital</Label>
      </div>
      {canManage && <div className="flex flex-wrap gap-2">
        <Button type="button" size="sm" variant="outline" disabled={drawing} onClick={onDraw}><PenLine aria-hidden="true" />{drawing ? 'Sedang mencoret...' : sources.signature ? 'Gambar ulang' : 'Gambar tanda tangan'}</Button>
        {sources.signature && <Button type="button" size="sm" variant="ghost" onClick={() => onDraft({ signature: card.signature_updated_at ? 'removed' : 'keep' })}><Eraser aria-hidden="true" />Hapus tanda tangan</Button>}
      </div>}
      {!sources.signature && <p className="text-xs text-muted-foreground">Belum ada tanda tangan digital.</p>}
      {canManage && draft.rule.signature && sources.signature && <PlacementControls kind="signature" heading={heading} placement={placement('signature')} custom={Boolean(draft.rule.signature_placement)} onChange={(value) => onPlacement('signature', value)} />}
    </div>

    <div className="grid gap-2 border-t pt-3">
      <div className="flex items-center gap-2">
        <Checkbox id={`${id}-stamp`} checked={draft.rule.stamp} disabled={!canManage} onCheckedChange={(value) => onRule({ stamp: value === true })} />
        <Label htmlFor={`${id}-stamp`} className="font-normal">Tambah stempel</Label>
      </div>
      {draft.rule.stamp && canManage && <>
        <div className="flex flex-wrap gap-2">
          <Button type="button" size="sm" variant="outline" nativeButton={false} render={<label htmlFor={`${id}-stamp-file`} />}><ImageUp aria-hidden="true" />{sources.stamp ? 'Ganti stempel' : 'Unggah stempel'}</Button>
          <input id={`${id}-stamp-file`} type="file" accept="image/png,image/jpeg" className="sr-only" onChange={(event) => { const file = event.target.files?.[0]; event.target.value = ''; if (file) void chooseStamp(file); }} />
          {sources.stamp && <Button type="button" size="sm" variant="ghost" onClick={() => onDraft({ stamp: card.stamp_updated_at ? 'removed' : 'keep' })}><Trash2 aria-hidden="true" />Hapus stempel</Button>}
        </div>
        <div className="flex items-center gap-2">
          <Checkbox id={`${id}-white`} checked={removeWhite} onCheckedChange={(value) => setRemoveWhite(value === true)} />
          <Label htmlFor={`${id}-white`} className="text-xs font-normal">Hapus latar putih hasil pindai</Label>
        </div>
        {sources.stamp && <PlacementControls kind="stamp" heading={heading} placement={placement('stamp')} custom={Boolean(draft.rule.stamp_placement)} onChange={(value) => onPlacement('stamp', value)} />}
      </>}
    </div>
  </section>;
}

type ControlsProps = { kind: SignatoryKind; heading: string; placement: MarkPlacement; custom: boolean; onChange: (placement: MarkPlacement | null) => void };

function PlacementControls({ kind, heading, placement, custom, onChange }: ControlsProps) {
  const label = kind === 'signature' ? 'tanda tangan' : 'stempel';
  const limits = sizeLimits[kind];
  return <div className="grid gap-2 rounded-md bg-muted/40 p-2">
    <div className="grid grid-cols-[3.5rem_minmax(0,1fr)_4.5rem] items-center gap-2 text-xs">
      <span>Ukuran</span>
      <input type="range" aria-label={`Ukuran ${label} ${heading}`} min={limits.min * 100} max={limits.max * 100} step={1} value={Math.round(placement.size * 100)} onChange={(event) => onChange({ ...placement, size: Number(event.target.value) / 100 })} />
      <ExactNumber label={`Ukuran ${label} ${heading} (persen)`} unit="%" value={Math.round(placement.size * 100)} min={limits.min * 100} max={limits.max * 100} onCommit={(value) => onChange({ ...placement, size: value / 100 })} />
    </div>
    <div className="grid grid-cols-[3.5rem_minmax(0,1fr)_4.5rem] items-center gap-2 text-xs">
      <span>Putar</span>
      <input type="range" aria-label={`Putar ${label} ${heading}`} min={-180} max={180} step={1} value={Math.round(placement.rotation)} onChange={(event) => onChange({ ...placement, rotation: Number(event.target.value) })} />
      <ExactNumber label={`Putar ${label} ${heading} (derajat)`} unit="°" value={Math.round(placement.rotation)} min={-180} max={180} onCommit={(value) => onChange({ ...placement, rotation: value })} />
    </div>
    <Button type="button" size="sm" variant="ghost" className="justify-self-start" disabled={!custom} onClick={() => onChange(null)}><RotateCcw aria-hidden="true" />Posisi bawaan {label}</Button>
  </div>;
}

type ExactNumberProps = { label: string; unit: string; value: number; min: number; max: number; onCommit: (value: number) => void };

/** Angka slider yang bisa diketik langsung; disimpan saat Enter atau keluar dari isian, dibatasi min/max. */
function ExactNumber({ label, unit, value, min, max, onCommit }: ExactNumberProps) {
  const [draft, setDraft] = useState<string | null>(null);
  const commit = () => {
    if (draft === null) return;
    const parsed = Number(draft.replace(',', '.'));
    if (draft.trim() !== '' && Number.isFinite(parsed)) onCommit(Math.round(Math.min(max, Math.max(min, parsed))));
    setDraft(null);
  };
  return <span className="flex items-center gap-0.5">
    <Input
      aria-label={label}
      inputMode="numeric"
      className="h-7 px-1.5 text-right text-xs tabular-nums"
      value={draft ?? String(value)}
      onFocus={(event) => { setDraft(String(value)); event.currentTarget.select(); }}
      onChange={(event) => setDraft(event.target.value)}
      onBlur={commit}
      onKeyDown={(event) => {
        if (event.key === 'Enter') { event.preventDefault(); commit(); }
        if (event.key === 'Escape') setDraft(null);
      }}
    />
    <span className="text-muted-foreground">{unit}</span>
  </span>;
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

function errorMessage(error: unknown) {
  return error instanceof ApiError || error instanceof Error ? error.message : 'Permintaan belum dapat diproses.';
}
