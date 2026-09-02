import { Navigate, RouteObject } from 'react-router-dom';
import { AppShell } from './AppShell';

function Placeholder({ title }: { title: string }) {
  return <section><h1>{title}</h1></section>;
}

export const dashboardRoutes: RouteObject[] = [{
  path: '/',
  element: <AppShell />,
  children: [
    { index: true, element: <Placeholder title="Ringkasan program" /> },
    { path: 'pengguna', element: <Placeholder title="Pengguna" /> },
    { path: 'role', element: <Placeholder title="Role & akses" /> },
    { path: 'pengaturan', element: <Placeholder title="Pengaturan sistem" /> },
    { path: 'kesehatan', element: <Placeholder title="Kesehatan sistem" /> },
    { path: 'riwayat', element: <Placeholder title="Riwayat aktivitas" /> },
    { path: 'profil', element: <Placeholder title="Profil saya" /> },
    { path: 'profil/password', element: <Placeholder title="Ubah password" /> },
    { path: '*', element: <Navigate to="/" replace /> },
  ],
}];
