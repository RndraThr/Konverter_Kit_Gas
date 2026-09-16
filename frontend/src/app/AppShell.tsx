import { useQuery } from '@tanstack/react-query';
import {
  Activity, CalendarClock, CalendarRange, Camera, ChevronDown, FileClock, FileSpreadsheet, FileText, ListFilter, MapPinned, Menu,
  PanelLeftClose, PanelLeftOpen, Settings, ShieldCheck, UserRound, UsersRound,
} from 'lucide-react';
import { ReactNode, useEffect, useState } from 'react';
import { NavLink, Outlet } from 'react-router-dom';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Separator } from '@/components/ui/separator';
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from '@/components/ui/sheet';
import { Skeleton } from '@/components/ui/skeleton';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { getBootstrap, type BootstrapUser } from '../lib/api';
import { PermissionsProvider } from '../lib/permissions';

type NavItem = { label: string; to: string; permission?: string; icon: ReactNode };
type NavGroup = { label: string; items: NavItem[] };
const sidebarPreferenceKey = 'konkit.sidebar.collapsed';

const groups: NavGroup[] = [
  { label: 'Dashboard', items: [
    { label: 'Data Penerima', to: '/', permission: 'recipients.view', icon: <ListFilter /> },
    { label: 'Map Distribusi', to: '/map-distribusi', permission: 'distribution.view', icon: <MapPinned /> },
  ] },
  { label: 'Operasional', items: [
    { label: 'Persiapan program', to: '/persiapan-program', permission: 'programs.view', icon: <CalendarRange /> },
    { label: 'DCP3', to: '/dcp3', permission: 'dcp3.view', icon: <FileSpreadsheet /> },
  ] },
  { label: 'Dokumentasi', items: [
    { label: 'Pendistribusian', to: '/dokumentasi/pendistribusian', permission: 'distribution.view', icon: <Camera /> },
    { label: 'Laporan', to: '/laporan', permission: 'distribution.view', icon: <FileText /> },
  ] },
  { label: 'Administrasi', items: [
    { label: 'Pengguna', to: '/pengguna', permission: 'users.view', icon: <UsersRound /> },
    { label: 'Role & akses', to: '/role', permission: 'roles.view', icon: <ShieldCheck /> },
  ] },
  { label: 'Sistem', items: [
    { label: 'Pengaturan', to: '/pengaturan', permission: 'settings.view', icon: <Settings /> },
    { label: 'Kesehatan sistem', to: '/kesehatan', permission: 'health.view', icon: <Activity /> },
    { label: 'Riwayat aktivitas', to: '/riwayat', permission: 'audit.view', icon: <FileClock /> },
  ] },
  { label: 'Akun', items: [
    { label: 'Profil saya', to: '/profil', icon: <UserRound /> },
  ] },
];

function canSee(permissions: string[], permission?: string) {
  return !permission || permissions.includes('*') || permissions.includes(permission);
}

function Brand() {
  return <div className="flex min-h-20 items-center gap-3 px-5 py-4">
    <img className="h-8 w-20 object-contain" src="/static/images/logo-ergas.png" alt="Ergas" />
    <Separator orientation="vertical" className="h-7 bg-sidebar-foreground/20" />
    <img className="h-8 w-25 object-contain" src="/static/images/logo-ksm-cropped.png" alt="PT Kian Santang Mulitama Tbk" />
  </div>;
}

function NavigationLink({ item, collapsed, onNavigate }: { item: NavItem; collapsed: boolean; onNavigate?: () => void }) {
  const content = <>
    <span aria-hidden="true" className="shrink-0 [&_svg]:size-4.5">{item.icon}</span>
    <span className={cn(collapsed && 'sr-only')}>{item.label}</span>
  </>;
  const link = <NavLink
    aria-label={collapsed ? item.label : undefined}
    className={({ isActive }) => cn(
      'relative flex min-h-11 items-center gap-3 rounded-md px-3 text-sm font-medium text-sidebar-foreground/85 transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-sidebar-ring',
      collapsed && 'justify-center px-0',
      isActive && 'bg-sidebar-accent text-sidebar-accent-foreground before:absolute before:inset-y-2 before:left-0 before:w-1 before:rounded-r before:bg-sidebar-primary',
    )}
    end={item.to === '/'}
    onClick={onNavigate}
    to={item.to}
  >{content}</NavLink>;

  if (!collapsed) return link;
  return <Tooltip><TooltipTrigger render={link} /><TooltipContent side="right">{item.label}</TooltipContent></Tooltip>;
}

function Navigation({ permissions, collapsed = false, onNavigate }: { permissions: string[]; collapsed?: boolean; onNavigate?: () => void }) {
  return <nav className={cn('min-h-0 flex-1 overflow-y-auto py-4', collapsed ? 'px-2' : 'px-3')} aria-label="Navigasi utama">
    {groups.map((group) => {
      const items = group.items.filter((item) => canSee(permissions, item.permission));
      if (!items.length) return null;

      return <section className="mb-5" key={group.label}>
        <h2 className={cn('mb-1 px-3 text-xs font-semibold tracking-wide text-sidebar-foreground/65', collapsed && 'sr-only')}>{group.label}</h2>
        <div className="space-y-0.5">
          {items.map((item) => <NavigationLink item={item} collapsed={collapsed} onNavigate={onNavigate} key={item.to} />)}
        </div>
      </section>;
    })}
  </nav>;
}

function SystemConnectionStatus({ collapsed = false }: { collapsed?: boolean }) {
  return <Tooltip>
    <TooltipTrigger render={<div className={cn('flex min-h-16 items-center gap-3 border-t border-sidebar-border text-xs text-sidebar-foreground/75', collapsed ? 'justify-center px-0' : 'px-5')} />}>
      <span aria-hidden="true" className="size-2 rounded-full bg-primary shadow-[0_0_0_3px_rgb(79_173_66/0.15)]" />
      <span className={cn(collapsed && 'sr-only')}>Sistem terhubung</span>
    </TooltipTrigger>
    <TooltipContent>Dashboard tersambung ke sistem</TooltipContent>
  </Tooltip>;
}

function DesktopBrand({ collapsed }: { collapsed: boolean }) {
  return <div className={cn('flex items-center', collapsed ? 'min-h-24 justify-center px-2 pb-12' : 'min-h-20 gap-2 px-4 pr-7')}>
    {collapsed ? <img className="h-6 w-12 object-contain" src="/static/images/logo-ergas.png" alt="Ergas" /> : <div className="flex min-w-0 flex-1 items-center gap-2">
      <img className="h-8 w-20 object-contain" src="/static/images/logo-ergas.png" alt="Ergas" />
      <Separator orientation="vertical" className="h-6 bg-sidebar-foreground/20" />
      <img className="h-8 w-25 object-contain" src="/static/images/logo-ksm-cropped.png" alt="PT Kian Santang Mulitama Tbk" />
    </div>}
  </div>;
}

function SidebarToggle({ collapsed, onToggle }: { collapsed: boolean; onToggle: () => void }) {
  const label = collapsed ? 'Maksimalkan sidebar' : 'Minimalkan sidebar';
  return <Tooltip>
    <TooltipTrigger render={<Button
      aria-label={label}
      aria-expanded={!collapsed}
      variant="outline"
      size="icon"
      className={cn(
        'absolute right-0 z-40 touch-manipulation rounded-full border-sidebar-border bg-background text-foreground shadow-sm transition-colors hover:bg-accent hover:text-accent-foreground',
        collapsed ? 'top-12' : 'top-5',
      )}
      style={{ transform: 'translateX(50%)' }}
      onClick={onToggle}
    />}>
      {collapsed ? <PanelLeftOpen className="size-4" aria-hidden="true" /> : <PanelLeftClose className="size-4" aria-hidden="true" />}
    </TooltipTrigger>
    <TooltipContent side="right">{label}</TooltipContent>
  </Tooltip>;
}

function MobileNavigation({ permissions }: { permissions: string[] }) {
  const [open, setOpen] = useState(false);

  return <Sheet open={open} onOpenChange={setOpen}>
    <SheetTrigger
      aria-label="Buka navigasi"
      className="mr-2 md:hidden"
      render={<Button variant="ghost" size="icon" />}
    >
      <Menu aria-hidden="true" />
      <span className="sr-only">Buka navigasi</span>
    </SheetTrigger>
    <SheetContent side="left" aria-label="Navigasi utama" className="w-[min(304px,88vw)] gap-0 border-sidebar-border bg-sidebar p-0 text-sidebar-foreground sm:max-w-none" showCloseButton={false}>
      <SheetHeader className="border-b border-sidebar-border p-0">
        <SheetTitle className="sr-only">Navigasi utama</SheetTitle>
        <Brand />
      </SheetHeader>
      <Navigation permissions={permissions} onNavigate={() => setOpen(false)} />
      <SystemConnectionStatus />
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

  return <div className="flex min-w-0 flex-1 items-center">
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
  return <div className={cn('min-h-svh bg-background md:grid', collapsed ? 'md:grid-cols-[76px_minmax(0,1fr)]' : 'md:grid-cols-[264px_minmax(0,1fr)]')}>
    <aside
      aria-label="Sidebar utama"
      data-state={collapsed ? 'collapsed' : 'expanded'}
      className={cn('hidden h-svh border-r border-sidebar-border bg-sidebar md:block', collapsed ? 'p-2' : 'p-5')}
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
  const toggleSidebar = () => setSidebarCollapsed((collapsed) => {
    const next = !collapsed;
    try { window.localStorage.setItem(sidebarPreferenceKey, String(next)); } catch { /* Preference storage may be unavailable. */ }
    return next;
  });

  return <TooltipProvider>
    <div className={cn('min-h-svh bg-background md:grid md:transition-[grid-template-columns] md:duration-200 motion-reduce:transition-none', sidebarCollapsed ? 'md:grid-cols-[76px_minmax(0,1fr)]' : 'md:grid-cols-[264px_minmax(0,1fr)]')}>
      <aside aria-label="Sidebar utama" data-state={sidebarCollapsed ? 'collapsed' : 'expanded'} className="sticky top-0 z-40 hidden h-svh flex-col overflow-visible border-r border-sidebar-border bg-sidebar text-sidebar-foreground md:flex">
        <DesktopBrand collapsed={sidebarCollapsed} />
        <Separator className="bg-sidebar-border" />
        <Navigation permissions={user.permissions} collapsed={sidebarCollapsed} />
        <SystemConnectionStatus collapsed={sidebarCollapsed} />
        <SidebarToggle collapsed={sidebarCollapsed} onToggle={toggleSidebar} />
      </aside>
      <div className="min-w-0">
        <header className="sticky top-0 z-30 flex h-16 items-center gap-3 border-b bg-background/95 px-4 backdrop-blur sm:px-6">
          <MobileNavigation permissions={user.permissions} />
          <LiveClock />
          <AccountMenu user={user} csrfToken={bootstrap.data.meta.csrf_token} />
        </header>
        <main className="mx-auto w-full max-w-360 p-4 sm:p-6 lg:p-8"><PermissionsProvider permissions={user.permissions}><Outlet /></PermissionsProvider></main>
      </div>
    </div>
  </TooltipProvider>;
}
