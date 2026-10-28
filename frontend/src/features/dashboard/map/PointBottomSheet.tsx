import { useEffect, useRef } from 'react';
import { DetailHeader, PointDetailBody, type DetailProps } from './PointDetailPanel';

/**
 * Phone (< 768 px): the same detail as the side panel, pinned to the bottom. It is deliberately NOT
 * modal (no focus trap, no backdrop), so the map above stays usable and the officer can pick another
 * point without closing it. It still behaves like a dialog for keyboards and screen readers: Escape
 * closes it, focus moves to its heading when it opens or the selection changes, and focus returns to
 * the previously focused element when it closes.
 */
export function PointBottomSheet(props: DetailProps) {
  const { onClose, selected } = props;
  const sheet = useRef<HTMLElement>(null);
  const onCloseRef = useRef(onClose);
  useEffect(() => { onCloseRef.current = onClose; });

  useEffect(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const onKey = (event: KeyboardEvent) => { if (event.key === 'Escape') onCloseRef.current(); };
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('keydown', onKey);
      if (previous && document.contains(previous)) previous.focus();
    };
  }, []);

  const selectionKey = selected.kind === 'point' ? `p:${selected.slotId}` : `s:${selected.siteId}`;
  useEffect(() => {
    const heading = sheet.current?.querySelector<HTMLElement>('h3');
    if (heading) { heading.tabIndex = -1; heading.focus(); }
  }, [selectionKey]);

  return <section ref={sheet} aria-label="Detail titik" data-variant="sheet" className="fixed inset-x-0 bottom-0 z-40 mb-0! max-h-[65vh] overflow-hidden rounded-t-xl border-t bg-card shadow-2xl">
    <div aria-hidden="true" className="mx-auto mt-2 h-1 w-10 rounded-full bg-muted-foreground/30" />
    <DetailHeader onClose={onClose} />
    <div className="max-h-[calc(65vh-5rem)] overflow-y-auto p-4 pb-8"><PointDetailBody {...props} /></div>
  </section>;
}
