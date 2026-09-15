import { FormEvent, useEffect, useState } from 'react';
import { X } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '../../components/ui/dialog';
import { Alert, AlertDescription } from '../../components/ui/alert';
import { FormField } from '../../components/FormField';
import { Label } from '../../components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../components/ui/select';
import type { Recipient, RecipientInput } from './types';

export type ScheduleOption = { id: string; name: string; regency_name: string; program_type: 'farmer' | 'fisherman' };

const empty: RecipientInput = { schedule_id: '', full_name: '', nik: '', sector_identifier: '', address: '', village: '', district: '', phone_number: '' };

export function RecipientDialog({ open, onOpenChange, recipient, schedules, pending, error, fields = {}, onSave }: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  recipient?: Recipient;
  schedules: ScheduleOption[];
  pending?: boolean;
  error?: string;
  fields?: Record<string, string>;
  onSave: (values: RecipientInput) => void;
}) {
  const [values, setValues] = useState<RecipientInput>(empty);
  useEffect(() => setValues(recipient ? {
    schedule_id: recipient.schedule_id, full_name: recipient.full_name, nik: recipient.nik, sector_identifier: recipient.sector_identifier,
    address: recipient.address, village: recipient.village, district: recipient.district, phone_number: recipient.phone_number,
  } : empty), [recipient, open]);

  const title = recipient ? `Edit ${recipient.full_name}` : 'Tambah penerima';
  const selectedSchedule = schedules.find((item) => item.id === values.schedule_id);
  const sectorLabel = selectedSchedule?.program_type === 'fisherman' ? 'Nomor KUSUKA' : 'Nomor kartu petani';

  return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent showCloseButton={false} aria-label={title} className="max-h-[calc(100dvh-2rem)] max-w-2xl overflow-y-auto p-0">
    <form onSubmit={(event: FormEvent) => { event.preventDefault(); onSave(values); }}>
      <DialogHeader className="relative border-b p-5 pr-16">
        <DialogTitle>{title}</DialogTitle>
        <DialogDescription>Identitas penerima yang dicatat dalam sistem.</DialogDescription>
        <DialogClose render={<Button variant="ghost" size="icon" type="button" aria-label="Tutup" className="absolute top-3 right-3" />}><X /></DialogClose>
      </DialogHeader>
      <div className="grid gap-4 p-5 sm:grid-cols-2">
        <div className="grid min-w-0 gap-2 sm:col-span-2">
          <Label id="recipient-schedule-label">Jadwal</Label>
          <Select required disabled={Boolean(recipient)} value={values.schedule_id} onValueChange={(value) => setValues({ ...values, schedule_id: value ?? '' })}>
            <SelectTrigger className="w-full" aria-labelledby="recipient-schedule-label"><SelectValue placeholder="Pilih kabupaten dan jadwal" /></SelectTrigger>
            <SelectContent>{schedules.map((item) => <SelectItem key={item.id} value={item.id}>{item.regency_name} / {item.name}</SelectItem>)}</SelectContent>
          </Select>
        </div>
        <FormField className="sm:col-span-2" error={fields.full_name} label="Nama lengkap" name="full_name" required value={values.full_name} onChange={(event) => setValues({ ...values, full_name: event.target.value.toUpperCase() })} />
        <FormField error={fields.nik} label="NIK" name="nik" maxLength={16} value={values.nik} onChange={(event) => setValues({ ...values, nik: event.target.value.replace(/\D/g, '') })} hint="16 digit, boleh dikosongkan." />
        <FormField label={sectorLabel} name="sector_identifier" value={values.sector_identifier} onChange={(event) => setValues({ ...values, sector_identifier: event.target.value.toUpperCase() })} />
        <FormField className="sm:col-span-2" label="Alamat" name="address" value={values.address} onChange={(event) => setValues({ ...values, address: event.target.value })} />
        <FormField label="Desa/kelurahan" name="village" value={values.village} onChange={(event) => setValues({ ...values, village: event.target.value })} />
        <FormField label="Kecamatan" name="district" value={values.district} onChange={(event) => setValues({ ...values, district: event.target.value })} />
        <FormField label="Nomor telepon" name="phone_number" value={values.phone_number} onChange={(event) => setValues({ ...values, phone_number: event.target.value })} />
        {error && <Alert className="sm:col-span-2" variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>}
      </div>
      <DialogFooter className="mx-0 mb-0"><DialogClose render={<Button variant="outline" type="button" />}>Batal</DialogClose><Button disabled={pending} type="submit">{pending ? 'Menyimpan...' : 'Simpan'}</Button></DialogFooter>
    </form>
  </DialogContent></Dialog>;
}
