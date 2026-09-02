import { Dialog } from '@base-ui/react/dialog';
import { useQuery } from '@tanstack/react-query';
import {
  Activity, ChevronDown, FileClock, Gauge, KeyRound, Menu, Settings,
  ShieldCheck, UserRound, UsersRound, X,
} from 'lucide-react';
import { ReactNode, useState } from 'react';
import { NavLink, Outlet, useLocation } from 'react-router-dom';
import { getBootstrap } from '../lib/api';
import styles from './AppShell.module.css';

type NavItem = { label: string; to: string; permission?: string; icon: ReactNode };
type NavGroup = { label: string; items: NavItem[] };

const groups: NavGroup[] = [
  { label: 'Dashboard', items: [{ label: 'Ringkasan', to: '/', permission: 'dashboard.view', icon: <Gauge /> }] },
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

function Navigation({ permissions, onNavigate }: { permissions: string[]; onNavigate?: () => void }) {
  return <nav className={styles.navigation} aria-label="Navigasi utama">
    {groups.map((group) => {
      const items = group.items.filter((item) => canSee(permissions, item.permission));
      if (!items.length) return null;
      return <section className={styles.navGroup} key={group.label}>
        <h2>{group.label}</h2>
        {items.map((item) => <NavLink
          className={({ isActive }) => `${styles.navLink} ${isActive ? styles.navLinkActive : ''}`}
          end={item.to === '/'}
          key={item.to}
          onClick={onNavigate}
          to={item.to}
        >
          <span aria-hidden="true">{item.icon}</span>{item.label}
        </NavLink>)}
      </section>;
    })}
  </nav>;
}

function Brand() {
  return <div className={styles.brand}>
    <img src="/static/images/logo-ergas.png" alt="Ergas" />
    <span aria-hidden="true" />
    <img src="/static/images/logo-ksm-cropped.png" alt="PT Kian Santang Mulitama Tbk" />
  </div>;
}

export function AppShell() {
  const [mobileOpen, setMobileOpen] = useState(false);
  const [accountOpen, setAccountOpen] = useState(false);
  const location = useLocation();
  const bootstrap = useQuery({ queryKey: ['bootstrap'], queryFn: getBootstrap });

  if (bootstrap.isPending) return <div className={styles.loading}>Menyiapkan ruang kerja...</div>;
  if (bootstrap.isError || !bootstrap.data) {
    return <div className={styles.loading} role="alert">Dashboard tidak dapat dimuat.</div>;
  }

  const { data: user } = bootstrap.data;
  const currentItem = groups.flatMap((group) => group.items).find((item) => item.to === location.pathname);

  return <div className={styles.shell}>
    <aside className={styles.sidebar}>
      <Brand />
      <Navigation permissions={user.permissions} />
      <div className={styles.sidebarFoot}>
        <span className={styles.onlineDot} aria-hidden="true" />
        Sistem terhubung
      </div>
    </aside>

    <div className={styles.workspace}>
      <header className={styles.topbar}>
        <Dialog.Root open={mobileOpen} onOpenChange={setMobileOpen}>
          <Dialog.Trigger className={styles.menuButton} aria-label="Buka navigasi">
            <Menu aria-hidden="true" />
          </Dialog.Trigger>
          <Dialog.Portal>
            <Dialog.Backdrop className={styles.backdrop} />
            <Dialog.Popup className={styles.drawer} aria-label="Navigasi utama">
              <div className={styles.drawerHead}>
                <Dialog.Title className={styles.srOnly}>Navigasi utama</Dialog.Title>
                <Brand />
                <Dialog.Close className={styles.closeButton} aria-label="Tutup navigasi"><X /></Dialog.Close>
              </div>
              <Navigation permissions={user.permissions} onNavigate={() => setMobileOpen(false)} />
            </Dialog.Popup>
          </Dialog.Portal>
        </Dialog.Root>
        <div className={styles.pageIdentity}>
          <span>Konkit gas</span>
          <strong>{currentItem?.label ?? 'Dashboard'}</strong>
        </div>
        <div className={styles.accountMenu}>
          <button className={styles.profileButton} type="button" aria-label="Menu akun" aria-expanded={accountOpen} onClick={() => setAccountOpen(!accountOpen)}>
            <span className={styles.avatar}>{user.full_name.slice(0, 1).toUpperCase()}</span>
            <span className={styles.profileText}><strong>{user.full_name}</strong><small>{user.email}</small></span>
            <ChevronDown aria-hidden="true" />
          </button>
          {accountOpen && <div className={styles.accountPopup}>
            <NavLink to="/profil" onClick={() => setAccountOpen(false)}>Profil saya</NavLink>
            <form method="post" action="/logout"><input type="hidden" name="csrf_token" value={bootstrap.data.meta.csrf_token} /><button type="submit">Keluar</button></form>
          </div>}
        </div>
      </header>
      <main className={styles.content}><Outlet /></main>
    </div>
  </div>;
}
