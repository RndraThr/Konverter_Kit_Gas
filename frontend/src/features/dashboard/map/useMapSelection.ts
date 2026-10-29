import { useCallback, useState } from 'react';
import type { MapSelection, MapUrlState } from './types';

/**
 * The selected point or site. A point (`slot`) and a handover site (`hl:`) live in the URL so a link
 * reopens them; a `geo:` site has no stable id across data changes, so it stays in component state.
 */
export function useMapSelection(state: MapUrlState, update: (patch: Partial<MapUrlState>) => void) {
  const [geoSite, setGeoSite] = useState<string | null>(null);

  const selected: MapSelection | null = state.slot
    ? { kind: 'point', slotId: state.slot }
    : state.site
      ? { kind: 'site', siteId: state.site }
      : geoSite
        ? { kind: 'site', siteId: geoSite }
        : null;

  const select = useCallback((next: MapSelection | null) => {
    if (next?.kind === 'site' && !next.siteId.startsWith('hl:')) {
      setGeoSite(next.siteId);
      update({ slot: undefined, site: undefined });
      return;
    }
    setGeoSite(null);
    if (!next) update({ slot: undefined, site: undefined });
    else if (next.kind === 'point') update({ slot: next.slotId, site: undefined });
    else update({ site: next.siteId, slot: undefined });
  }, [update]);

  return { selected, select };
}
