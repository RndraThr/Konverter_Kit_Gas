import { Tabs } from '@base-ui/react/tabs';
import { RegenciesPanel } from './RegenciesPanel';
import { ProgramsPanel } from './ProgramsPanel';
import { SchedulesPanel } from './SchedulesPanel';
import { TemplatesPanel } from './TemplatesPanel';
import styles from './ProgramSetup.module.css';

export function ProgramSetupPage() {
  return <div className={`page ${styles.page}`}><header className={`pageHeader ${styles.header}`}><div><h1>Persiapan program</h1><p>Atur wilayah, jenis bantuan, jadwal, dan nilai baku sebelum data DCP3 diproses.</p></div><div className={styles.contextMark}><span aria-hidden="true" />Satu data lintas kabupaten</div></header>
    <Tabs.Root defaultValue="regencies" className={styles.tabs}>
      <Tabs.List className={styles.tabList} aria-label="Persiapan program">
        <Tabs.Tab className={styles.tab} value="regencies">Kabupaten</Tabs.Tab>
        <Tabs.Tab className={styles.tab} value="programs">Program</Tabs.Tab>
        <Tabs.Tab className={styles.tab} value="schedules">Jadwal</Tabs.Tab>
        <Tabs.Tab className={styles.tab} value="templates">Template</Tabs.Tab>
        <Tabs.Indicator className={styles.indicator} />
      </Tabs.List>
      <Tabs.Panel className={styles.panel} value="regencies"><RegenciesPanel /></Tabs.Panel>
      <Tabs.Panel className={styles.panel} value="programs"><ProgramsPanel /></Tabs.Panel>
      <Tabs.Panel className={styles.panel} value="schedules"><SchedulesPanel /></Tabs.Panel>
      <Tabs.Panel className={styles.panel} value="templates"><TemplatesPanel /></Tabs.Panel>
    </Tabs.Root>
  </div>;
}
