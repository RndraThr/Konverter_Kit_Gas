import { DetailHeader, PointDetailBody, type DetailProps } from './PointDetailPanel';

/**
 * Phone (< 768 px): the same detail as the side panel, pinned to the bottom. It is deliberately not
 * modal, so the map above stays tappable and the officer can pick another point without closing it.
 */
export function PointBottomSheet(props: DetailProps) {
  return <section aria-label="Detail titik" data-variant="sheet" className="fixed inset-x-0 bottom-0 z-40 max-h-[65vh] overflow-hidden rounded-t-xl border-t bg-card shadow-2xl">
    <div aria-hidden="true" className="mx-auto mt-2 h-1 w-10 rounded-full bg-muted-foreground/30" />
    <DetailHeader onClose={props.onClose} />
    <div className="max-h-[calc(65vh-5rem)] overflow-y-auto p-4 pb-8"><PointDetailBody {...props} /></div>
  </section>;
}
