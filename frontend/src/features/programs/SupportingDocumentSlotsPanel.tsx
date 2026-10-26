import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Plus } from 'lucide-react';
import { useState } from 'react';
import { toast } from 'sonner';
import { Button } from '../../components/ui/button';
import { Checkbox } from '../../components/ui/checkbox';
import { FormField } from '../../components/FormField';
import { apiRequest, ApiError } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import type { SupportingDocumentSlot } from '../activities/types';

type DataResponse<T> = { data: T };

export function SupportingDocumentSlotsPanel({ regencyId }: { regencyId: string }) {
  const canManage = useCan('programs.manage');
  const client = useQueryClient();
  const [name, setName] = useState('');
  const slots = useQuery({
    queryKey: ['supporting-document-slots', regencyId, 'all'],
    // Deactivated names stay listed here so they can be switched back on.
    queryFn: () => apiRequest<DataResponse<SupportingDocumentSlot[]>>(`/api/v1/activities/supporting-documents/slots?regency_id=${encodeURIComponent(regencyId)}&include_inactive=true`),
    enabled: Boolean(regencyId),
  });
  const create = useMutation({
    mutationFn: () => apiRequest<DataResponse<SupportingDocumentSlot>>('/api/v1/activities/supporting-documents/slots', { method: 'POST', body: JSON.stringify({ regency_id: regencyId, name }) }),
    onSuccess: () => { setName(''); void client.invalidateQueries({ queryKey: ['supporting-document-slots', regencyId] }); },
    onError: (error) => toast.error(error instanceof ApiError || error instanceof Error ? error.message : 'Dokumen pendukung belum dapat ditambahkan.'),
  });
  const toggle = useMutation({
    mutationFn: (slot: SupportingDocumentSlot) => apiRequest<DataResponse<SupportingDocumentSlot>>(`/api/v1/activities/supporting-documents/slots/${slot.id}`, { method: 'PATCH', body: JSON.stringify({ is_active: !slot.is_active }) }),
    onSuccess: () => void client.invalidateQueries({ queryKey: ['supporting-document-slots', regencyId] }),
    onError: (error) => toast.error(error instanceof ApiError || error instanceof Error ? error.message : 'Status dokumen pendukung belum dapat diubah.'),
  });
  const items = slots.data?.data ?? [];

  return <div className="space-y-3">
    {slots.isPending ? <p className="text-sm text-muted-foreground">Memuat dokumen pendukung...</p>
      : items.length === 0 ? <p className="text-sm text-muted-foreground">Belum ada nama dokumen pendukung untuk kabupaten ini.</p>
      : <ul className="space-y-2">{items.map((slot) => <li key={slot.id} className="flex items-center justify-between gap-2 text-sm">
          <span className={slot.is_active ? '' : 'text-muted-foreground line-through'}>{slot.name}</span>
          <label className="flex items-center gap-2">
            <Checkbox checked={slot.is_active} disabled={!canManage || toggle.isPending} aria-label={`Aktifkan ${slot.name}`} onCheckedChange={() => toggle.mutate(slot)} />
            <span className="text-xs text-muted-foreground">Aktif</span>
          </label>
        </li>)}</ul>}
    {canManage && <form className="flex items-end gap-2" onSubmit={(event) => { event.preventDefault(); if (name.trim()) create.mutate(); }}>
      <FormField label="Nama dokumen baru" name="new_slot_name" value={name} onChange={(event) => setName(event.target.value)} />
      <Button type="submit" disabled={!name.trim() || create.isPending}><Plus aria-hidden="true" />Tambah</Button>
    </form>}
  </div>;
}
