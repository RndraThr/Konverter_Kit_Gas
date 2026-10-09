export function formatSettingsPreview(isoValue: string, timezone: string, dateFormat: string, locale: string) {
  const date = new Date(isoValue);
  if (Number.isNaN(date.getTime())) return 'Waktu belum tersedia';

  try {
    const parts = new Intl.DateTimeFormat(locale, {
      timeZone: timezone,
      day: '2-digit',
      month: '2-digit',
      year: 'numeric',
    }).formatToParts(date);
    const value = (type: Intl.DateTimeFormatPartTypes) => parts.find((part) => part.type === type)?.value ?? '';
    const dateText = dateFormat === '02 January 2006'
      ? new Intl.DateTimeFormat(locale, { timeZone: timezone, day: '2-digit', month: 'long', year: 'numeric' }).format(date)
      : `${value('day')}/${value('month')}/${value('year')}`;
    const timeText = new Intl.DateTimeFormat(locale, {
      timeZone: timezone,
      hour: '2-digit',
      minute: '2-digit',
      hour12: false,
    }).format(date);
    return `${dateText} · ${timeText}`;
  } catch {
    return 'Format regional belum valid';
  }
}
