export function StatusBadge({ active, activeText = 'Aktif', inactiveText = 'Nonaktif' }: { active: boolean; activeText?: string; inactiveText?: string }) {
  return <span className={`statusBadge ${active ? 'statusActive' : 'statusInactive'}`}>
    <span aria-hidden="true" />{active ? activeText : inactiveText}
  </span>;
}
