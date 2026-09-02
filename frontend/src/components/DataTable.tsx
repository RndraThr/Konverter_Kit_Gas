import { ReactNode } from 'react';

export function DataTable({ children, label }: { children: ReactNode; label: string }) {
  return <div className="tableFrame"><table aria-label={label}>{children}</table></div>;
}
