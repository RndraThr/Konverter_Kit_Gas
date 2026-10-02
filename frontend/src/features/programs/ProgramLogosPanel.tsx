import { useQuery } from '@tanstack/react-query';
import { Images } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { DataState } from '@/components/DataState';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { apiRequest } from '@/lib/api';
import { LogoTenderPanel } from '../berita-acara/LogoTenderPanel';
import type { DataResponse, Program } from './types';

export function ProgramLogosPanel() {
  const programs = useQuery({
    queryKey: ['program-setup', 'programs'],
    queryFn: () => apiRequest<DataResponse<Program[]>>('/api/v1/program-setup/programs'),
  });
  const [programID, setProgramID] = useState('');
  const items = useMemo(() => programs.data?.data ?? [], [programs.data?.data]);

  useEffect(() => {
    if (items.length && !items.some((program) => program.id === programID)) setProgramID(items[0].id);
  }, [items, programID]);

  return <section className="setupPanel" aria-labelledby="program-logos-heading">
    <header className="panelHeading">
      <div>
        <h2 id="program-logos-heading"><Images aria-hidden="true" />Logo dokumen tender</h2>
        <p>Atur kumpulan logo satu kali untuk seluruh dokumen dan kabupaten dalam program yang dipilih.</p>
      </div>
    </header>

    {programs.isPending
      ? <DataState kind="loading" title="Memuat program tender" description="Mengambil program yang dapat dikonfigurasi." />
      : programs.isError
        ? <DataState kind="error" title="Program tender belum dapat dimuat" description="Periksa koneksi, lalu coba kembali." action={{ label: 'Coba lagi', onClick: () => programs.refetch() }} />
        : items.length === 0
          ? <DataState kind="empty" title="Belum ada program tender" description="Tambahkan program terlebih dahulu sebelum mengatur logo dokumen." />
          : <>
            <div className="mb-5 grid max-w-xl gap-2">
              <Label id="program-logo-select-label">Program tender</Label>
              <Select value={programID} onValueChange={(value) => setProgramID(value ?? '')}>
                <SelectTrigger className="w-full" aria-labelledby="program-logo-select-label"><SelectValue /></SelectTrigger>
                <SelectContent>{items.map((program) => <SelectItem key={program.id} value={program.id}>{program.name} · {program.fiscal_year}</SelectItem>)}</SelectContent>
              </Select>
              <p className="text-xs leading-5 text-muted-foreground">Logo berlaku pada semua jadwal, zona, dan kabupaten yang berada dalam program ini.</p>
            </div>
            {programID && <LogoTenderPanel programID={programID} />}
          </>}
  </section>;
}
