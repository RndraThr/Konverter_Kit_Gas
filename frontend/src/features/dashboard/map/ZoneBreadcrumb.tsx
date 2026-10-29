import { ChevronRight } from 'lucide-react';

type Props = {
  zoneName?: string;
  regencyName?: string;
  /** Clears zone and regency and shows the whole of Indonesia. */
  onIndonesia: () => void;
  /** Clears the regency and returns to the zone. */
  onZone: () => void;
};

const crumbClass = 'inline-flex min-h-11 items-center rounded-md px-1.5 text-sm text-primary underline-offset-2 hover:underline focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50';

/** `Indonesia › Zona › Kabupaten`; the last crumb is the current place. */
export function ZoneBreadcrumb({ zoneName, regencyName, onIndonesia, onZone }: Props) {
  const crumbs: Array<{ label: string; onClick?: () => void }> = [{ label: 'Indonesia', onClick: onIndonesia }];
  if (zoneName) crumbs.push({ label: zoneName, onClick: onZone });
  if (regencyName) crumbs.push({ label: regencyName });

  return <nav aria-label="Lokasi peta">
    <ol className="flex flex-wrap items-center gap-0.5">
      {crumbs.map((crumb, index) => {
        const last = index === crumbs.length - 1;
        return <li key={crumb.label} className="flex items-center gap-0.5">
          {index > 0 && <ChevronRight className="size-3.5 text-muted-foreground" aria-hidden="true" />}
          {last || !crumb.onClick
            ? <span className="px-1.5 text-sm font-medium" aria-current="location">{crumb.label}</span>
            : <button type="button" className={crumbClass} onClick={crumb.onClick}>{crumb.label}</button>}
        </li>;
      })}
    </ol>
  </nav>;
}
