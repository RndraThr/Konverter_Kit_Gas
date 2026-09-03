import { Navigate, RouteObject } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { ReactNode } from 'react';
import { AppShell } from './AppShell';
import { getBootstrap } from '../lib/api';
import { DashboardPage } from '../features/dashboard/DashboardPage';
import { ProfilePage } from '../features/profile/ProfilePage';
import { ChangePasswordForm } from '../features/profile/ChangePasswordForm';
import { UsersPage } from '../features/users/UsersPage';
import { RolesPage } from '../features/roles/RolesPage';
import { SettingsPage } from '../features/settings/SettingsPage';
import { HealthPage } from '../features/health/HealthPage';
import { AuditPage } from '../features/audit/AuditPage';
import { ProgramSetupPage } from '../features/programs/ProgramSetupPage';
import { DCP3ImportPage } from '../features/dcp3/DCP3ImportPage';
import { DistributionPage } from '../features/distribution/DistributionPage';
import { ReportsPage } from '../features/reports/ReportsPage';

function ProtectedPage({ permission, children }: { permission: string; children: ReactNode }) {
  const bootstrap = useQuery({ queryKey: ['bootstrap'], queryFn: getBootstrap });
  if (!bootstrap.data) return null;
  const permissions = bootstrap.data.data.permissions;
  return permissions.includes('*') || permissions.includes(permission) ? children : <Navigate to="/" replace />;
}

export const dashboardRoutes: RouteObject[] = [{
  path: '/',
  element: <AppShell />,
  children: [
    { index: true, element: <DashboardPage /> },
    { path: 'persiapan-program', element: <ProtectedPage permission="programs.view"><ProgramSetupPage /></ProtectedPage> },
    { path: 'dcp3', element: <ProtectedPage permission="dcp3.view"><DCP3ImportPage /></ProtectedPage> },
    { path: 'dokumentasi/pendistribusian', element: <ProtectedPage permission="distribution.view"><DistributionPage /></ProtectedPage> },
    { path: 'laporan', element: <ProtectedPage permission="distribution.view"><ReportsPage /></ProtectedPage> },
    { path: 'pengguna', element: <ProtectedPage permission="users.view"><UsersPage /></ProtectedPage> },
    { path: 'role', element: <ProtectedPage permission="roles.view"><RolesPage /></ProtectedPage> },
    { path: 'pengaturan', element: <ProtectedPage permission="settings.view"><SettingsPage /></ProtectedPage> },
    { path: 'kesehatan', element: <ProtectedPage permission="health.view"><HealthPage /></ProtectedPage> },
    { path: 'riwayat', element: <ProtectedPage permission="audit.view"><AuditPage /></ProtectedPage> },
    { path: 'profil', element: <ProfilePage /> },
    { path: 'profil/password', element: <ChangePasswordForm /> },
    { path: '*', element: <Navigate to="/" replace /> },
  ],
}];
