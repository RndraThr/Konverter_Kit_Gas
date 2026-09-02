import { createRoot } from 'react-dom/client';
import { DashboardApp } from './app/DashboardApp';
import { LoginPage } from './pages/LoginPage/LoginPage';
import './styles/global.css';

const rootElement = document.getElementById('konkit-root');

if (!rootElement) {
  throw new Error('Missing #konkit-root element');
}

createRoot(rootElement).render(window.location.pathname.startsWith('/dashboard') ? <DashboardApp /> : <LoginPage />);
