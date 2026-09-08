import { PageHeader } from '../../components/PageHeader';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '../../components/ui/tabs';
import { RegenciesPanel } from './RegenciesPanel';
import { ProgramsPanel } from './ProgramsPanel';
import { SchedulesPanel } from './SchedulesPanel';
import { TemplatesPanel } from './TemplatesPanel';
import styles from './ProgramSetup.module.css';

export function ProgramSetupPage() {
  return <div className={`page ${styles.page}`}><PageHeader title="Persiapan program" description="Atur wilayah, jenis bantuan, jadwal, dan nilai baku sebelum data DCP3 diproses." context={<div className={styles.contextMark}><span aria-hidden="true" />Satu data lintas kabupaten</div>} />
    <Tabs defaultValue="regencies" className={styles.tabs}>
      <TabsList activateOnFocus variant="line" className={styles.tabList} aria-label="Persiapan program">
        <TabsTrigger className={styles.tab} value="regencies">Kabupaten</TabsTrigger>
        <TabsTrigger className={styles.tab} value="programs">Program</TabsTrigger>
        <TabsTrigger className={styles.tab} value="schedules">Jadwal</TabsTrigger>
        <TabsTrigger className={styles.tab} value="templates">Template</TabsTrigger>
      </TabsList>
      <TabsContent className={styles.panel} value="regencies"><RegenciesPanel /></TabsContent>
      <TabsContent className={styles.panel} value="programs"><ProgramsPanel /></TabsContent>
      <TabsContent className={styles.panel} value="schedules"><SchedulesPanel /></TabsContent>
      <TabsContent className={styles.panel} value="templates"><TemplatesPanel /></TabsContent>
    </Tabs>
  </div>;
}
