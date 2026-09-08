import { useMutation, useQuery } from '@tanstack/react-query';
import { AlertTriangle, ArrowLeft, ArrowRight, CheckCircle2, Upload } from 'lucide-react';
import { useMemo, useState } from 'react';
import { DataState } from '../../components/DataState';
import { DataTable } from '../../components/DataTable';
import { PageHeader } from '../../components/PageHeader';
import { Alert, AlertDescription } from '../../components/ui/alert';
import { Badge } from '../../components/ui/badge';
import { Button } from '../../components/ui/button';
import { Card, CardContent, CardHeader } from '../../components/ui/card';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../components/ui/select';
import { Separator } from '../../components/ui/separator';
import { ApiError, apiRequest } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { formatDate } from '../programs/types';
import { ColumnMappingStep } from './ColumnMappingStep';
import { ImportPreviewTable, rowStatus } from './ImportPreviewTable';
import { ImportStepper } from './ImportStepper';
import { DataResponse, DCP3Mapping, DCP3Preview, emptyMapping, ImportResult, ScheduleResponse } from './types';
import styles from './DCP3Import.module.css';

const importSteps = [
  { label: 'Jadwal', description: 'Pilih jadwal aktif' },
  { label: 'Workbook', description: 'Unggah file DCP3' },
  { label: 'Pemetaan', description: 'Cocokkan kolom' },
  { label: 'Tinjau', description: 'Periksa dan import' },
];

export function DCP3ImportPage() {
  const canImport = useCan('dcp3.import');
  const [step, setStep] = useState(1);
  const [scheduleID, setScheduleID] = useState('');
  const [file, setFile] = useState<File | null>(null);
  const [headerRow, setHeaderRow] = useState(1);
  const [rawRows, setRawRows] = useState<string[][] | null>(null);
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
      body.set('header_row', String(headerRow));
      return apiRequest<DataResponse<DCP3Preview>>('/api/v1/dcp3/previews', { method: 'POST', body });
    },
    onSuccess: ({ data }) => { setPreview(data); setMapping(emptyMapping); setRawRows(null); setStep(3); },
  });
  const rawPreview = useMutation({
    mutationFn: async () => {
      if (!file) throw new Error('Pilih file DCP3 terlebih dahulu');
      const body = new FormData();
      body.set('file', file);
      return apiRequest<DataResponse<string[][]>>('/api/v1/dcp3/raw-preview', { method: 'POST', body });
    },
    onSuccess: ({ data }) => setRawRows(data),
  });
  const headersInvalid = upload.error instanceof ApiError && upload.error.code === 'dcp3_headers_invalid';
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

  const scheduleContext = selectedSchedule ? <div className={styles.scheduleContext}>
    <strong>{selectedSchedule.regency?.document_code}</strong>
    <span><b>{selectedSchedule.name}</b><small>{selectedSchedule.regency?.name}</small></span>
  </div> : undefined;

  return <div className={`page ${styles.page}`}>
    <PageHeader title="DCP3" description="Data calon penerima paket perdana" context={scheduleContext} />
    <ImportStepper currentStep={step} steps={importSteps} />

    {!canImport ? <DataState kind="empty" title="Daftar DCP3" description="Akses import diperlukan untuk menambahkan data DCP3." /> : <>
      {step === 1 && <Card className={styles.workspace}>
        <CardHeader className={styles.sectionHeading}>
          <div><h2>Pilih jadwal aktif</h2><p>Data akan tersimpan di jadwal dan kabupaten yang dipilih.</p></div>
        </CardHeader>
        <CardContent className={styles.workspaceContent}>
          <div className={styles.wideField}>
            <label id="dcp3-schedule-label">Jadwal distribusi</label>
            <Select value={scheduleID} onValueChange={(value) => setScheduleID(value ?? '')} disabled={schedules.isPending || schedules.isError}>
              <SelectTrigger className="w-full" aria-labelledby="dcp3-schedule-label"><SelectValue placeholder={schedules.isPending ? 'Memuat jadwal...' : 'Pilih jadwal'} /></SelectTrigger>
              <SelectContent>{schedules.data?.data.filter((item) => item.status === 'active').map((schedule) => <SelectItem value={schedule.id} key={schedule.id}>{schedule.regency?.name} - {schedule.name}</SelectItem>)}</SelectContent>
            </Select>
          </div>
          {schedules.isError ? <Alert variant="destructive"><AlertDescription>Jadwal aktif belum dapat dimuat. Muat ulang halaman untuk mencoba kembali.</AlertDescription></Alert> : null}
          {selectedSchedule ? <>
            <Separator />
            <div className={styles.scheduleDetail}>
              <span><small>Program</small>{selectedSchedule.program?.name}</span>
              <span><small>Kabupaten</small>{selectedSchedule.regency?.name}</span>
              <span><small>Periode</small>{formatDate(selectedSchedule.start_date)} - {formatDate(selectedSchedule.end_date)}</span>
            </div>
          </> : null}
          <div className={styles.actions}><Button type="button" disabled={!scheduleID} onClick={() => setStep(2)}>Lanjut ke upload <ArrowRight /></Button></div>
        </CardContent>
      </Card>}

      {step === 2 && <Card className={styles.workspace}>
        <CardHeader className={styles.sectionHeading}>
          <div><h2>Upload workbook DCP3</h2><p>Gunakan file .xlsx dengan maksimal 5.000 baris.</p></div>
        </CardHeader>
        <CardContent className={styles.workspaceContent}>
          <label className={styles.dropzone}>
            <Upload aria-hidden="true" />
            <strong>{file?.name ?? 'Letakkan file Excel di sini'}</strong>
            <span>{file ? `${(file.size / 1024).toFixed(1)} KB` : 'atau pilih dari perangkat'}</span>
            <input aria-label="Pilih file DCP3" accept=".xlsx" type="file" onChange={(event) => { setFile(event.target.files?.[0] ?? null); setHeaderRow(1); setRawRows(null); upload.reset(); rawPreview.reset(); }} />
          </label>
          {upload.error ? <Alert variant="destructive"><AlertTriangle /><AlertDescription>{upload.error instanceof ApiError ? upload.error.message : 'File DCP3 tidak dapat dibaca.'}</AlertDescription></Alert> : null}
          {headersInvalid && <div className={styles.headerRowPicker}>
            {!rawRows ? <Button type="button" variant="outline" disabled={rawPreview.isPending} onClick={() => rawPreview.mutate()}>{rawPreview.isPending ? 'Membaca baris...' : 'Lihat & pilih baris header'}</Button> : <>
              <p>File ini punya baris tambahan sebelum header. Pilih baris mana yang sebenarnya header:</p>
              <div className={styles.rawTable}><DataTable label="Baris awal workbook DCP3">
                <tbody>{rawRows.map((row, index) => <tr key={index}>
                  <td><label><input type="radio" name="header_row_pick" checked={headerRow === index + 1} onChange={() => setHeaderRow(index + 1)} /> Baris {index + 1}</label></td>
                  <td>{(row ?? []).filter(Boolean).join(' | ') || <em>(baris kosong)</em>}</td>
                </tr>)}</tbody>
              </DataTable></div>
              <div className={styles.actions}><Button type="button" onClick={() => upload.mutate()}>Coba lagi dengan baris ini</Button></div>
            </>}
            {rawPreview.isError ? <Alert variant="destructive"><AlertDescription>Baris file tidak dapat dibaca.</AlertDescription></Alert> : null}
          </div>}
          <div className={styles.actions}><Button type="button" variant="outline" onClick={() => setStep(1)}><ArrowLeft /> Kembali</Button><Button type="button" disabled={!file || upload.isPending} onClick={() => upload.mutate()}>{upload.isPending ? 'Membaca file...' : 'Unggah dan baca file'}</Button></div>
        </CardContent>
      </Card>}

      {step === 3 && preview && <Card className={styles.workspace}>
        <CardHeader className={styles.sectionHeading}>
          <div><h2>Cocokkan kolom Excel</h2><p>Pilih kolom sumber untuk setiap data penerima. Tanda * wajib dipetakan.</p></div>
          <Badge variant="outline" className={styles.fileMeta}>{preview.sheet_name} / {preview.rows.length} baris</Badge>
        </CardHeader>
        <CardContent className={styles.workspaceContent}>
          <ColumnMappingStep headers={preview.headers} mapping={mapping} programType={preview.program_type} onChange={setMapping} />
          <div className={styles.actions}><Button type="button" variant="outline" onClick={() => setStep(2)}><ArrowLeft /> Kembali</Button><Button type="button" disabled={!mapping.source_sequence || !mapping.full_name} onClick={() => setStep(4)}>Periksa data <ArrowRight /></Button></div>
        </CardContent>
      </Card>}

      {step === 4 && preview && !result && <Card className={styles.workspace}>
        <CardHeader className={styles.sectionHeading}>
          <div><h2>Periksa hasil pembacaan</h2><p>Status ini adalah pemeriksaan awal. Server akan memvalidasi ulang saat import.</p></div>
        </CardHeader>
        <CardContent className={styles.workspaceContent}>
          <div className={styles.summary} aria-label="Ringkasan pratinjau">
            <Badge variant="outline" className={styles.valid}><strong>{summary.valid}</strong> Valid</Badge>
            <Badge variant="outline" className={styles.warning}><strong>{summary.warning}</strong> Peringatan</Badge>
            <Badge variant="outline" className={styles.conflict}><strong>{summary.conflict}</strong> Konflik</Badge>
          </div>
          <ImportPreviewTable rows={preview.rows} mapping={mapping} />
          {commit.error ? <Alert variant="destructive"><AlertDescription>{commit.error instanceof ApiError ? commit.error.message : 'Import tidak dapat diselesaikan.'}</AlertDescription></Alert> : null}
          <div className={styles.actions}><Button type="button" variant="outline" onClick={() => setStep(3)}><ArrowLeft /> Kembali</Button><Button type="button" disabled={commit.isPending} onClick={() => commit.mutate()}>{commit.isPending ? 'Mengimport...' : `Import ${preview.rows.length} data`}</Button></div>
        </CardContent>
      </Card>}

      {result && <Card className={styles.success} aria-live="polite">
        <CheckCircle2 aria-hidden="true" />
        <div><h2>{result.total_rows} data selesai diproses</h2><p>Data penerima dan nomor distribusi sudah dibuat untuk jadwal ini.</p><div><Badge variant="outline">{result.valid_rows} valid</Badge><Badge variant="outline">{result.warning_rows} peringatan</Badge><Badge variant="outline">{result.invalid_rows} konflik</Badge></div></div>
        <Button type="button" variant="outline" onClick={() => { setStep(1); setScheduleID(''); setFile(null); setHeaderRow(1); setRawRows(null); setPreview(null); setMapping(emptyMapping); setResult(null); }}>Import file lain</Button>
      </Card>}
    </>}
  </div>;
}
