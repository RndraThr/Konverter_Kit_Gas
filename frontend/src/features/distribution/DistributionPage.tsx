import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { apiRequest } from '../../lib/api';
import { RecipientSearch } from './RecipientSearch';
import { RecipientWorkspace } from './RecipientWorkspace';
import type { DataResponse, RecipientWorkspaceData, ScheduleResponse, SearchResult } from './types';
import styles from './Distribution.module.css';

export function DistributionPage() {
	const queryClient = useQueryClient();
  const [scheduleID, setScheduleID] = useState('');
  const [query, setQuery] = useState('');
  const [debouncedQuery, setDebouncedQuery] = useState('');
  const [allocationID, setAllocationID] = useState('');
  const [workspace, setWorkspace] = useState<RecipientWorkspaceData | null>(null);
  const schedules = useQuery({ queryKey: ['program-setup', 'schedules'], queryFn: () => apiRequest<ScheduleResponse>('/api/v1/program-setup/schedules') });
  useEffect(() => { const timer = window.setTimeout(() => setDebouncedQuery(query.trim()), 300); return () => window.clearTimeout(timer); }, [query]);
  useEffect(() => { setQuery(''); setDebouncedQuery(''); setAllocationID(''); setWorkspace(null); }, [scheduleID]);
  const canSearch = Boolean(scheduleID && (debouncedQuery.length >= 2 || /^\d+$/.test(debouncedQuery)));
  const search = useQuery({
    queryKey: ['distribution', 'search', scheduleID, debouncedQuery],
    queryFn: () => apiRequest<DataResponse<SearchResult[]>>(`/api/v1/distribution/search?schedule_id=${encodeURIComponent(scheduleID)}&q=${encodeURIComponent(debouncedQuery)}&limit=20`),
    enabled: canSearch,
  });
  const detail = useQuery({
    queryKey: ['distribution', 'allocation', allocationID],
    queryFn: () => apiRequest<DataResponse<RecipientWorkspaceData>>(`/api/v1/distribution/allocations/${allocationID}`),
    enabled: Boolean(allocationID),
  });
  useEffect(() => { if (detail.data?.data) setWorkspace(detail.data.data); }, [detail.data]);
  const selectedSchedule = schedules.data?.data.find((schedule) => schedule.id === scheduleID);

  return <div className={`page ${styles.page}`}>
    <header className="pageHeader"><div><h1>Pendistribusian</h1><p>Cari penerima, periksa data, dan lengkapi dokumentasi pembagian.</p></div>{selectedSchedule && <span className={styles.context}><strong>{selectedSchedule.regency?.document_code}</strong>{selectedSchedule.name}</span>}</header>
    <section className={styles.lookup}>
      <label className={styles.scheduleField}><span>Jadwal distribusi</span><select value={scheduleID} onChange={(event) => setScheduleID(event.target.value)}><option value="">Pilih kabupaten dan jadwal</option>{schedules.data?.data.filter((schedule) => schedule.status === 'active').map((schedule) => <option value={schedule.id} key={schedule.id}>{schedule.regency?.name} / {schedule.name}</option>)}</select></label>
		<RecipientSearch query={query} disabled={!scheduleID} loading={search.isFetching} results={search.data?.data ?? []} onQueryChange={(value) => { setQuery(value); setAllocationID(''); setWorkspace(null); }} onSelect={(id) => { setAllocationID(id); setQuery(''); setDebouncedQuery(''); }} />
    </section>
    {allocationID && detail.isPending && <p className={styles.loading}>Memuat data lengkap penerima...</p>}
	{workspace && <RecipientWorkspace data={workspace} onSaved={(next) => { setWorkspace(next); if (next.distribution_status === 'completed') { void queryClient.invalidateQueries({ queryKey: ['distribution'] }); } }} />}
  </div>;
}
