import { useQuery } from '@tanstack/react-query';
import { Plus } from 'lucide-react';
import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { DataState } from '@/components/DataState';
import { PageHeader } from '@/components/PageHeader';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { fetchProfile, fetchRequests, financeKey, type RequestType } from './api';
import { RequestRow } from './shared';

const descriptions: Record<RequestType, string> = {
  cashless: 'Pembayaran langsung ke rekening penerima. Selesai setelah finance mengunggah bukti bayar.',
  kasbon: 'Uang muka untuk tim. Wajib dilaporkan dengan nota setelah kegiatan.',
};

export function RequestsPage() {
  const [type, setType] = useState<RequestType>('cashless');
  const navigate = useNavigate();
  const profile = useQuery({ queryKey: [...financeKey, 'profile'], queryFn: fetchProfile });
  const requests = useQuery({
    queryKey: [...financeKey, 'mine', type],
    queryFn: () => fetchRequests(new URLSearchParams({ scope: 'mine', type, limit: '100' })),
  });
  const teams = profile.data?.data.leader_teams ?? [];
  const rows = requests.data?.data.rows ?? [];
  const drafts = rows.filter((request) => request.status === 'draft');
  const sent = rows.filter((request) => request.status !== 'draft');

  return <div className="space-y-6">
    <PageHeader
      title="Pengajuan keuangan"
      description="Buat pengajuan untuk tim Anda. Isian tersimpan otomatis sebagai draf, jadi tidak perlu mengisi ulang."
      actions={teams.length > 0 ? <Button onClick={() => navigate(`/keuangan/${crypto.randomUUID()}?baru=${type}`)}><Plus />Buat pengajuan</Button> : undefined}
    />
    {profile.data && teams.length === 0 && <DataState kind="empty" title="Anda belum menjadi leader tim" description="Minta admin menambahkan Anda ke tim di Keuangan, Tim keuangan." />}
    <Card>
      <CardContent className="grid gap-4 pt-5">
        <Tabs value={type} onValueChange={(value) => setType(value as RequestType)}>
          <TabsList variant="line" aria-label="Jenis pengajuan">
            <TabsTrigger value="cashless">Cashless</TabsTrigger>
            <TabsTrigger value="kasbon">Kasbon</TabsTrigger>
          </TabsList>
        </Tabs>
        <p className="text-sm text-muted-foreground">{descriptions[type]}</p>
        {requests.isPending && <DataState kind="loading" title="Memuat pengajuan" description="Mengambil daftar pengajuan Anda." />}
        {requests.isError && <DataState kind="error" title="Pengajuan belum dapat dimuat" description="Periksa koneksi lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => requests.refetch() }} />}
        {requests.isSuccess && rows.length === 0 && <DataState kind="empty" title="Belum ada pengajuan" description="Pengajuan dan draf Anda akan tampil di sini." />}
        {drafts.length > 0 && <section className="grid gap-2">
          <h2 className="text-sm font-semibold">Draf ({drafts.length})</h2>
          <ul className="grid gap-2" aria-label="Draf pengajuan">{drafts.map((request) => <RequestRow key={request.id} request={request} />)}</ul>
        </section>}
        {sent.length > 0 && <section className="grid gap-2">
          {drafts.length > 0 && <h2 className="text-sm font-semibold">Sudah dikirim ({sent.length})</h2>}
          <ul className="grid gap-2" aria-label="Daftar pengajuan">{sent.map((request) => <RequestRow key={request.id} request={request} />)}</ul>
        </section>}
      </CardContent>
    </Card>
  </div>;
}
