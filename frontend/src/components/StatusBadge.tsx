import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';

type StatusBadgeProps = {
  active: boolean;
  activeText?: string;
  inactiveText?: string;
};

export function StatusBadge({ active, activeText = 'Aktif', inactiveText = 'Nonaktif' }: StatusBadgeProps) {
  const label = active ? activeText : inactiveText;

  return <Badge className={cn('gap-1.5', active ? 'bg-primary text-primary-foreground' : 'border-border bg-muted text-muted-foreground')} variant={active ? 'default' : 'outline'}>
    <span aria-hidden="true" className="size-1.5 rounded-full bg-current" />
    {label}
  </Badge>;
}
