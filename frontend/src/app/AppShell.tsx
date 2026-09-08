import { useQuery } from '@tanstack/react-query';
import {
  Activity, CalendarRange, Camera, ChevronDown, FileClock, FileSpreadsheet, FileText, Gauge, KeyRound, Menu, Settings,
  ShieldCheck, UserRound, UsersRound,
} from 'lucide-react';
import { ReactNode, useState } from 'react';
import { NavLink, Outlet, useLocation } from 'react-router-dom';
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

const groups: NavGroup[] = [
  { label: 'Dashboard', items: [{ label: 'Ringkasan', to: '/', permission: 'dashboard.view', icon: <Gauge /> }] },
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
    { label: 'Ubah password', to: '/profil/password', icon: <KeyRound /> },
  ] },
];

function canSee(permissions: string[], permission?: string) {
  return !permission || permissions.includes('*') || permissions.includes(permission);
}

function Brand() {
  return <div className="flex min-h-20 items-center gap-3 px-5 py-4">
    <img className="h-9 w-[88px] object-contain" src="/static/images/logo-ergas.png" alt="Ergas" />
    <Separator orientation="vertical" className="h-7 bg-sidebar-foreground/20" />
    <img className="h-10 w-[92px] object-contain" src="/static/images/logo-ksm-cropped.png" alt="PT Kian Santang Mulitama Tbk" />
  </div>;
}

function Navigation({ permissions, onNavigate }: { permissions: string[]; onNavigate?: () => void }) {
  return <nav className="min-h-0 flex-1 overflow-y-auto px-3 py-4" aria-label="Navigasi utama">
    {groups.map((group) => {
      const items = group.items.filter((item) => canSee(permissions, item.permission));
      if (!items.length) return null;

      return <section className="mb-5" key={group.label}>
        <h2 className="mb-1 px-3 text-xs font-semibold tracking-wide text-sidebar-foreground/65">{group.label}</h2>
        <div className="space-y-0.5">
          {items.map((item) => <NavLink
            className={({ isActive }) => cn(
              'relative flex min-h-11 items-center gap-3 rounded-md px-3 text-sm font-medium text-sidebar-foreground/85 transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-sidebar-ring',
              isActive && 'bg-sidebar-accent text-sidebar-accent-foreground before:absolute before:inset-y-2 before:left-0 before:w-1 before:rounded-r before:bg-sidebar-primary',
            )}
            end={item.to === '/'}
            key={item.to}
            onClick={onNavigate}
            to={item.to}
          >
            <span aria-hidden="true" className="shrink-0 [&_svg]:size-[18px]">{item.icon}</span>
            {item.label}
          </NavLink>)}
        </div>
      </section>;
    })}
  </nav>;
}

function SystemConnectionStatus() {
  return <Tooltip>
    <TooltipTrigger render={<div className="flex min-h-16 items-center gap-3 border-t border-sidebar-border px-5 text-xs text-sidebar-foreground/75" />}>
      <span aria-hidden="true" className="size-2 rounded-full bg-primary shadow-[0_0_0_3px_rgb(79_173_66_/_0.15)]" />
      Sistem terhubung
    </TooltipTrigger>
    <TooltipContent>Dashboard tersambung ke sistem</TooltipContent>
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

function PageIdentity({ currentItem }: { currentItem?: NavItem }) {
  return <div className="min-w-0 flex-1">
    <p className="text-xs font-semibold text-muted-foreground">Konkit gas</p>
    <p className="truncate text-base font-semibold text-foreground">{currentItem?.label ?? 'Dashboard'}</p>
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
        <DropdownMenuItem variant="destructive" render={<button type="submit" />}>Keluar</DropdownMenuItem>
      </form>
    </DropdownMenuContent>
  </DropdownMenu>;
}

function AppShellSkeleton() {
  return <div className="min-h-svh bg-background md:grid md:grid-cols-[264px_minmax(0,1fr)]">
    <aside className="hidden h-svh border-r border-sidebar-border bg-sidebar p-5 md:block">
      <Skeleton className="h-10 w-48 bg-sidebar-foreground/15" />
      <div className="mt-10 space-y-3"><Skeleton className="h-11 bg-sidebar-foreground/10" /><Skeleton className="h-11 bg-sidebar-foreground/10" /><Skeleton className="h-11 bg-sidebar-foreground/10" /></div>
    </aside>
    <div className="min-w-0"><header className="flex h-16 items-center border-b bg-background px-4 sm:px-6"><Skeleton className="h-10 w-44" /></header><main className="mx-auto w-full max-w-[1440px] p-4 sm:p-6 lg:p-8"><Skeleton className="h-72 w-full" /></main></div>
  </div>;
}

export function AppShell() {
  const location = useLocation();
  const bootstrap = useQuery({ queryKey: ['bootstrap'], queryFn: getBootstrap });

  if (bootstrap.isPending) return <AppShellSkeleton />;
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
  const currentItem = groups.flatMap((group) => group.items).find((item) => item.to === location.pathname);

  return <TooltipProvider>
    <div className="min-h-svh bg-background md:grid md:grid-cols-[264px_minmax(0,1fr)]">
      <aside className="sticky top-0 hidden h-svh flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground md:flex">
        <Brand />
        <Separator className="bg-sidebar-border" />
        <Navigation permissions={user.permissions} />
        <SystemConnectionStatus />
      </aside>
      <div className="min-w-0">
        <header className="sticky top-0 z-30 flex h-16 items-center border-b bg-background/95 px-4 backdrop-blur sm:px-6">
          <MobileNavigation permissions={user.permissions} />
          <PageIdentity currentItem={currentItem} />
          <AccountMenu user={user} csrfToken={bootstrap.data.meta.csrf_token} />
        </header>
        <main className="mx-auto w-full max-w-[1440px] p-4 sm:p-6 lg:p-8"><PermissionsProvider permissions={user.permissions}><Outlet /></PermissionsProvider></main>
      </div>
    </div>
  </TooltipProvider>;
}
