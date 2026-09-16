import { Children, createElement, Fragment, isValidElement, ReactNode, type ElementType } from 'react';
import {
  Table, TableBody, TableCaption, TableCell, TableFooter, TableHead, TableHeader, TableRow,
} from '@/components/ui/table';
import { cn } from '@/lib/utils';

type DataTableProps = {
  children: ReactNode;
  className?: string;
  label: string;
  minimumWidth?: number | string;
};

const tablePrimitives: Record<string, ElementType> = {
  caption: TableCaption,
  tbody: TableBody,
  td: TableCell,
  tfoot: TableFooter,
  th: TableHead,
  thead: TableHeader,
  tr: TableRow,
};

function withTablePrimitives(children: ReactNode): ReactNode {
  return Children.map(children, (child) => {
    if (!isValidElement(child)) return child;
    if (child.type === Fragment) {
      const nestedChildren = (child.props as { children?: ReactNode }).children;
      return createElement(Fragment, { key: child.key }, withTablePrimitives(nestedChildren));
    }
    if (typeof child.type !== 'string') return child;
    const Primitive = tablePrimitives[child.type];
    if (!Primitive) return child;

    const { children: nestedChildren, ...props } = child.props as { children?: ReactNode; [key: string]: unknown };
    return createElement(Primitive, { ...props, key: child.key }, withTablePrimitives(nestedChildren));
  });
}

export function DataTable({ children, className, label, minimumWidth = 720 }: DataTableProps) {
  return <Table
    aria-label={label}
    className={cn('min-w-full w-full', className)}
    containerClassName="rounded-lg border bg-card [scrollbar-color:color-mix(in_srgb,var(--muted-foreground)_35%,transparent)_transparent] [scrollbar-width:thin] [&::-webkit-scrollbar]:h-2 [&::-webkit-scrollbar-track]:bg-muted/30 [&::-webkit-scrollbar-thumb]:rounded-full [&::-webkit-scrollbar-thumb]:bg-muted-foreground/35 focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
    containerProps={{ 'aria-label': label, role: 'region', tabIndex: 0 }}
    style={{ minWidth: minimumWidth }}
  >
    {withTablePrimitives(children)}
  </Table>;
}
