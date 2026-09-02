import { QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter, useRoutes } from 'react-router-dom';
import { queryClient } from '../lib/queryClient';
import { dashboardRoutes } from './routes';

function Routes() {
  return useRoutes(dashboardRoutes);
}

export function DashboardApp() {
  return <QueryClientProvider client={queryClient}>
    <BrowserRouter basename="/dashboard"><Routes /></BrowserRouter>
  </QueryClientProvider>;
}
