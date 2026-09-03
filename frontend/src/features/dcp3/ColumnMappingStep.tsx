import type { DCP3Mapping } from './types';
import styles from './DCP3Import.module.css';

type MappingField = { key: keyof DCP3Mapping; label: string; required?: boolean };

const sharedFields: MappingField[] = [
  { key: 'source_sequence', label: 'Nomor urut DCP3', required: true },
  { key: 'full_name', label: 'Nama lengkap', required: true },
  { key: 'nik', label: 'NIK' },
  { key: 'address', label: 'Alamat' },
  { key: 'village', label: 'Desa / kelurahan' },
  { key: 'district', label: 'Kecamatan' },
  { key: 'phone_number', label: 'Nomor telepon' },
];

export function ColumnMappingStep({ headers, mapping, programType, onChange }: {
  headers: string[];
  mapping: DCP3Mapping;
  programType: 'farmer' | 'fisherman';
  onChange: (mapping: DCP3Mapping) => void;
}) {
  const sectorField: MappingField = programType === 'farmer'
    ? { key: 'farmer_card_number', label: 'Nomor kartu petani' }
    : { key: 'kusuka_number', label: 'Nomor kartu KUSUKA' };
  return <div className={styles.mappingGrid}>
    {[...sharedFields.slice(0, 3), sectorField, ...sharedFields.slice(3)].map((field) => <label className={styles.mappingField} key={field.key}>
      <span>{field.label}{field.required && <strong aria-label="wajib">*</strong>}</span>
      <select value={mapping[field.key]} onChange={(event) => onChange({ ...mapping, [field.key]: event.target.value })}>
        <option value="">Tidak dipetakan</option>
        {headers.map((header) => <option value={header} key={header}>{header}</option>)}
      </select>
    </label>)}
  </div>;
}
