import { useQuery } from '@tanstack/react-query';
import { UploadQueueIndicator } from '@/components/UploadQueueIndicator';
import {
  Activity, BookOpen, BookOpenCheck, BookOpenText, Building2, CalendarClock, CalendarRange, Camera, ChevronDown, ClipboardList, FileClock, Flag, Truck, Warehouse, Wrench,
  ClipboardCheck, Gauge, ReceiptText, FileCheck2, Files, FileSpreadsheet, FileText, GraduationCap, Images, ListFilter, MapPinned, Menu, PackageOpen, PanelLeftClose, PanelLeftOpen,
  ChevronRight, PartyPopper, Presentation, Settings, ShieldCheck, Users, UsersRound, Wallet, WalletCards, X,
} from 'lucide-react';
import { ReactNode, useEffect, useId, useState } from 'react';
import { Link, NavLink, Outlet, useLocation } from 'react-router-dom';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Separator } from '@/components/ui/separator';
import { Sheet, SheetClose, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from '@/components/ui/sheet';
import { Skeleton } from '@/components/ui/skeleton';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { apiRequest, getBootstrap, type BootstrapUser } from '../lib/api';
import { PermissionsProvider } from '../lib/permissions';
import { NotificationBell } from '../features/finance/NotificationBell';
import { PageHelp } from '../features/guide/PageHelp';

type NavItem = { label: string; pageTitle?: string; to: string; permission?: string; icon: ReactNode };
// flyout: when the sidebar is minimized the group is one icon that opens its items to the side.
type NavGroup = { label: string; items: NavItem[]; icon?: ReactNode; flyout?: boolean };
type ConnectionState = 'checking' | 'connected' | 'disconnected';
const sidebarPreferenceKey = 'konkit.sidebar.collapsed';
const closedGroupsPreferenceKey = 'konkit.sidebar.closed-groups';

const groups: NavGroup[] = [
  { label: 'Ringkasan', items: [
    { label: 'Data Penerima', to: '/', permission: 'recipients.view', icon: <ListFilter /> },
    { label: 'Map Distribusi', to: '/map-distribusi', permission: 'distribution.view', icon: <MapPinned /> },
  ] },
  { label: 'Persiapan', items: [
    { label: 'Persiapan program', to: '/persiapan-program', permission: 'programs.view', icon: <CalendarRange /> },
    { label: 'DCP3', to: '/dcp3', permission: 'dcp3.view', icon: <FileSpreadsheet /> },
  ] },
  { label: 'Distribusi', items: [
    { label: 'Pendistribusian', to: '/dokumentasi/pendistribusian', permission: 'distribution.view', icon: <Camera /> },
    { label: 'Dokumen Pendukung', to: '/dokumen-pendukung', permission: 'activities.view', icon: <FileCheck2 /> },
    { label: 'Berita Acara', to: '/berita-acara', permission: 'bast.view', icon: <Files /> },
  ] },
  // In the order of the Panduan Kegiatan Pendistribusian, Kegiatan Lainnya last.
  { label: 'Dokumentasi Kegiatan', icon: <Images />, flyout: true, items: [
    { label: 'Kunjungan Kantor Dinas', to: '/dokumentasi/kunjungan-dinas', permission: 'activities.view', icon: <Building2 /> },
    { label: 'Gudang Titik Serah', to: '/dokumentasi/gudang-titik-serah', permission: 'activities.view', icon: <Warehouse /> },
    { label: 'Unloading', to: '/dokumentasi/unloading', permission: 'activities.view', icon: <PackageOpen /> },
    { label: 'Perakitan Mesin', to: '/dokumentasi/perakitan-mesin', permission: 'activities.view', icon: <Wrench /> },
    { label: 'Rakor', to: '/dokumentasi/rakor', permission: 'activities.view', icon: <Users /> },
    { label: 'Pemeriksaan Barang', to: '/dokumentasi/pemeriksaan-barang', permission: 'activities.view', icon: <ClipboardList /> },
    { label: 'Ceremony & Sosialisasi', to: '/dokumentasi/ceremony-sosialisasi', permission: 'activities.view', icon: <PartyPopper /> },
    { label: 'Training 10%', to: '/dokumentasi/training-10', permission: 'activities.view', icon: <BookOpen /> },
    { label: 'Pelatihan Teknis', to: '/dokumentasi/pelatihan-teknis', permission: 'activities.view', icon: <GraduationCap /> },
    { label: 'Kegiatan Pendistribusian', to: '/dokumentasi/kegiatan-pendistribusian', permission: 'activities.view', icon: <Truck /> },
    { label: 'Training 100%', to: '/dokumentasi/training-100', permission: 'activities.view', icon: <BookOpenCheck /> },
    { label: 'Closing Kabupaten', to: '/dokumentasi/closing-kabupaten', permission: 'activities.view', icon: <Flag /> },
    { label: 'Kegiatan Lainnya', to: '/dokumentasi/kegiatan', permission: 'activities.view', icon: <Images /> },
  ] },
  { label: 'Keuangan', items: [
    { label: 'Pengajuan', pageTitle: 'Pengajuan keuangan', to: '/keuangan', permission: 'finance.request', icon: <Wallet /> },
    { label: 'Review keuangan', to: '/keuangan-review', permission: 'finance.review', icon: <ClipboardCheck /> },
    { label: 'Voucher', pageTitle: 'Voucher keuangan', to: '/keuangan-voucher', permission: 'finance.review', icon: <ReceiptText /> },
    { label: 'Limit kabupaten', to: '/keuangan-limit', permission: 'finance.review', icon: <Gauge /> },
    { label: 'Monitoring keuangan', to: '/keuangan-monitoring', permission: 'finance.monitor', icon: <WalletCards /> },
    { label: 'Tim keuangan', to: '/keuangan-tim', permission: 'finance.manage', icon: <UsersRound /> },
  ] },
  { label: 'Laporan', items: [
    { label: 'Laporan DP3', to: '/laporan', permission: 'distribution.view', icon: <FileText /> },
    { label: 'Laporan Akhir', to: '/laporan-akhir', permission: 'final_report.view', icon: <Presentation /> },
  ] },
  { label: 'Administrasi', items: [
    { label: 'Pengguna', to: '/pengguna', permission: 'users.view', icon: <UsersRound /> },
    { label: 'Role & akses', to: '/role', permission: 'roles.view', icon: <ShieldCheck /> },
    { label: 'Pengaturan', to: '/pengaturan', permission: 'settings.view', icon: <Settings /> },
    { label: 'Kesehatan sistem', to: '/kesehatan', permission: 'health.view', icon: <Activity /> },
    { label: 'Riwayat aktivitas', to: '/riwayat', permission: 'audit.view', icon: <FileClock /> },
  ] },
];

// Panduan sits in the sidebar footer, next to the connection status.
const helpItem: NavItem = { label: 'Panduan', to: '/panduan', icon: <BookOpenText /> };
const allItems = [...groups.flatMap((group) => group.items), helpItem];

function canSee(permissions: string[], permission?: string) {
  return !permission || permissions.includes('*') || permissions.includes(permission);
}

function Brand() {
  return <div className="flex h-16 items-center gap-3 pr-16 pl-5">
    <img className="h-8 w-20 object-contain" src="/static/images/logo-ergas.png" alt="Ergas" />
    <Separator orientation="vertical" className="h-7! self-center! bg-sidebar-foreground/20" />
    <img className="h-8 w-25 object-contain" src="/static/images/logo-ksm-cropped.png" alt="PT Kian Santang Mulitama Tbk" />
  </div>;
}

function NavigationLink({ item, collapsed, onNavigate }: { item: NavItem; collapsed: boolean; onNavigate?: () => void }) {
  const accessibleLabel = item.pageTitle ?? item.label;
  const content = <>
    <span aria-hidden="true" className="shrink-0 [&_svg]:size-4.5">{item.icon}</span>
    <span
      aria-hidden={collapsed}
      className={cn(
        'min-w-0 overflow-hidden whitespace-nowrap transition-[max-width,opacity,transform] duration-200 motion-reduce:transition-none',
        collapsed ? 'max-w-0 -translate-x-1 opacity-0' : 'max-w-48 translate-x-0 opacity-100',
      )}
    >{item.label}</span>
  </>;
  const link = <NavLink
    aria-label={collapsed ? accessibleLabel : undefined}
    className={({ isActive }) => cn(
      'relative flex min-h-11 items-center gap-3 rounded-md px-3 text-sm font-medium text-sidebar-foreground/85 transition-[color,background-color,padding,gap] duration-200 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-sidebar-ring motion-reduce:transition-none',
      collapsed && 'justify-center px-0',
      isActive && 'bg-sidebar-accent text-sidebar-accent-foreground before:absolute before:inset-y-2 before:left-0 before:w-1 before:rounded-r before:bg-sidebar-primary',
    )}
    end={item.to === '/'}
    onClick={onNavigate}
    to={item.to}
  >{content}</NavLink>;

  if (!collapsed) return link;
  return <Tooltip><TooltipTrigger render={link} /><TooltipContent side="right">{accessibleLabel}</TooltipContent></Tooltip>;
}

function Navigation({ permissions, collapsed = false, onNavigate }: { permissions: string[]; collapsed?: boolean; onNavigate?: () => void }) {
  const { pathname } = useLocation();
  const navigationId = useId();
  const [closedGroups, setClosedGroups] = useState<Set<string>>(() => {
    try {
      const raw = window.localStorage.getItem(closedGroupsPreferenceKey);
      if (raw === null) return new Set(groups.map((group) => group.label));
      const stored = JSON.parse(raw);
      return new Set(Array.isArray(stored) ? stored.filter((value): value is string => typeof value === 'string') : []);
    } catch {
      return new Set(groups.map((group) => group.label));
    }
  });
  const isItemActive = (to: string) => to === '/' ? pathname === '/' : pathname === to || pathname.startsWith(`${to}/`);

  const toggleGroup = (label: string) => {
    setClosedGroups((current) => {
      const next = new Set(current);
      if (next.has(label)) next.delete(label);
      else next.add(label);
      try { window.localStorage.setItem(closedGroupsPreferenceKey, JSON.stringify([...next])); } catch { /* Preference storage may be unavailable. */ }
      return next;
    });
  };

  return <nav className={cn('min-h-0 flex-1 overflow-y-auto py-3 [scrollbar-width:thin] [scrollbar-color:var(--sidebar-border)_transparent]', collapsed ? 'px-2' : 'px-3')} aria-label="Navigasi utama">
    {groups.map((group) => {
      const items = group.items.filter((item) => canSee(permissions, item.permission));
      if (!items.length) return null;
      const hasActiveItem = items.some((item) => isItemActive(item.to));
      const isOpen = collapsed || hasActiveItem || !closedGroups.has(group.label);
      const contentId = `${navigationId}-${group.label.toLowerCase().replace(/[^a-z0-9]+/g, '-')}`;

      if (collapsed && group.flyout) {
        return <section className="mb-1 border-t border-sidebar-border/60 pt-1 first:border-t-0 first:pt-0" key={group.label}>
          <h2 className="sr-only">{group.label}</h2>
          <FlyoutGroup group={group} items={items} active={hasActiveItem} onNavigate={onNavigate} />
        </section>;
      }

      return <section className={cn(collapsed ? 'mb-1 border-t border-sidebar-border/60 pt-1 first:border-t-0 first:pt-0' : 'mb-3')} key={group.label}>
        {collapsed
          ? <h2 className="sr-only">{group.label}</h2>
          : <button
            aria-controls={contentId}
            aria-expanded={isOpen}
            className="mb-0.5 flex min-h-8 w-full items-center justify-between rounded-md px-3 text-left text-[11px] font-semibold tracking-wider text-sidebar-foreground/55 uppercase transition-colors hover:bg-sidebar-accent/70 hover:text-sidebar-accent-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-sidebar-ring"
            onClick={() => toggleGroup(group.label)}
            type="button"
          >
            <span>{group.label}</span>
            <ChevronDown aria-hidden="true" className={cn('size-3.5 transition-transform duration-200 motion-reduce:transition-none', !isOpen && '-rotate-90')} />
          </button>}
        <div className="space-y-0.5" hidden={!isOpen} id={contentId}>
          {items.map((item) => <NavigationLink key={item.to} item={item} collapsed={collapsed} onNavigate={onNavigate} />)}
        </div>
      </section>;
    })}
  </nav>;
}

/** A group shown as one icon in the minimized sidebar; its items open to the side. */
function FlyoutGroup({ group, items, active, onNavigate }: { group: NavGroup; items: NavItem[]; active: boolean; onNavigate?: () => void }) {
  const { pathname } = useLocation();
  return <DropdownMenu>
    <Tooltip>
      <TooltipTrigger render={<DropdownMenuTrigger
        aria-label={group.label}
        className={cn(
          'relative flex min-h-11 w-full items-center justify-center rounded-md text-sidebar-foreground/85 transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-sidebar-ring data-popup-open:bg-sidebar-accent',
          active && 'bg-sidebar-accent text-sidebar-accent-foreground before:absolute before:inset-y-2 before:left-0 before:w-1 before:rounded-r before:bg-sidebar-primary',
        )}
      />}>
        <span aria-hidden="true" className="[&_svg]:size-4.5">{group.icon}</span>
        <ChevronRight aria-hidden="true" className="absolute right-0.5 size-3 opacity-60" />
      </TooltipTrigger>
      <TooltipContent side="right">{group.label}</TooltipContent>
    </Tooltip>
    <DropdownMenuContent side="right" align="start" sideOffset={10} className="max-h-[min(70svh,32rem)] w-60 overflow-y-auto">
      <p className="px-2 py-1.5 text-xs font-semibold text-muted-foreground">{group.label}</p>
      {items.map((item) => <DropdownMenuItem key={item.to} render={<NavLink to={item.to} onClick={onNavigate} />} className={cn('gap-2.5 [&_svg]:size-4 [&_svg]:text-muted-foreground', (pathname === item.to || pathname.startsWith(`${item.to}/`)) && 'bg-accent font-semibold text-accent-foreground [&_svg]:text-primary')}>
        {item.icon}{item.label}
      </DropdownMenuItem>)}
    </DropdownMenuContent>
  </DropdownMenu>;
}

function SystemConnectionStatus({ collapsed = false, state }: { collapsed?: boolean; state: ConnectionState }) {
  const label = state === 'checking' ? 'Memeriksa koneksi' : state === 'disconnected' ? 'Koneksi terganggu' : 'Sistem terhubung';
  return <Tooltip>
    <TooltipTrigger render={<div aria-live="polite" role="status" className={cn('flex min-h-11 items-center gap-3 text-xs text-sidebar-foreground/70', collapsed ? 'justify-center px-0' : 'px-6')} />}>
      <span aria-hidden="true" className={cn('size-2 rounded-full', state === 'checking' ? 'bg-sidebar-primary' : state === 'disconnected' ? 'bg-destructive' : 'bg-primary shadow-[0_0_0_3px_rgb(79_173_66/0.15)]')} />
      <span className={cn(collapsed && 'sr-only')}>{label}</span>
    </TooltipTrigger>
    <TooltipContent>{state === 'disconnected' ? 'Dashboard tidak dapat menjangkau layanan' : label}</TooltipContent>
  </Tooltip>;
}

/**
 * The sidebar's top row, as tall as the page header so both borders line up.
 * Expanded: both logos and the minimize button. Minimized: only the button.
 */
function DesktopBrand({ collapsed, onToggle }: { collapsed: boolean; onToggle: () => void }) {
  return <div className={cn('flex h-16 shrink-0 items-center border-b border-sidebar-border', collapsed ? 'justify-center px-2' : 'justify-between gap-2 pr-3 pl-5')}>
    {!collapsed && <div className="flex min-w-0 items-center gap-2.5">
      <img className="h-7 w-18 object-contain" src="/static/images/logo-ergas.png" alt="Ergas" />
      <Separator orientation="vertical" className="h-6! self-center! bg-sidebar-foreground/20" />
      <img className="h-7 w-22 object-contain" src="/static/images/logo-ksm-cropped.png" alt="PT Kian Santang Mulitama Tbk" />
    </div>}
    <SidebarToggle collapsed={collapsed} onToggle={onToggle} />
  </div>;
}

function SidebarToggle({ collapsed, onToggle }: { collapsed: boolean; onToggle: () => void }) {
  const label = collapsed ? 'Maksimalkan sidebar' : 'Minimalkan sidebar';
  return <Tooltip>
    <TooltipTrigger render={<Button
      aria-label={label}
      aria-controls="primary-sidebar"
      aria-expanded={!collapsed}
      variant="ghost"
      size="icon"
      className="shrink-0 touch-manipulation text-sidebar-foreground/75 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground aria-expanded:bg-transparent aria-expanded:text-sidebar-foreground/75 aria-expanded:hover:bg-sidebar-accent"
      onClick={onToggle}
    />}>
      {collapsed ? <PanelLeftOpen className="size-4.5" aria-hidden="true" /> : <PanelLeftClose className="size-4.5" aria-hidden="true" />}
    </TooltipTrigger>
    <TooltipContent side="right">{label}</TooltipContent>
  </Tooltip>;
}

/** Bottom of the sidebar: Panduan and the connection status. */
function SidebarFooter({ collapsed = false, state, onNavigate }: { collapsed?: boolean; state: ConnectionState; onNavigate?: () => void }) {
  return <div className={cn('shrink-0 border-t border-sidebar-border py-2', collapsed ? 'px-2' : 'px-3')}>
    <NavigationLink item={helpItem} collapsed={collapsed} onNavigate={onNavigate} />
    <SystemConnectionStatus collapsed={collapsed} state={state} />
  </div>;
}

function MobileNavigation({ permissions, connectionState }: { permissions: string[]; connectionState: ConnectionState }) {
  const [open, setOpen] = useState(false);

  return <Sheet open={open} onOpenChange={setOpen}>
    <SheetTrigger
      aria-label="Buka navigasi"
      className="mr-2 lg:hidden"
      render={<Button variant="ghost" size="icon" />}
    >
      <Menu aria-hidden="true" />
      <span className="sr-only">Buka navigasi</span>
    </SheetTrigger>
    <SheetContent side="left" aria-label="Navigasi utama" className="w-[min(304px,88vw)] gap-0 border-sidebar-border bg-sidebar p-0 text-sidebar-foreground sm:max-w-none" showCloseButton={false}>
      <SheetHeader className="relative border-b border-sidebar-border p-0">
        <SheetTitle className="sr-only">Navigasi utama</SheetTitle>
        <Brand />
        <SheetClose aria-label="Tutup navigasi" render={<Button type="button" variant="ghost" size="icon" className="absolute top-4 right-3 text-sidebar-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground" />}><X aria-hidden="true" /></SheetClose>
      </SheetHeader>
      <Navigation permissions={permissions} onNavigate={() => setOpen(false)} />
      <SidebarFooter state={connectionState} onNavigate={() => setOpen(false)} />
    </SheetContent>
  </Sheet>;
}

function LiveClock() {
  const [now, setNow] = useState(() => new Date());

  useEffect(() => {
    const id = setInterval(() => setNow(new Date()), 30_000);
    return () => clearInterval(id);
  }, []);

  const fullDate = now.toLocaleDateString('id-ID', { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' });
  const shortDate = now.toLocaleDateString('id-ID', { day: 'numeric', month: 'short' });
  const time = now.toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit' });

  return <div className="hidden shrink-0 items-center xl:flex">
    <div className="flex min-w-0 items-center gap-2 rounded-full border border-border bg-muted/40 px-3.5 py-1.5">
      <CalendarClock aria-hidden="true" className="hidden size-4 shrink-0 text-muted-foreground sm:block" />
      <p className="truncate text-sm font-medium text-foreground">
        <span className="hidden sm:inline">{fullDate}</span>
        <span className="sm:hidden">{shortDate}</span>
        <span className="text-muted-foreground"> &middot; </span>
        {time}
      </p>
    </div>
  </div>;
}

function PageContext() {
  const { pathname } = useLocation();
  const matches = (item: NavItem) => item.to === '/' ? pathname === '/' : pathname === item.to || pathname.startsWith(`${item.to}/`);
  const matchedGroup = groups.find((group) => group.items.some(matches));
  const matchedItem = allItems.find(matches);
  const title = pathname.startsWith('/profil') ? 'Profil saya' : matchedItem?.pageTitle ?? matchedItem?.label ?? 'Dashboard';
  const section = pathname.startsWith('/profil') ? 'Akun' : matchedItem === helpItem ? 'Bantuan' : matchedGroup?.label ?? 'Dashboard';

  return <div aria-label="Konteks halaman" className="min-w-0 flex-1" role="group">
    <nav aria-label="Breadcrumb" className="hidden items-center gap-1 text-xs text-muted-foreground sm:flex">
      {section === 'Dashboard'
        ? <span>Dashboard</span>
        : <><Link className="hover:text-foreground hover:underline" to="/">Dashboard</Link><span aria-hidden="true">/</span><span className="truncate">{section}</span></>}
    </nav>
    <p className="truncate text-sm font-semibold text-foreground sm:text-base">{title}</p>
  </div>;
}

function AccountMenu({ user, csrfToken }: { user: BootstrapUser; csrfToken: string }) {
  const [open, setOpen] = useState(false);

  return <DropdownMenu open={open} onOpenChange={setOpen}>
    <DropdownMenuTrigger aria-label="Menu akun" onClick={() => setOpen(true)} render={<Button variant="ghost" className="h-11 max-w-64 justify-start gap-2 px-2 text-left" />}>
      <span className="grid size-9 shrink-0 place-items-center rounded-full bg-primary text-sm font-bold text-primary-foreground">
        {user.full_name.slice(0, 1).toUpperCase()}
      </span>
      <span className="hidden min-w-0 flex-1 sm:grid">
        <strong className="truncate text-sm">{user.full_name}</strong>
        <small className="truncate text-xs text-muted-foreground">{user.email}</small>
      </span>
      <ChevronDown aria-hidden="true" className="hidden size-4 text-muted-foreground sm:block" />
    </DropdownMenuTrigger>
    <DropdownMenuContent align="end" className="w-52">
      <DropdownMenuItem render={<NavLink to="/profil" />}>Profil saya</DropdownMenuItem>
      <DropdownMenuSeparator />
      <form method="post" action="/logout">
        <input type="hidden" name="csrf_token" value={csrfToken} />
        <DropdownMenuItem nativeButton variant="destructive" render={<button type="submit" />}>Keluar</DropdownMenuItem>
      </form>
    </DropdownMenuContent>
  </DropdownMenu>;
}

function AppShellSkeleton({ collapsed }: { collapsed: boolean }) {
  return <div className={cn('min-h-svh bg-background lg:grid', collapsed ? 'lg:grid-cols-[68px_minmax(0,1fr)]' : 'lg:grid-cols-[264px_minmax(0,1fr)]')}>
    <aside
      aria-label="Sidebar utama"
      data-state={collapsed ? 'collapsed' : 'expanded'}
      className={cn('hidden h-svh border-r border-sidebar-border bg-sidebar lg:block', collapsed ? 'p-2' : 'p-5')}
    >
      <Skeleton className={cn('bg-sidebar-foreground/15', collapsed ? 'mx-auto h-8 w-8' : 'h-10 w-48')} />
      <div className={cn('mt-10 space-y-3', collapsed && 'px-1')}><Skeleton className="h-11 bg-sidebar-foreground/10" /><Skeleton className="h-11 bg-sidebar-foreground/10" /><Skeleton className="h-11 bg-sidebar-foreground/10" /></div>
    </aside>
    <div className="min-w-0"><header className="flex h-16 items-center border-b bg-background px-4 sm:px-6"><Skeleton className="h-10 w-44" /></header><main className="mx-auto w-full max-w-360 p-4 sm:p-6 lg:p-8"><Skeleton className="h-72 w-full" /></main></div>
  </div>;
}

export function AppShell() {
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => {
    try { return window.localStorage.getItem(sidebarPreferenceKey) === 'true'; } catch { return false; }
  });
  const bootstrap = useQuery({ queryKey: ['bootstrap'], queryFn: getBootstrap });
  const liveness = useQuery({
    queryKey: ['liveness'],
    queryFn: async () => {
      const controller = new AbortController();
      const timeoutID = window.setTimeout(() => controller.abort(), 5_000);
      try {
        return await apiRequest<{ status: string }>('/api/v1/health', { signal: controller.signal });
      } finally {
        window.clearTimeout(timeoutID);
      }
    },
    enabled: Boolean(bootstrap.data),
    retry: false,
    refetchInterval: 60_000,
  });

  if (bootstrap.isPending) return <AppShellSkeleton collapsed={sidebarCollapsed} />;
  if (bootstrap.isError || !bootstrap.data) {
    return <div className="grid min-h-svh place-items-center bg-background p-4">
      <Alert variant="destructive" className="max-w-md">
        <AlertTitle>Dashboard tidak dapat dimuat.</AlertTitle>
        <AlertDescription>Periksa koneksi Anda, lalu coba lagi.</AlertDescription>
        <Button variant="outline" className="mt-3" onClick={() => void bootstrap.refetch()}>Coba lagi</Button>
      </Alert>
    </div>;
  }

  const { data: user } = bootstrap.data;
  const connectionState: ConnectionState = liveness.isFetching ? 'checking' : liveness.isError ? 'disconnected' : liveness.isSuccess ? 'connected' : 'checking';
  const toggleSidebar = () => setSidebarCollapsed((collapsed) => {
    const next = !collapsed;
    try { window.localStorage.setItem(sidebarPreferenceKey, String(next)); } catch { /* Preference storage may be unavailable. */ }
    return next;
  });

  return <TooltipProvider>
    <div className={cn('min-h-svh bg-background lg:grid lg:transition-[grid-template-columns] lg:duration-200 motion-reduce:transition-none', sidebarCollapsed ? 'lg:grid-cols-[68px_minmax(0,1fr)]' : 'lg:grid-cols-[264px_minmax(0,1fr)]')}>
      <aside id="primary-sidebar" aria-label="Sidebar utama" data-state={sidebarCollapsed ? 'collapsed' : 'expanded'} className="sticky top-0 z-40 hidden h-svh flex-col overflow-visible border-r border-sidebar-border bg-sidebar text-sidebar-foreground lg:flex">
        <DesktopBrand collapsed={sidebarCollapsed} onToggle={toggleSidebar} />
        <Navigation permissions={user.permissions} collapsed={sidebarCollapsed} />
        <SidebarFooter collapsed={sidebarCollapsed} state={connectionState} />
      </aside>
      <div className="min-w-0">
        <header className="sticky top-0 z-30 flex h-16 items-center gap-3 border-b bg-background/95 px-4 backdrop-blur sm:px-6">
          <MobileNavigation permissions={user.permissions} connectionState={connectionState} />
          <PageContext />
          <UploadQueueIndicator userId={user.id} />
          <NotificationBell />
          <LiveClock />
          <PageHelp permissions={user.permissions} />
          <AccountMenu user={user} csrfToken={bootstrap.data.meta.csrf_token} />
        </header>
        <main className="mx-auto w-full max-w-360 p-4 sm:p-6 lg:p-8"><PermissionsProvider permissions={user.permissions}><Outlet /></PermissionsProvider></main>
      </div>
    </div>
  </TooltipProvider>;
}
