import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { FormEvent, useEffect, useState } from 'react';
import { FormField } from '../../components/FormField';
import { apiRequest } from '../../lib/api';

type Setting = { key: string; value: string; type: string; description: string };
const labels: Record<string, string> = { application_name: 'Nama aplikasi', organization_name: 'Nama organisasi', timezone: 'Zona waktu', date_format: 'Format tanggal', locale: 'Bahasa' };
const options: Record<string, string[]> = { timezone: ['Asia/Jakarta', 'Asia/Makassar', 'Asia/Jayapura'], date_format: ['02/01/2006', '02 January 2006'], locale: ['id-ID'] };

export function SettingsPage() {
  const client = useQueryClient();
  const query = useQuery({ queryKey: ['settings'], queryFn: () => apiRequest<{ data: Setting[] }>('/api/v1/system/settings') });
  const [values, setValues] = useState<Record<string, string>>({});
  useEffect(() => { if (query.data) setValues(Object.fromEntries(query.data.data.map((item) => [item.key, item.value]))); }, [query.data]);
  const mutation = useMutation({ mutationFn: () => apiRequest('/api/v1/system/settings', { method: 'PATCH', body: JSON.stringify({ values }) }), onSuccess: () => client.invalidateQueries({ queryKey: ['settings'] }) });
  return <div className="page"><header className="pageHeader"><div><h1>Pengaturan sistem</h1><p>Nilai dasar yang digunakan secara konsisten di seluruh program.</p></div></header>
    {query.isError ? <div className="errorState">Pengaturan belum dapat dimuat.</div> : <form onSubmit={(event: FormEvent) => { event.preventDefault(); mutation.mutate(); }} className="dialogBody" style={{ maxWidth: 760, padding: 0 }}>
      {query.data?.data.map((setting) => options[setting.key] ? <label className="formField" key={setting.key}><span>{labels[setting.key]}</span><select className="selectField" aria-label={labels[setting.key]} value={values[setting.key] ?? ''} onChange={(e) => setValues({ ...values, [setting.key]: e.target.value })}>{options[setting.key].map((item) => <option key={item}>{item}</option>)}</select><small>{setting.description}</small></label> : <FormField key={setting.key} label={labels[setting.key] ?? setting.key} name={setting.key} maxLength={setting.key === 'organization_name' ? 160 : 120} value={values[setting.key] ?? ''} onChange={(e) => setValues({ ...values, [setting.key]: e.target.value })} hint={setting.description} />)}
      {mutation.isError && <p className="formNotice">Pengaturan belum dapat disimpan.</p>}{mutation.isSuccess && <span className="successNotice">Pengaturan berhasil disimpan.</span>}
      <div className="fullField"><button className="primaryButton" disabled={mutation.isPending}>Simpan pengaturan</button></div>
    </form>}
  </div>;
}
