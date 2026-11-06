import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { AlarmClock, Banknote, Bell, CheckCheck, ClipboardList, ShieldCheck } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Button } from '@/components/ui/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { apiRequest } from '@/lib/api';
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
};
const reconnectDelays = [1_000, 3_000, 10_000, 30_000];

/** Sinyal realtime: notifikasi baru langsung memuat ulang kotak dan data keuangan. */
function useNotificationSignals() {
  const queryClient = useQueryClient();
  useEffect(() => {
    if (typeof WebSocket === 'undefined') return;
    let socket: WebSocket | null = null;
    let attempts = 0;
    let timer = 0;
    let stopped = false;
    const connect = () => {
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      socket = new WebSocket(`${protocol}//${window.location.host}/api/v1/realtime`);
      socket.onopen = () => { attempts = 0; };
      socket.onmessage = (event: MessageEvent<string>) => {
        const signal = JSON.parse(event.data) as { t?: string };
        if (signal.t !== 'notification') return;
        void queryClient.invalidateQueries({ queryKey: inboxKey });
        void queryClient.invalidateQueries({ queryKey: ['finance'] });
      };
      socket.onclose = () => {
        if (stopped) return;
        timer = window.setTimeout(connect, reconnectDelays[Math.min(attempts, reconnectDelays.length - 1)]);
        attempts += 1;
      };
    };
    connect();
    return () => { stopped = true; window.clearTimeout(timer); socket?.close(); };
  }, [queryClient]);
}

export function NotificationBell() {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
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
        return <DropdownMenuItem key={row.id} className={cn('items-start gap-2', !row.read_at && 'bg-primary/5')} onClick={() => { if (!row.read_at) read.mutate(row.id); if (row.link) navigate(row.link); }}>
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
