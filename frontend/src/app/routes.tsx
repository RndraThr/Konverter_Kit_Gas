import { Navigate, RouteObject } from 'react-router-dom';
import { AppShell } from './AppShell';
import { DashboardPage } from '../features/dashboard/DashboardPage';
import { ProfilePage } from '../features/profile/ProfilePage';
import { ChangePasswordForm } from '../features/profile/ChangePasswordForm';
import { UsersPage } from '../features/users/UsersPage';
import { RolesPage } from '../features/roles/RolesPage';

function Placeholder({ title }: { title: string }) { return <section className="page"><h1>{title}</h1></section>; }

export const dashboardRoutes: RouteObject[] = [{
  path: '/',
  element: <AppShell />,
  children: [
    { index: true, element: <DashboardPage /> },
    { path: 'pengguna', element: <UsersPage /> },
    { path: 'role', element: <RolesPage /> },
    { path: 'pengaturan', element: <Placeholder title="Pengaturan sistem" /> },
    { path: 'kesehatan', element: <Placeholder title="Kesehatan sistem" /> },
    { path: 'riwayat', element: <Placeholder title="Riwayat aktivitas" /> },
    { path: 'profil', element: <ProfilePage /> },
    { path: 'profil/password', element: <ChangePasswordForm /> },
    { path: '*', element: <Navigate to="/" replace /> },
  ],
}];
