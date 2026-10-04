import { FormEvent, lazy, Suspense, useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { Cog, Lock, ScanBarcode } from 'lucide-react';
import type { DistributionSlot, EquipmentOption, UpdateEquipmentInput } from './types';
import { DocumentationSlot } from './DocumentationSlot';
import { PosSectionShell } from './PosSectionShell';
import { apiRequest, ApiError } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Button } from '@/components/ui/button';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { uppercaseBusinessText } from '@/lib/text';

const BarcodeScanner = lazy(() => import('./BarcodeScanner'));

type SlotMesinSectionProps = {
	slot: DistributionSlot;
	machineOptions: EquipmentOption[];
	converterOptions: EquipmentOption[];
	hoseOptions: EquipmentOption[];
	onChanged: (slot: DistributionSlot) => void;
};

type SerialField = 'machine_serial_number' | 'converter_serial_number';

// Keeps a historical option code selectable/visible even once it's no longer part of the
// schedule's current package template (e.g. a discontinued machine variant), instead of the
// select silently showing blank.
function withLegacyOption(options: EquipmentOption[], code: string | undefined): EquipmentOption[] {
	if (!code || options.some((option) => option.code === code)) return options;
	return [...options, { code, brand: code }];
}

export function SlotMesinSection({ slot, machineOptions, converterOptions, hoseOptions, onChanged }: SlotMesinSectionProps) {
	const canManage = useCan('distribution.pos_mesin');
	const [distributionDate, setDistributionDate] = useState(slot.distribution_date);
	const [equipment, setEquipment] = useState<UpdateEquipmentInput>({
		machine_option_code: slot.machine_option_code ?? '', machine_serial_number: slot.machine_serial_number ?? '',
		hose_option_code: slot.hose_option_code ?? '', hose_serial_number: '',
		converter_option_code: slot.converter_option_code ?? '', converter_serial_number: slot.converter_serial_number ?? '',
	});
	const [scanning, setScanning] = useState<SerialField | null>(null);
	const documentation = slot.documentation.filter((item) => item.stage === 'mesin');
	const dateLocked = slot.documentation.some((item) => (item.files?.length ?? 0) > 0);
	const equipmentLocked = documentation.some((item) => (item.files?.length ?? 0) > 0);
	const updateDocumentation = (next: DistributionSlot['documentation'][number]) => onChanged({ ...slot, documentation: slot.documentation.map((item) => item.code === next.code ? next : item) });
	const updateDate = useMutation({
		mutationFn: () => apiRequest<{ data: DistributionSlot }>(`/api/v1/distribution/slots/${slot.slot_number}/date?schedule_id=${encodeURIComponent(slot.schedule_id)}`, { method: 'PATCH', body: JSON.stringify({ distribution_date: distributionDate }) }),
		onSuccess: ({ data }) => onChanged(data),
	});
	const updateEquipment = useMutation({
		mutationFn: () => apiRequest<{ data: DistributionSlot }>(`/api/v1/distribution/slots/${slot.slot_number}/equipment?schedule_id=${encodeURIComponent(slot.schedule_id)}`, { method: 'PATCH', body: JSON.stringify(equipment) }),
		onSuccess: ({ data }) => onChanged(data),
	});

	const machineItems = withLegacyOption(machineOptions, slot.machine_option_code);
	const converterItems = withLegacyOption(converterOptions, slot.converter_option_code);
	const hoseItems = withLegacyOption(hoseOptions, slot.hose_option_code);

	const serialField = (label: string, field: SerialField) => <div className="grid min-w-0 gap-2">
		<Label htmlFor={field}>{label}</Label>
		<div className="flex gap-2">
			<Input id={field} name={field} className="flex-1" disabled={equipmentLocked || !canManage} value={equipment[field]} onChange={(event) => setEquipment({ ...equipment, [field]: uppercaseBusinessText(event.target.value) })} />
			{canManage && !equipmentLocked && <Button type="button" variant="outline" size="icon" aria-label={`Scan ${label}`} title="Scan barcode" onClick={() => setScanning(field)}><ScanBarcode aria-hidden="true" /></Button>}
		</div>
	</div>;

	return <PosSectionShell label="POS Mesin" badge="POS Mesin" icon={<Cog aria-hidden="true" />} title={`Nomor bagi #${slot.slot_number}`} state="done" status="Tercatat">
		<div className="rounded-lg border bg-muted/30 p-3">
			<div className="flex flex-wrap items-end gap-3">
				<div className="grid min-w-52 flex-1 gap-2">
					<Label htmlFor={`distribution-date-${slot.id}`}>Tanggal distribusi</Label>
					<Input id={`distribution-date-${slot.id}`} type="date" required disabled={dateLocked || !canManage} value={distributionDate} onChange={(event) => setDistributionDate(event.target.value)} />
				</div>
				{canManage && !dateLocked && <Button type="button" variant="outline" disabled={updateDate.isPending || !distributionDate} onClick={() => updateDate.mutate()}>{updateDate.isPending ? 'Menyimpan...' : 'Simpan tanggal distribusi'}</Button>}
			</div>
			<p className="mt-2 text-xs text-muted-foreground">{dateLocked ? 'Tanggal dikunci setelah foto pertama diunggah.' : 'Tanggal ini digunakan oleh folder semua foto Pos 1–3.'}</p>
			{updateDate.isError && <Alert className="mt-3" variant="destructive"><AlertDescription>{updateDate.error instanceof ApiError ? updateDate.error.message : 'Tanggal distribusi belum dapat disimpan.'}</AlertDescription></Alert>}
		</div>

		<form className="grid gap-4 sm:grid-cols-2" onSubmit={(event: FormEvent) => { event.preventDefault(); updateEquipment.mutate(); }}>
			<div className="grid min-w-0 gap-2">
				<Label id={`mesin-machine-label-${slot.id}`}>Merk/Tipe Mesin</Label>
				<Select disabled={equipmentLocked || !canManage} value={equipment.machine_option_code} onValueChange={(value) => setEquipment({ ...equipment, machine_option_code: value ?? '' })}>
					<SelectTrigger className="w-full" aria-labelledby={`mesin-machine-label-${slot.id}`}><SelectValue placeholder="Pilih mesin" /></SelectTrigger>
					<SelectContent>{machineItems.map((option) => <SelectItem key={option.code} value={option.code}>{option.brand} {option.type}</SelectItem>)}</SelectContent>
				</Select>
			</div>
			{serialField('Serial Number Mesin', 'machine_serial_number')}
			<div className="grid min-w-0 gap-2">
				<Label id={`mesin-converter-label-${slot.id}`}>Merk Konkit/Reducer</Label>
				<Select disabled={equipmentLocked || !canManage} value={equipment.converter_option_code} onValueChange={(value) => setEquipment({ ...equipment, converter_option_code: value ?? '' })}>
					<SelectTrigger className="w-full" aria-labelledby={`mesin-converter-label-${slot.id}`}><SelectValue placeholder="Pilih konkit/reducer" /></SelectTrigger>
					<SelectContent>{converterItems.map((option) => <SelectItem key={option.code} value={option.code}>{option.brand}</SelectItem>)}</SelectContent>
				</Select>
			</div>
			{serialField('Serial Number Konkit/Reducer', 'converter_serial_number')}
			<div className="grid min-w-0 gap-2">
				<Label id={`mesin-hose-label-${slot.id}`}>Merk/Spesifikasi Selang</Label>
				<Select disabled={equipmentLocked || !canManage} value={equipment.hose_option_code} onValueChange={(value) => setEquipment({ ...equipment, hose_option_code: value ?? '' })}>
					<SelectTrigger className="w-full" aria-labelledby={`mesin-hose-label-${slot.id}`}><SelectValue placeholder="Pilih selang" /></SelectTrigger>
					<SelectContent>{hoseItems.map((option) => <SelectItem key={option.code} value={option.code}>{option.brand} {option.spec}</SelectItem>)}</SelectContent>
				</Select>
			</div>
			<div className="grid min-w-0 gap-2">
				<Label htmlFor={`mesin-hose-serial-${slot.id}`}>Serial Number Selang</Label>
				<Input id={`mesin-hose-serial-${slot.id}`} value="-" disabled />
			</div>
			{equipmentLocked
				? <p className="sm:col-span-2 flex items-center gap-1.5 text-xs text-muted-foreground"><Lock className="size-3.5" aria-hidden="true" />Data mesin dikunci setelah foto POS Mesin diunggah.</p>
				: canManage && <Button className="sm:col-span-2" type="submit" disabled={updateEquipment.isPending}>{updateEquipment.isPending ? 'Menyimpan...' : 'Simpan data mesin'}</Button>}
			{updateEquipment.isError && <Alert className="sm:col-span-2" variant="destructive"><AlertDescription>{updateEquipment.error instanceof ApiError ? updateEquipment.error.message : 'Data mesin belum dapat disimpan.'}</AlertDescription></Alert>}
		</form>

		{documentation.length > 0 && <div className="grid gap-4 sm:grid-cols-2">{documentation.map((item) => <DocumentationSlot key={item.code} slot={item} onChanged={updateDocumentation} />)}</div>}
		{scanning && <Suspense fallback={null}>
			<BarcodeScanner onResult={(text) => { setEquipment((prev) => ({ ...prev, [scanning]: uppercaseBusinessText(text) })); setScanning(null); }} onClose={() => setScanning(null)} />
		</Suspense>}
	</PosSectionShell>;
}
