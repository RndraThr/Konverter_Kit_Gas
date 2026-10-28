import { Link } from 'react-router-dom';
import { cn } from '@/lib/utils';
import { ExternalLink, MapPinned, Navigation, TriangleAlert, X } from 'lucide-react';
import { Button, buttonVariants } from '../../../components/ui/button';
import { directionsUrl, formatStageDate } from './format';
import { groupSites } from './layerData';
import { MILESTONES, REVIEW_COLOR } from './milestones';
import { SiteList } from './SiteList';
import type { MapSelection, PointFeature } from './types';
import { useSlotDetail } from './useSlotDetail';

export type DetailProps = {
  selected: MapSelection;
  /** Every loaded point (unfiltered); the selection may refer to one the current filters hide. */
  points: PointFeature[];
  regencyNames: Map<string, string>;
  onSelect: (next: MapSelection | null) => void;
  onClose: () => void;
};

const POS_ROUTE = '/dokumentasi/pendistribusian';
const stageLabel = { mesin: 'Mesin', dokumen: 'Dokumen', penyerahan: 'Penyerahan' } as const;

function ReviewNote({ point }: { point: PointFeature }) {
  const { location_status, site_outlier } = point.properties;
  const reasons = [
    location_status === 'outside' && 'Titik berada di luar batas kabupaten jadwalnya (toleransi 500 m).',
    site_outlier && 'Titik berjarak lebih dari 1 km dari titik serahnya; mungkin salah pilih titik serah atau GPS meleset.',
  ].filter(Boolean);
  if (reasons.length === 0) {
    return location_status === 'unmatched' ? <p className="text-xs text-muted-foreground">Batas kabupaten ini belum tersedia, jadi lokasi tidak divalidasi.</p> : null;
  }
  return <div role="status" className="space-y-1 rounded-md border px-3 py-2 text-sm" style={{ borderColor: REVIEW_COLOR }}>
    <strong className="flex items-center gap-1.5" style={{ color: REVIEW_COLOR }}><TriangleAlert className="size-4" aria-hidden="true" />Perlu ditinjau</strong>
    {reasons.map((reason) => <p key={String(reason)}>{reason}</p>)}
  </div>;
}

function SlotInfo({ point }: { point: PointFeature }) {
  const detail = useSlotDetail(point.properties.schedule_id, point.properties.code);
  if (detail.isPending) return <p className="text-sm text-muted-foreground">Memuat data penerima…</p>;
  if (detail.isError) return <p className="text-sm text-muted-foreground">Data penerima tidak dapat dimuat.</p>;
  const slot = detail.data;
  const photos = slot.documentation.flatMap((item) => (item.files ?? []).filter((file) => file.mime_type.startsWith('image/')).map((file) => ({ file, label: item.label, stage: item.stage })));
  const machine = [slot.machine_option_code, slot.machine_serial_number && `SN ${slot.machine_serial_number}`].filter(Boolean).join(' · ');
  return <section aria-label="Penerima dan bukti" className="space-y-3">
    <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-sm">
      <dt className="text-muted-foreground">Penerima</dt><dd className="font-medium">{slot.full_name || 'Belum ditautkan'}</dd>
      {slot.village && <><dt className="text-muted-foreground">Desa</dt><dd>{slot.village}</dd></>}
      {machine && <><dt className="text-muted-foreground">Mesin</dt><dd>{machine}</dd></>}
    </dl>
    {photos.length > 0 && <ul aria-label="Foto bukti" className="grid grid-cols-3 gap-2">
      {photos.slice(0, 6).map(({ file, label, stage }) => <li key={file.id}>
        <a href={file.content_url} target="_blank" rel="noreferrer" className="block aspect-square overflow-hidden rounded-md border bg-muted">
          <img src={file.content_url} alt={`${label} (${stageLabel[stage]})`} loading="lazy" className="size-full object-cover" />
        </a>
      </li>)}
    </ul>}
  </section>;
}

function PointView({ point, members, regencyNames, onSelect }: { point: PointFeature; members: PointFeature[]; regencyNames: Map<string, string>; onSelect: DetailProps['onSelect'] }) {
  const { code, regency_id, site_id, site_name, stage_at, stage } = point.properties;
  const [lon, lat] = point.geometry.coordinates;
  const hasSite = site_id.startsWith('hl:') || members.length > 1;
  return <div className="space-y-4">
    <div>
      <h3 className="text-base font-semibold">Nomor bagi {code}</h3>
      <p className="text-sm text-muted-foreground">{regencyNames.get(regency_id) ?? '-'}{site_name ? ` · ${site_name}` : ''}</p>
    </div>
    <ReviewNote point={point} />
    <ol aria-label="Milestone" className="space-y-1.5">
      {MILESTONES.map((milestone, index) => {
        const at = stage_at[index];
        const reached = Boolean(at) && milestone.stage <= stage;
        return <li key={milestone.stage} className="flex items-center gap-2 text-sm">
          <span aria-hidden="true" className="size-3 shrink-0 rounded-full border" style={{ backgroundColor: reached ? milestone.color : 'transparent' }} />
          <span className={reached ? 'font-medium' : 'text-muted-foreground'}>{milestone.label}</span>
          <span className="ml-auto text-xs text-muted-foreground">{reached ? formatStageDate(at) : 'Belum'}</span>
        </li>;
      })}
    </ol>
    <SlotInfo point={point} />
    <p className="text-xs text-muted-foreground tabular-nums">Koordinat {lat.toFixed(6)}, {lon.toFixed(6)}</p>
    <div className="flex flex-wrap gap-2">
      <Link to={POS_ROUTE} className={cn(buttonVariants({ size: "sm" }), "min-h-11")}><ExternalLink aria-hidden="true" />Buka nomor bagi</Link>
      <a href={directionsUrl(lon, lat)} target="_blank" rel="noreferrer" className={cn(buttonVariants({ size: "sm", variant: "outline" }), "min-h-11")}><Navigation aria-hidden="true" />Arahkan ke lokasi</a>
      {hasSite && <Button type="button" size="sm" variant="ghost" className="min-h-11" onClick={() => onSelect({ kind: 'site', siteId: site_id })}>Lihat titik serah</Button>}
    </div>
  </div>;
}

/** Shared by the desktop panel and the phone bottom sheet. */
export function PointDetailBody({ selected, points, regencyNames, onSelect, onClose }: DetailProps) {
  if (selected.kind === 'point') {
    const point = points.find((item) => item.properties.slot_id === selected.slotId);
    if (!point) {
      return <div className="space-y-3 text-sm">
        <p>Titik ini tidak ada di hasil filter saat ini.</p>
        <Button type="button" size="sm" variant="outline" className="min-h-11" onClick={onClose}>Tutup pilihan</Button>
      </div>;
    }
    const members = groupSites(points).get(point.properties.site_id) ?? [point];
    return <PointView point={point} members={members} regencyNames={regencyNames} onSelect={onSelect} />;
  }
  const members = groupSites(points).get(selected.siteId) ?? [];
  if (members.length === 0) {
    return <div className="space-y-3 text-sm">
      <p>Titik serah ini tidak ada di hasil filter saat ini.</p>
      <Button type="button" size="sm" variant="outline" className="min-h-11" onClick={onClose}>Tutup pilihan</Button>
    </div>;
  }
  const name = members[0].properties.site_name || `${members.length} nomor bagi berdekatan`;
  const regency = regencyNames.get(members[0].properties.regency_id);
  return <div className="space-y-3">
    <div>
      <h3 className="text-base font-semibold">{name}</h3>
      <p className="text-sm text-muted-foreground">{members.length} nomor bagi{regency ? ` · ${regency}` : ''}</p>
    </div>
    <SiteList members={members} onPick={(slotId) => onSelect({ kind: 'point', slotId })} />
  </div>;
}

export function DetailHeader({ onClose }: { onClose: () => void }) {
  return <div className="flex items-center gap-3 border-b px-4 py-3">
    <MapPinned className="size-5 text-primary" aria-hidden="true" />
    <h2 className="text-base leading-snug font-medium">Detail titik</h2>
    <Button type="button" size="icon" variant="ghost" className="ml-auto size-11" aria-label="Tutup" onClick={onClose}><X aria-hidden="true" /></Button>
  </div>;
}

/** Desktop: the right-hand panel next to the map. */
export function PointDetailPanel(props: DetailProps) {
  return <section aria-label="Detail titik" data-variant="panel" className="overflow-hidden rounded-lg border bg-card">
    <DetailHeader onClose={props.onClose} />
    <div className="max-h-[calc(100vh-14rem)] overflow-y-auto p-4"><PointDetailBody {...props} /></div>
  </section>;
}
