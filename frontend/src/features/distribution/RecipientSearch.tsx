import { AlertCircle, Check, Search, X } from 'lucide-react';
import type { SearchResult } from './types';
import styles from './Distribution.module.css';

const eligibilityLabel: Record<string, string> = {
  eligible: 'Layak menerima', incomplete: 'Data belum lengkap', previously_received: 'Pernah menerima',
  approval_required: 'Perlu persetujuan', identity_conflict: 'Konflik identitas',
};

export function RecipientSearch({ query, disabled, loading, results, onQueryChange, onSelect }: {
  query: string; disabled: boolean; loading: boolean; results: SearchResult[];
  onQueryChange: (value: string) => void; onSelect: (allocationID: string) => void;
}) {
  return <div className={styles.searchArea}>
    <label className={styles.searchField}><span className={styles.srOnly}>Cari penerima</span><Search aria-hidden="true" /><input
      role="combobox" aria-label="Cari penerima" aria-controls="recipient-results" aria-expanded={results.length > 0}
      autoComplete="off" disabled={disabled} placeholder={disabled ? 'Pilih jadwal terlebih dahulu' : 'Cari nomor pembagian, nama, NIK, atau nomor kartu'}
      value={query} onChange={(event) => onQueryChange(event.target.value)}
    /></label>
    {loading && <p className={styles.searchNote}>Mencari data penerima...</p>}
    {!loading && query.length === 1 && !/^\d$/.test(query) && <p className={styles.searchNote}>Ketik minimal dua karakter untuk pencarian nama.</p>}
    {results.length > 0 && <div className={styles.results} id="recipient-results" role="listbox" aria-label="Hasil pencarian penerima">
      {results.map((result) => <button className={styles.resultRow} role="option" aria-selected="false" key={result.allocation_id} onClick={() => onSelect(result.allocation_id)}>
        <span className={styles.resultNumber}>No. {result.distribution_number}</span>
        <span className={styles.resultIdentity}><strong>{result.full_name}</strong><small>{result.masked_nik || 'NIK belum tersedia'} / {result.location}</small></span>
        <span className={`${styles.eligibility} ${styles[result.eligibility]}`}><AlertCircle aria-hidden="true" />{eligibilityLabel[result.eligibility] ?? result.eligibility}</span>
        <span className={styles.slotDots}>{result.documentation.map((slot) => {
          const complete = slot.status === 'complete';
          const Icon = complete ? Check : X;
          return <span className={complete ? styles.slotComplete : styles.slotMissing} aria-label={`${slot.label} ${complete ? 'lengkap' : 'belum lengkap'}`} title={slot.label} key={slot.code}><Icon aria-hidden="true" /></span>;
        })}</span>
      </button>)}
    </div>}
  </div>;
}
