import type { DCP3Mapping } from './types';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../components/ui/select';
import styles from './DCP3Import.module.css';

type MappingField = { key: keyof DCP3Mapping; label: string; required?: boolean };
const unmappedValue = 'unmapped';

const sharedFields: MappingField[] = [
  { key: 'full_name', label: 'Nama lengkap', required: true },
  { key: 'nik', label: 'NIK' },
  { key: 'address', label: 'Alamat' },
  { key: 'village', label: 'Desa / kelurahan' },
  { key: 'district', label: 'Kecamatan' },
  { key: 'phone_number', label: 'Nomor telepon' },
];

export function ColumnMappingStep({ headers, mapping, programType, multipleMachineVariants, onChange }: {
  headers: string[];
  mapping: DCP3Mapping;
  programType: 'farmer' | 'fisherman';
  multipleMachineVariants: boolean;
  onChange: (mapping: DCP3Mapping) => void;
}) {
  const sectorField: MappingField = programType === 'farmer'
    ? { key: 'farmer_card_number', label: 'Nomor kartu petani' }
    : { key: 'kusuka_number', label: 'Nomor kartu KUSUKA' };
  const fields: MappingField[] = [...sharedFields.slice(0, 2), sectorField, ...sharedFields.slice(2)];
  if (multipleMachineVariants) fields.push({ key: 'machine_option', label: 'Varian mesin', required: true });
  return <div className={styles.mappingGrid}>
    {fields.map((field) => {
      const labelID = `dcp3-mapping-${field.key}`;
      const selectedHeaderIndex = headers.indexOf(mapping[field.key]);
      return <div className={styles.mappingField} key={field.key}>
        <label id={labelID}>{field.label}{field.required && <strong aria-label="wajib">*</strong>}</label>
        <Select value={selectedHeaderIndex === -1 ? unmappedValue : String(selectedHeaderIndex)} onValueChange={(value) => {
          const headerIndex = value === unmappedValue ? -1 : Number(value);
          onChange({ ...mapping, [field.key]: headers[headerIndex] ?? '' });
        }}>
          <SelectTrigger className="w-full" aria-labelledby={labelID}><SelectValue placeholder="Tidak dipetakan" /></SelectTrigger>
          <SelectContent>
            <SelectItem value={unmappedValue}>Tidak dipetakan</SelectItem>
            {headers.map((header, index) => <SelectItem value={String(index)} key={`${header}-${index}`}>{header}</SelectItem>)}
          </SelectContent>
        </Select>
      </div>;
    })}
  </div>;
}
