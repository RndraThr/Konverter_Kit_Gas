import type { ReactNode } from 'react';
import { buttonVariants } from '@/components/ui/button';
import { cn } from '@/lib/utils';

type ButtonSampleProps = {
  icon?: ReactNode;
  children: ReactNode;
  variant?: 'default' | 'outline' | 'ghost' | 'secondary' | 'destructive';
  /** Ikon di kanan label (mis. ikon unduh). */
  trailingIcon?: ReactNode;
};

/**
 * Contoh tombol di panduan: tampilan sama persis dengan tombol asli (komponen
 * dan ikon yang sama) tetapi tidak bisa diklik.
 */
export function ButtonSample({ icon, children, variant = 'default', trailingIcon }: ButtonSampleProps) {
  return <span
    role="img"
    aria-label={`Contoh tombol ${typeof children === 'string' ? children : ''}`.trim()}
    className={cn(buttonVariants({ variant, size: 'sm' }), 'pointer-events-none mx-0.5 h-7 px-2 align-middle text-xs [&_svg]:size-3.5')}
  >{icon}{children}{trailingIcon}</span>;
}

/** Contoh isian/pilihan di panduan (mis. "Pilih jadwal"). */
export function FieldSample({ children }: { children: ReactNode }) {
  return <span role="img" aria-label={`Contoh isian ${typeof children === 'string' ? children : ''}`.trim()} className="pointer-events-none mx-0.5 inline-flex h-7 items-center rounded-md border bg-background px-2 align-middle text-xs text-muted-foreground">{children}</span>;
}
