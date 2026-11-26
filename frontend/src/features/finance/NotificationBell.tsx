import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { AlarmClock, Banknote, Bell, CheckCheck, ClipboardList, ImageOff, ShieldCheck } from 'lucide-react';
import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Button } from '@/components/ui/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { apiRequest } from '@/lib/api';
import { useCan } from '@/lib/permissions';
import { useRealtimeSignals } from '@/lib/realtime';
import { cn } from '@/lib/utils';
import { timeAgo, type DataResponse } from './api';

type Notification = { id: string; category: string; title: string; body: string; link: string; read_at: string | null; created_at: string };
type Inbox = { rows: Notification[]; unread: number };

const inboxKey = ['notifications'];
const categoryStyles: Record<string, { icon: typeof Bell; tone: string }> = {
  finance_review: { icon: ClipboardList, tone: 'bg-amber-100 text-amber-900 dark:bg-amber-950/40 dark:text-amber-200' },
  finance_result: { icon: ShieldCheck, tone: 'bg-emerald-100 text-emerald-900 dark:bg-emerald-950/40 dark:text-emerald-200' },
  finance_payment: { icon: Banknote, tone: 'bg-sky-100 text-sky-900 dark:bg-sky-950/40 dark:text-sky-200' },
  finance_reminder: { icon: AlarmClock, tone: 'bg-destructive/10 text-destructive' },
  distribution_revision: { icon: ImageOff, tone: 'bg-orange-100 text-orange-900 dark:bg-orange-950/40 dark:text-orange-200' },
};
/** Sinyal realtime: notifikasi baru langsung memuat ulang kotak dan data keuangan. */
function useNotificationSignals() {
  const queryClient = useQueryClient();
  useRealtimeSignals((signal) => {
    if (signal.t !== 'notification') return;
    void queryClient.invalidateQueries({ queryKey: inboxKey });
    void queryClient.invalidateQueries({ queryKey: ['finance'] });
  });
}

/**
 * A distribution revision links to the app's nomor bagi page; on the website
 * it opens the slot in the monitoring (central team) or the Pendistribusian page.
 */
export function webLink(link: string, canMonitor: boolean): string {
  const ba = /^\/ba\/([a-z0-9-]+)\/[0-9-]+\?(.*)$/.exec(link);
  if (ba) {
    const schedule = new URLSearchParams(ba[2]).get('schedule') ?? '';
    return canMonitor ? `/monitoring-distribusi?tab=ba&schedule=${schedule}&ba=${ba[1]}` : '/berita-acara';
  }
  const activity = /^\/kegiatan\/([a-z0-9_]+)\?(.*)$/.exec(link);
  if (activity) {
    const [, type, query] = activity;
    if (canMonitor) return `/monitoring-distribusi?tab=kegiatan&schedule=${new URLSearchParams(query).get('schedule') ?? ''}&activity=${type}`;
    if (type.startsWith('unloading_')) return `/dokumentasi/unloading?type=${type}`;
    return `/dokumentasi/${type === 'kegiatan_distribusi' ? 'kegiatan-pendistribusian' : type.replaceAll('_', '-')}`;
  }
  const match = /^\/nomor-bagi\/(\d+)\?(.*)$/.exec(link);
  if (!match) return link;
  const schedule = new URLSearchParams(match[2]).get('schedule') ?? '';
  return canMonitor ? `/monitoring-distribusi?tab=antrean&schedule=${schedule}&slot=${match[1]}` : '/dokumentasi/pendistribusian';
}

export function NotificationBell() {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const canMonitor = useCan('monitoring.view');
  useNotificationSignals();
  const inbox = useQuery({
    queryKey: inboxKey,
    queryFn: () => apiRequest<DataResponse<Inbox>>('/api/v1/notifications?limit=30'),
    refetchInterval: 60_000,
    retry: false,
  });
  const refresh = () => queryClient.invalidateQueries({ queryKey: inboxKey });
  const read = useMutation({ mutationFn: (id: string) => apiRequest(`/api/v1/notifications/${id}/read`, { method: 'POST' }), onSuccess: refresh });
  const readAll = useMutation({ mutationFn: () => apiRequest('/api/v1/notifications/read-all', { method: 'POST' }), onSuccess: refresh });
  const rows = inbox.data?.data.rows ?? [];
  const unread = inbox.data?.data.unread ?? 0;

  return <DropdownMenu open={open} onOpenChange={setOpen}>
    <DropdownMenuTrigger aria-label={unread > 0 ? `Notifikasi, ${unread} belum dibaca` : 'Notifikasi'} onClick={() => setOpen(true)} render={<Button variant="ghost" size="icon" className="relative" />}>
      <Bell aria-hidden="true" />
      {unread > 0 && <span className="absolute -top-0.5 -right-0.5 grid h-4.5 min-w-4.5 place-items-center rounded-full bg-destructive px-1 text-[0.625rem] font-bold text-white">{unread > 99 ? '99+' : unread}</span>}
    </DropdownMenuTrigger>
    <DropdownMenuContent align="end" className="max-h-[28rem] w-80 overflow-y-auto">
      {rows.length === 0 && <p className="p-3 text-sm text-muted-foreground">Belum ada notifikasi.</p>}
      {rows.map((row) => {
        const style = categoryStyles[row.category] ?? { icon: Bell, tone: 'bg-muted text-muted-foreground' };
        return <DropdownMenuItem key={row.id} className={cn('items-start gap-2', !row.read_at && 'bg-primary/5')} onClick={() => { if (!row.read_at) read.mutate(row.id); if (row.link) navigate(webLink(row.link, canMonitor)); }}>
          <span className={cn('grid size-8 shrink-0 place-items-center rounded-full', style.tone)}><style.icon aria-hidden="true" className="size-4" /></span>
          <span className="min-w-0 flex-1">
            <strong className={cn('block text-sm', row.read_at && 'font-medium text-muted-foreground')}>{row.title}</strong>
            <span className="block text-xs break-words text-muted-foreground">{row.body}</span>
            <span className="block text-[0.6875rem] text-muted-foreground">{timeAgo(row.created_at)}</span>
          </span>
          {!row.read_at && <span aria-label="Belum dibaca" className="mt-1.5 size-2 shrink-0 rounded-full bg-primary" />}
        </DropdownMenuItem>;
      })}
      {unread > 0 && <>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={() => readAll.mutate()}><CheckCheck aria-hidden="true" />Tandai semua dibaca</DropdownMenuItem>
      </>}
    </DropdownMenuContent>
  </DropdownMenu>;
}
