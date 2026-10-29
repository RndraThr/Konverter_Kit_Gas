import { useEffect, useState } from 'react';

export const PHONE_QUERY = '(max-width: 767px)';

const supported = () => typeof window !== 'undefined' && typeof window.matchMedia === 'function';

/** True below the 768 px breakpoint (Tailwind `md`), where the detail panel becomes a bottom sheet. */
export function useIsPhone(): boolean {
  const [phone, setPhone] = useState(() => (supported() ? window.matchMedia(PHONE_QUERY).matches : false));
  useEffect(() => {
    if (!supported()) return;
    const query = window.matchMedia(PHONE_QUERY);
    const onChange = (event: MediaQueryListEvent) => setPhone(event.matches);
    setPhone(query.matches);
    query.addEventListener('change', onChange);
    return () => query.removeEventListener('change', onChange);
  }, []);
  return phone;
}
