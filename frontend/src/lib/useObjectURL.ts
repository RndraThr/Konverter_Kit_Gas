import { useEffect, useState } from 'react';

/** URL blob: sementara untuk pratinjau berkas lokal; dicabut saat berkas berganti atau komponen dilepas. */
export function useObjectURL(blob: Blob | null | undefined) {
  const [url, setURL] = useState('');
  useEffect(() => {
    if (!blob) { setURL(''); return; }
    const next = URL.createObjectURL(blob);
    setURL(next);
    return () => URL.revokeObjectURL(next);
  }, [blob]);
  return url;
}
