import { AlertTriangle, Check, X } from 'lucide-react';
import { DataTable } from '../../components/DataTable';
import { Badge } from '../../components/ui/badge';
import type { DCP3Mapping, PreviewRow } from './types';
import styles from './DCP3Import.module.css';

export type PreviewStatus = 'valid' | 'warning' | 'conflict';

export function rowStatus(row: PreviewRow, mapping: DCP3Mapping): PreviewStatus {
  const sequence = Number(row.values[mapping.source_sequence]);
  if (!row.values[mapping.full_name]?.trim() || !Number.isInteger(sequence) || sequence < 1) return 'conflict';
  if (!row.values[mapping.nik]?.trim()) return 'warning';
  return 'valid';
}

const statusPresentation = {
  valid: { label: 'Valid', icon: Check },
  warning: { label: 'Peringatan', icon: AlertTriangle },
  conflict: { label: 'Konflik', icon: X },
};

export function ImportPreviewTable({ rows, mapping }: { rows: PreviewRow[]; mapping: DCP3Mapping }) {
  return <div className={styles.previewTable}>
    <DataTable label="Pratinjau data DCP3">
      <thead><tr><th>Baris Excel</th><th>No. DCP3</th><th>Nama penerima</th><th>NIK</th><th>Status awal</th></tr></thead>
      <tbody>{rows.map((row) => {
        const status = rowStatus(row, mapping);
        const presentation = statusPresentation[status];
        const Icon = presentation.icon;
        return <tr key={row.source_row_number}>
          <td>{row.source_row_number}</td>
          <td>{row.values[mapping.source_sequence] || '-'}</td>
          <td><strong>{row.values[mapping.full_name] || 'Nama belum tersedia'}</strong></td>
          <td>{row.values[mapping.nik] || '-'}</td>
          <td><Badge variant="outline" className={`${styles.rowStatus} ${styles[status]}`}><Icon aria-hidden="true" />{presentation.label}</Badge></td>
        </tr>;
      })}</tbody>
    </DataTable>
  </div>;
}
