import { useMutation, useQuery } from '@tanstack/react-query';
import { ArrowLeft, ArrowRight, CheckCircle2, FileSpreadsheet, Upload } from 'lucide-react';
import { useMemo, useState } from 'react';
import { ApiError, apiRequest } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { formatDate } from '../programs/types';
import { ColumnMappingStep } from './ColumnMappingStep';
import { ImportPreviewTable, rowStatus } from './ImportPreviewTable';
import { DataResponse, DCP3Mapping, DCP3Preview, emptyMapping, ImportResult, ScheduleResponse } from './types';
import styles from './DCP3Import.module.css';

const steps = ['Pilih jadwal', 'Upload DCP3', 'Cocokkan kolom', 'Periksa dan import'];

export function DCP3ImportPage() {
  const canImport = useCan('dcp3.import');
  const [step, setStep] = useState(1);
  const [scheduleID, setScheduleID] = useState('');
  const [file, setFile] = useState<File | null>(null);
  const [preview, setPreview] = useState<DCP3Preview | null>(null);
  const [mapping, setMapping] = useState<DCP3Mapping>(emptyMapping);
  const [result, setResult] = useState<ImportResult | null>(null);
  const schedules = useQuery({
    queryKey: ['program-setup', 'schedules'],
    queryFn: () => apiRequest<ScheduleResponse>('/api/v1/program-setup/schedules'),
  });
  const selectedSchedule = schedules.data?.data.find((schedule) => schedule.id === scheduleID);

  const upload = useMutation({
    mutationFn: async () => {
      if (!file) throw new Error('Pilih file DCP3 terlebih dahulu');
      const body = new FormData();
      body.set('schedule_id', scheduleID);
      body.set('file', file);
      return apiRequest<DataResponse<DCP3Preview>>('/api/v1/dcp3/previews', { method: 'POST', body });
    },
    onSuccess: ({ data }) => { setPreview(data); setMapping(emptyMapping); setStep(3); },
  });
  const commit = useMutation({
    mutationFn: () => apiRequest<DataResponse<ImportResult>>('/api/v1/dcp3/imports', {
      method: 'POST', body: JSON.stringify({ batch_id: preview?.id, mapping }),
    }),
    onSuccess: ({ data }) => setResult(data),
  });
  const summary = useMemo(() => {
    const values = { valid: 0, warning: 0, conflict: 0 };
    preview?.rows.forEach((row) => { values[rowStatus(row, mapping)] += 1; });
    return values;
  }, [mapping, preview]);

  return <div className={`page ${styles.page}`}>
    <header className="pageHeader"><div><h1>DCP3</h1><p>Data calon penerima paket perdana</p></div>{selectedSchedule && <div className={styles.scheduleContext}><strong>{selectedSchedule.regency?.document_code}</strong><span>{selectedSchedule.name}</span></div>}</header>
    <ol className={styles.steps} aria-label="Tahapan import DCP3">
      {steps.map((label, index) => <li className={step === index + 1 ? styles.currentStep : step > index + 1 ? styles.completedStep : ''} key={label}><span>{index + 1}</span>{index + 1} {label}</li>)}
    </ol>

    {!canImport ? <section className={styles.readOnly}><FileSpreadsheet aria-hidden="true" /><div><h2>Daftar DCP3</h2><p>Akses import diperlukan untuk menambahkan data DCP3.</p></div></section> : <>
      {step === 1 && <section className={styles.workspace}>
        <div className={styles.sectionHeading}><div><h2>Pilih jadwal aktif</h2><p>Data akan tersimpan di jadwal dan kabupaten yang dipilih.</p></div></div>
        <label className={styles.wideField}><span>Jadwal distribusi</span><select value={scheduleID} onChange={(event) => setScheduleID(event.target.value)}><option value="">Pilih jadwal</option>{schedules.data?.data.filter((item) => item.status === 'active').map((schedule) => <option value={schedule.id} key={schedule.id}>{schedule.regency?.name} - {schedule.name}</option>)}</select></label>
        {selectedSchedule && <div className={styles.scheduleDetail}><span><small>Program</small>{selectedSchedule.program?.name}</span><span><small>Kabupaten</small>{selectedSchedule.regency?.name}</span><span><small>Periode</small>{formatDate(selectedSchedule.start_date)} - {formatDate(selectedSchedule.end_date)}</span></div>}
        <div className={styles.actions}><button className="primaryButton" disabled={!scheduleID} onClick={() => setStep(2)}>Lanjut ke upload <ArrowRight /></button></div>
      </section>}

      {step === 2 && <section className={styles.workspace}>
        <div className={styles.sectionHeading}><div><h2>Upload workbook DCP3</h2><p>Gunakan file .xlsx dengan maksimal 5.000 baris.</p></div></div>
        <label className={styles.dropzone}><Upload aria-hidden="true" /><strong>{file?.name ?? 'Letakkan file Excel di sini'}</strong><span>{file ? `${(file.size / 1024).toFixed(1)} KB` : 'atau pilih dari perangkat'}</span><input aria-label="Pilih file DCP3" accept=".xlsx" type="file" onChange={(event) => setFile(event.target.files?.[0] ?? null)} /></label>
        {upload.error && <p className={styles.error} role="alert">{upload.error instanceof ApiError ? upload.error.message : 'File DCP3 tidak dapat dibaca.'}</p>}
        <div className={styles.actions}><button className="secondaryButton" onClick={() => setStep(1)}><ArrowLeft /> Kembali</button><button className="primaryButton" disabled={!file || upload.isPending} onClick={() => upload.mutate()}>{upload.isPending ? 'Membaca file...' : 'Unggah dan baca file'}</button></div>
      </section>}

      {step === 3 && preview && <section className={styles.workspace}>
        <div className={styles.sectionHeading}><div><h2>Cocokkan kolom Excel</h2><p>Pilih kolom sumber untuk setiap data penerima. Tanda * wajib dipetakan.</p></div><span className={styles.fileMeta}>{preview.sheet_name} / {preview.rows.length} baris</span></div>
        <ColumnMappingStep headers={preview.headers} mapping={mapping} programType={preview.program_type} onChange={setMapping} />
        <div className={styles.actions}><button className="secondaryButton" onClick={() => setStep(2)}><ArrowLeft /> Kembali</button><button className="primaryButton" disabled={!mapping.source_sequence || !mapping.full_name} onClick={() => setStep(4)}>Periksa data <ArrowRight /></button></div>
      </section>}

      {step === 4 && preview && !result && <section className={styles.workspace}>
        <div className={styles.sectionHeading}><div><h2>Periksa hasil pembacaan</h2><p>Status ini adalah pemeriksaan awal. Server akan memvalidasi ulang saat import.</p></div></div>
        <div className={styles.summary}><span className={styles.valid}><strong>{summary.valid}</strong> Valid</span><span className={styles.warning}><strong>{summary.warning}</strong> Peringatan</span><span className={styles.conflict}><strong>{summary.conflict}</strong> Konflik</span></div>
        <ImportPreviewTable rows={preview.rows} mapping={mapping} />
        {commit.error && <p className={styles.error} role="alert">{commit.error instanceof ApiError ? commit.error.message : 'Import tidak dapat diselesaikan.'}</p>}
        <div className={styles.actions}><button className="secondaryButton" onClick={() => setStep(3)}><ArrowLeft /> Kembali</button><button className="primaryButton" disabled={commit.isPending} onClick={() => commit.mutate()}>{commit.isPending ? 'Mengimport...' : `Import ${preview.rows.length} data`}</button></div>
      </section>}

      {result && <section className={styles.success} aria-live="polite"><CheckCircle2 aria-hidden="true" /><div><h2>{result.total_rows} data selesai diproses</h2><p>Data penerima dan nomor distribusi sudah dibuat untuk jadwal ini.</p><div><span>{result.valid_rows} valid</span><span>{result.warning_rows} peringatan</span><span>{result.invalid_rows} konflik</span></div></div><button className="secondaryButton" onClick={() => { setStep(1); setScheduleID(''); setFile(null); setPreview(null); setMapping(emptyMapping); setResult(null); }}>Import file lain</button></section>}
    </>}
  </div>;
}
