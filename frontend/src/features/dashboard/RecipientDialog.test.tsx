import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { RecipientDialog } from './RecipientDialog';

const schedules = [{ id: 'schedule-1', name: 'Wajo Tahap 1', regency_name: 'Wajo', program_type: 'farmer' as const }];

test('requires selecting a schedule and full name before submit, then forwards trimmed values', async () => {
  const onSave = vi.fn();
  render(<RecipientDialog open onOpenChange={() => {}} schedules={schedules} onSave={onSave} />);

  await userEvent.click(screen.getByRole('combobox', { name: 'Jadwal' }));
  await userEvent.click(await screen.findByRole('option', { name: /Wajo Tahap 1/ }));
  await userEvent.type(screen.getByLabelText('Nama lengkap'), 'budi santoso');
  await userEvent.type(screen.getByLabelText('Alamat'), 'Jl. Melati');
  await userEvent.type(screen.getByLabelText('Desa/kelurahan'), 'Desa Baru');
  await userEvent.type(screen.getByLabelText('Kecamatan'), 'Wajo');
  await userEvent.type(screen.getByLabelText('Nomor telepon'), '081AbC');

  expect(screen.getByLabelText('Alamat')).toHaveValue('JL. MELATI');
  expect(screen.getByLabelText('Desa/kelurahan')).toHaveValue('DESA BARU');
  expect(screen.getByLabelText('Kecamatan')).toHaveValue('WAJO');
  expect(screen.getByLabelText('Nomor telepon')).toHaveValue('081AbC');
  await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));

  expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ schedule_id: 'schedule-1', full_name: 'BUDI SANTOSO' }));
});

test('shows KUSUKA label for fisherman schedules and locks schedule selection when editing', async () => {
  const fisherSchedules = [{ id: 'schedule-2', name: 'Bone Tahap 1', regency_name: 'Bone', program_type: 'fisherman' as const }];
  render(<RecipientDialog open onOpenChange={() => {}} schedules={fisherSchedules} recipient={{
    allocation_id: 'allocation-1', distribution_number: 1, allocation_status: 'ready', distribution_status: null,
    full_name: 'Siti', nik: '', sector_identifier_type: '', sector_identifier: '', address: '', village: '', district: '', phone_number: '',
    program_id: 'program-1', program_name: 'Program Nelayan', program_type: 'fisherman', zone_id: 'zone-1', zone_code: 'ZONA-1', zone_name: 'Zona 1', regency_id: 'regency-1', regency_name: 'Bone',
    regency_document_code: 'BON', schedule_id: 'schedule-2', schedule_name: 'Bone Tahap 1', evidence_slots: [],
  }} onSave={vi.fn()} />);

  expect(screen.getByRole('combobox', { name: 'Jadwal' })).toBeDisabled();
  expect(screen.getByLabelText('Nomor KUSUKA')).toBeInTheDocument();
});
