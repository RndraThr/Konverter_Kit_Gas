import { FormEvent, useEffect, useState } from 'react';
import { X } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '../../components/ui/dialog';
import { Alert, AlertDescription } from '../../components/ui/alert';
import { FormField } from '../../components/FormField';
import { Label } from '../../components/ui/label';
import { SearchableSelect } from '../../components/SearchableSelect';
import type { Recipient, RecipientInput } from './types';
import { uppercaseBusinessText } from '../../lib/text';

export type ScheduleOption = { id: string; name: string; regency_name: string; program_type: 'farmer' | 'fisherman' };

const empty: RecipientInput = { schedule_id: '', full_name: '', nik: '', sector_identifier: '', address: '', village: '', district: '', phone_number: '' };

/**
 * Adds or edits a recipient. From the distribution NIK search it opens with
 * the schedule fixed (`initial.schedule_id` plus `fixedSchedule`) and the NIK
 * typed so far, and then needs a full NIK so the recipient can be linked.
 */
export function RecipientDialog({ open, onOpenChange, recipient, schedules, pending, error, fields = {}, onSave, initial, fixedSchedule, requireNIK, description }: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  recipient?: Recipient;
  schedules: ScheduleOption[];
  pending?: boolean;
  error?: string;
  fields?: Record<string, string>;
  onSave: (values: RecipientInput) => void;
  initial?: Partial<RecipientInput>;
  fixedSchedule?: string;
  requireNIK?: boolean;
  description?: string;
}) {
  const [values, setValues] = useState<RecipientInput>(empty);
  useEffect(() => setValues(recipient ? {
    schedule_id: recipient.schedule_id, full_name: recipient.full_name, nik: recipient.nik, sector_identifier: recipient.sector_identifier,
    address: recipient.address, village: recipient.village, district: recipient.district, phone_number: recipient.phone_number,
  } : { ...empty, ...initial }), [recipient, open]); // eslint-disable-line react-hooks/exhaustive-deps -- initial is read when the dialog opens

  const title = recipient ? `Edit ${recipient.full_name}` : 'Tambah penerima';
  const nikError = requireNIK && values.nik.length !== 16 ? 'NIK 16 digit wajib diisi agar penerima bisa langsung dihubungkan.' : undefined;
  const selectedSchedule = schedules.find((item) => item.id === values.schedule_id);
  const sectorLabel = selectedSchedule?.program_type === 'fisherman' ? 'Nomor KUSUKA' : 'Nomor kartu petani';

  return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent showCloseButton={false} aria-label={title} className="max-h-[calc(100dvh-2rem)] max-w-2xl overflow-y-auto p-0">
    <form onSubmit={(event: FormEvent) => { event.preventDefault(); onSave(values); }}>
      <DialogHeader className="relative border-b p-5 pr-16">
        <DialogTitle>{title}</DialogTitle>
        <DialogDescription>{description ?? 'Identitas penerima yang dicatat dalam sistem.'}</DialogDescription>
        <DialogClose render={<Button variant="ghost" size="icon" type="button" aria-label="Tutup" className="absolute top-3 right-3" />}><X /></DialogClose>
      </DialogHeader>
      <div className="grid gap-4 p-5 sm:grid-cols-2">
        {fixedSchedule ? <p className="text-sm sm:col-span-2"><span className="text-muted-foreground">Jadwal:</span> {fixedSchedule}</p> : <div className="grid min-w-0 gap-2 sm:col-span-2">
          <Label id="recipient-schedule-label">Jadwal</Label>
          <SearchableSelect required disabled={Boolean(recipient)} value={values.schedule_id ?? ''} onValueChange={(value) => setValues({ ...values, schedule_id: value })} aria-labelledby="recipient-schedule-label" placeholder="Cari kabupaten atau jadwal" emptyText="Jadwal tidak ditemukan." options={schedules.map((item) => ({ value: item.id, label: `${item.regency_name} / ${item.name}` }))} />
        </div>}
        <FormField className="sm:col-span-2" error={fields.full_name} label="Nama lengkap" name="full_name" required value={values.full_name} onChange={(event) => setValues({ ...values, full_name: uppercaseBusinessText(event.target.value) })} />
        <FormField error={fields.nik ?? (values.nik ? nikError : undefined)} label="NIK" name="nik" required={requireNIK} maxLength={16} inputMode="numeric" value={values.nik} onChange={(event) => setValues({ ...values, nik: event.target.value.replace(/\D/g, '') })} hint={requireNIK ? '16 digit.' : '16 digit, boleh dikosongkan.'} />
        <FormField label={sectorLabel} name="sector_identifier" value={values.sector_identifier} onChange={(event) => setValues({ ...values, sector_identifier: uppercaseBusinessText(event.target.value) })} />
        <FormField className="sm:col-span-2" label="Alamat" name="address" value={values.address} onChange={(event) => setValues({ ...values, address: uppercaseBusinessText(event.target.value) })} />
        <FormField label="Desa/kelurahan" name="village" value={values.village} onChange={(event) => setValues({ ...values, village: uppercaseBusinessText(event.target.value) })} />
        <FormField label="Kecamatan" name="district" value={values.district} onChange={(event) => setValues({ ...values, district: uppercaseBusinessText(event.target.value) })} />
        <FormField label="Nomor telepon" name="phone_number" value={values.phone_number} onChange={(event) => setValues({ ...values, phone_number: event.target.value })} />
        {error && <Alert className="sm:col-span-2" variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>}
      </div>
      <DialogFooter className="mx-0 mb-0"><DialogClose render={<Button variant="outline" type="button" />}>Batal</DialogClose><Button disabled={pending || Boolean(nikError)} type="submit">{pending ? 'Menyimpan...' : 'Simpan'}</Button></DialogFooter>
    </form>
  </DialogContent></Dialog>;
}
