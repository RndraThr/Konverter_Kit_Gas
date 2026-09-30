import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { AlertTriangle, MapPinned, Plus } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { DataState } from '@/components/DataState';
import { apiRequest } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import type { DataResponse, Program, ProgramZone, Regency } from './types';
import { programTypeLabel } from './types';

export function ZonesPanel() {
  const canManage = useCan('programs.manage');
  const client = useQueryClient();
  const [programID, setProgramID] = useState('');
  const [code, setCode] = useState('');
  const [name, setName] = useState('');
  const programs = useQuery({ queryKey: ['program-setup', 'programs'], queryFn: () => apiRequest<DataResponse<Program[]>>('/api/v1/program-setup/programs') });
  const regencies = useQuery({ queryKey: ['program-setup', 'regencies'], queryFn: () => apiRequest<DataResponse<Regency[]>>('/api/v1/program-setup/regencies') });
  useEffect(() => { if (!programID && programs.data?.data[0]) setProgramID(programs.data.data[0].id); }, [programID, programs.data]);
  const zones = useQuery({ queryKey: ['program-setup', 'programs', programID, 'zones'], queryFn: () => apiRequest<DataResponse<ProgramZone[]>>(`/api/v1/program-setup/programs/${programID}/zones`), enabled: Boolean(programID) });
  const refresh = () => client.invalidateQueries({ queryKey: ['program-setup', 'programs', programID, 'zones'] });
  const saveZone = useMutation({ mutationFn: () => apiRequest(`/api/v1/program-setup/programs/${programID}/zones`, { method: 'POST', body: JSON.stringify({ code, name, sort_order: zones.data?.data.length ?? 0 }) }), onSuccess: () => { setCode(''); setName(''); refresh(); toast.success('Zona berhasil ditambahkan.'); }, onError: () => toast.error('Zona belum dapat disimpan.') });
  const assign = useMutation({ mutationFn: ({ regencyID, zoneID }: { regencyID: string; zoneID: string }) => apiRequest(`/api/v1/program-setup/programs/${programID}/assignments/${regencyID}`, { method: 'PUT', body: JSON.stringify({ zone_id: zoneID }) }), onSuccess: () => { refresh(); toast.success('Kabupaten berhasil dipindahkan.'); }, onError: () => toast.error('Kabupaten belum dapat dipindahkan.') });
  const selectedProgram = programs.data?.data.find((item) => item.id === programID);
  const assignment = useMemo(() => new Map((zones.data?.data ?? []).flatMap((zone) => zone.regencies.map((regency) => [regency.id, zone] as const))), [zones.data]);
  const placeholder = zones.data?.data.find((zone) => zone.is_placeholder);

  return <section className="setupPanel" aria-label="Zona program">
    <div className="panelHeading"><div><h2><MapPinned />Zona tender</h2><p>Kelompokkan kabupaten per zona untuk menentukan struktur folder Drive.</p></div></div>
    <div className="grid gap-4 lg:grid-cols-[minmax(260px,0.7fr)_minmax(0,1.3fr)]">
      <Card><CardHeader><CardTitle>Pilih program</CardTitle></CardHeader><CardContent className="space-y-4">
        <Select value={programID} onValueChange={(value) => setProgramID(value ?? '')}><SelectTrigger aria-label="Program zona"><SelectValue placeholder="Pilih program" /></SelectTrigger><SelectContent>{programs.data?.data.map((program) => <SelectItem key={program.id} value={program.id}>{program.name}</SelectItem>)}</SelectContent></Select>
        {selectedProgram && <div className="flex flex-wrap gap-2"><Badge>{programTypeLabel(selectedProgram.program_type)}</Badge><Badge variant="outline">{selectedProgram.code}</Badge></div>}
        {canManage && <form className="space-y-3 border-t pt-4" onSubmit={(event) => { event.preventDefault(); saveZone.mutate(); }}><div className="grid gap-2"><Label htmlFor="zone-code">Kode zona</Label><Input id="zone-code" value={code} onChange={(event) => setCode(event.target.value.toUpperCase())} placeholder="ZONA-1" required /></div><div className="grid gap-2"><Label htmlFor="zone-name">Nama zona</Label><Input id="zone-name" value={name} onChange={(event) => setName(event.target.value)} placeholder="Zona 1" required /></div><Button type="submit" disabled={!programID || saveZone.isPending}><Plus />Tambah zona</Button></form>}
      </CardContent></Card>
      <div className="space-y-4">
        {zones.isPending ? <DataState kind="loading" title="Memuat zona" description="Mengambil susunan zona dan kabupaten." /> : zones.isError ? <DataState kind="error" title="Zona belum dapat dimuat" description="Periksa koneksi lalu coba kembali." action={{ label: 'Coba lagi', onClick: () => zones.refetch() }} /> : <>
          {placeholder && placeholder.regencies.length > 0 && <div className="flex gap-3 rounded-lg border border-amber-300 bg-amber-50 p-4 text-amber-950"><AlertTriangle className="size-5 shrink-0" /><div><strong>{placeholder.regencies.length} kabupaten belum memiliki zona</strong><p className="mt-1 text-sm">Upload dokumen untuk kabupaten tersebut akan ditahan sampai zona dipilih.</p></div></div>}
          <div className="grid gap-3 md:grid-cols-2">{(zones.data?.data ?? []).filter((zone) => !zone.is_placeholder).map((zone) => <Card key={zone.id}><CardHeader><CardTitle>{zone.name}</CardTitle><p className="text-xs text-muted-foreground">{zone.code} · {zone.regencies.length} kabupaten</p></CardHeader><CardContent className="flex flex-wrap gap-2">{zone.regencies.length ? zone.regencies.map((regency) => <Badge key={regency.id} variant="outline">{regency.document_code} · {regency.name}</Badge>) : <span className="text-sm text-muted-foreground">Belum ada kabupaten.</span>}</CardContent></Card>)}</div>
          {canManage && <Card><CardHeader><CardTitle>Penempatan kabupaten</CardTitle></CardHeader><CardContent className="grid gap-3 sm:grid-cols-2">{regencies.data?.data.map((regency) => <div key={regency.id} className="grid gap-2 rounded-lg border p-3"><Label>{regency.document_code} · {regency.name}</Label><Select value={assignment.get(regency.id)?.id ?? placeholder?.id ?? ''} onValueChange={(zoneID) => { if (zoneID) assign.mutate({ regencyID: regency.id, zoneID }); }}><SelectTrigger aria-label={`Zona ${regency.name}`}><SelectValue placeholder="Pilih zona" /></SelectTrigger><SelectContent>{zones.data?.data.map((zone) => <SelectItem key={zone.id} value={zone.id}>{zone.name}</SelectItem>)}</SelectContent></Select></div>)}</CardContent></Card>}
        </>}
      </div>
    </div>
  </section>;
}
