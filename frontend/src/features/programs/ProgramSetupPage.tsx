import { useQuery } from '@tanstack/react-query';
import { Building2, CalendarDays, ClipboardList, Layers3, type LucideIcon } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { PageHeader } from '../../components/PageHeader';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '../../components/ui/tabs';
import { apiRequest } from '../../lib/api';
import { RegenciesPanel } from './RegenciesPanel';
import { ProgramsPanel } from './ProgramsPanel';
import { SchedulesPanel } from './SchedulesPanel';
import { TemplatesPanel } from './TemplatesPanel';
import styles from './ProgramSetup.module.css';
import { DataResponse, DocumentationTemplate, PackageTemplate, Program, Regency, Schedule } from './types';

const workspaceValues = ['regencies', 'programs', 'schedules', 'templates'] as const;
type Workspace = typeof workspaceValues[number];

function TabLabel({ children, count, pending, icon: Icon }: { children: string; count?: number; pending: boolean; icon: LucideIcon }) {
  return <span className={styles.tabLabel}><Icon aria-hidden="true" /><span>{children}</span><span className={styles.tabCount} aria-hidden="true">{pending ? '\u2026' : count ?? 0}</span></span>;
}

export function ProgramSetupPage() {
  const [params, setParams] = useSearchParams();
  const requestedTab = params.get('tab');
  const urlTab: Workspace = workspaceValues.includes(requestedTab as Workspace) ? requestedTab as Workspace : 'regencies';
  const [activeTab, setActiveTab] = useState<Workspace>(urlTab);
  useEffect(() => setActiveTab(urlTab), [urlTab]);
  const regencies = useQuery({ queryKey: ['program-setup', 'regencies'], queryFn: () => apiRequest<DataResponse<Regency[]>>('/api/v1/program-setup/regencies') });
  const programs = useQuery({ queryKey: ['program-setup', 'programs'], queryFn: () => apiRequest<DataResponse<Program[]>>('/api/v1/program-setup/programs') });
  const schedules = useQuery({ queryKey: ['program-setup', 'schedules'], queryFn: () => apiRequest<DataResponse<Schedule[]>>('/api/v1/program-setup/schedules') });
  const packages = useQuery({ queryKey: ['program-setup', 'package-templates'], queryFn: () => apiRequest<DataResponse<PackageTemplate[]>>('/api/v1/program-setup/package-templates') });
  const documents = useQuery({ queryKey: ['program-setup', 'documentation-templates'], queryFn: () => apiRequest<DataResponse<DocumentationTemplate[]>>('/api/v1/program-setup/documentation-templates') });
  const selectWorkspace = (value: string | number) => {
    const next = workspaceValues.includes(value as Workspace) ? value as Workspace : 'regencies';
    setActiveTab(next);
    const nextParams = new URLSearchParams(params);
    nextParams.set('tab', next);
    setParams(nextParams, { replace: true });
  };

  return <div className={`page ${styles.page}`}><PageHeader title="Persiapan program" description="Atur wilayah, jenis bantuan, jadwal, dan nilai baku sebelum data DCP3 diproses." context={<div className={styles.contextMark}><span aria-hidden="true" />Satu data lintas kabupaten</div>} />
    <Tabs value={activeTab} onValueChange={selectWorkspace} className={styles.tabs}>
      <TabsList activateOnFocus variant="line" className={styles.tabList} aria-label="Persiapan program">
        <TabsTrigger className={styles.tab} value="regencies"><TabLabel icon={Building2} count={regencies.data?.data.length} pending={regencies.isPending}>Kabupaten</TabLabel></TabsTrigger>
        <TabsTrigger className={styles.tab} value="programs"><TabLabel icon={ClipboardList} count={programs.data?.data.length} pending={programs.isPending}>Program</TabLabel></TabsTrigger>
        <TabsTrigger className={styles.tab} value="schedules"><TabLabel icon={CalendarDays} count={schedules.data?.data.length} pending={schedules.isPending}>Jadwal</TabLabel></TabsTrigger>
        <TabsTrigger className={styles.tab} value="templates"><TabLabel icon={Layers3} count={(packages.data?.data.length ?? 0) + (documents.data?.data.length ?? 0)} pending={packages.isPending || documents.isPending}>Template</TabLabel></TabsTrigger>
      </TabsList>
      <TabsContent className={styles.panel} value="regencies"><RegenciesPanel /></TabsContent>
      <TabsContent className={styles.panel} value="programs"><ProgramsPanel /></TabsContent>
      <TabsContent className={styles.panel} value="schedules"><SchedulesPanel /></TabsContent>
      <TabsContent className={styles.panel} value="templates"><TemplatesPanel /></TabsContent>
    </Tabs>
  </div>;
}
