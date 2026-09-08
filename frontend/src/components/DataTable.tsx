import { ReactNode } from 'react';
import { Table } from '@/components/ui/table';
import { cn } from '@/lib/utils';

type DataTableProps = {
  children: ReactNode;
  label: string;
  minimumWidth?: number | string;
};

export function DataTable({ children, label, minimumWidth = 720 }: DataTableProps) {
  return <Table
    aria-label={label}
    className={cn('min-w-full w-full')}
    containerClassName="rounded-lg border focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
    containerProps={{ 'aria-label': label, role: 'region', tabIndex: 0 }}
    style={{ minWidth: minimumWidth }}
  >
    {children}
  </Table>;
}
