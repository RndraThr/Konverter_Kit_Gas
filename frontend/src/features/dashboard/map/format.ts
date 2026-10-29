const dateFormat = new Intl.DateTimeFormat('id-ID', { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'Asia/Jakarta' });

/** Milestone date as the field team reads it: "3 November 2026", always in WIB. */
export const formatStageDate = (iso: string) => dateFormat.format(new Date(iso));

/** Opens turn-by-turn directions to a point in Google Maps (phones and desktop). */
export const directionsUrl = (lon: number, lat: number) => `https://www.google.com/maps/dir/?api=1&destination=${lat},${lon}`;
