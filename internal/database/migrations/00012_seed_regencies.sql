-- +goose Up
INSERT INTO regencies (province_name, name, document_code) VALUES
    ('ACEH', 'KOTA LANGSA', 'LNG'),
    ('ACEH', 'KAB. BIREUEN', 'BRN'),
    ('SUMATERA UTARA', 'KAB. SERDANG BEDAGAI', 'SBG'),
    ('SUMATERA UTARA', 'KAB. DELI SERDANG', 'DLS'),
    ('SUMATERA BARAT', 'KAB. TANAH DATAR', 'TDT'),
    ('SUMATERA BARAT', 'KAB. SIJUNJUNG', 'SJJ'),
    ('SUMATERA BARAT', 'KOTA PADANG PANJANG', 'PDP'),
    ('RIAU', 'KAB. INDRAGIRI HULU', 'IRH'),
    ('RIAU', 'KAB. INDRAGIRI HILIR', 'IRL'),
    ('RIAU', 'KAB. SIAK', 'SIK'),
    ('RIAU', 'KAB. ROKAN HILIR', 'RHL'),
    ('RIAU', 'KAB. KAMPAR', 'KPR'),
    ('JAWA TENGAH', 'KAB. CILACAP', 'CLP'),
    ('JAWA TENGAH', 'KAB. BANYUMAS', 'BMS'),
    ('JAWA TENGAH', 'KAB. GROBOGAN', 'GRB'),
    ('JAWA TENGAH', 'KAB. BLORA', 'BLR'),
    ('JAWA TENGAH', 'KAB. WONOGIRI', 'WNG'),
    ('JAWA TENGAH', 'KAB. BREBES', 'BBS'),
    ('JAWA TENGAH', 'KAB. TEGAL', 'TGL'),
    ('JAWA TENGAH', 'KAB. JEPARA', 'JPR'),
    ('JAWA TENGAH', 'KAB. KEBUMEN', 'KBM'),
    ('JAWA TENGAH', 'KAB. PURBALINGGA', 'PBG'),
    ('JAWA TENGAH', 'KAB. PEMALANG', 'PML'),
    ('SUMATERA SELATAN', 'KAB. OGAN KOMERING ILIR', 'OKI'),
    ('SUMATERA SELATAN', 'KAB. OGAN KOMERING ULU', 'OKU'),
    ('SUMATERA SELATAN', 'KAB. OGAN KOMERING ULU TIMUR', 'OKT'),
    ('JAMBI', 'KAB. TANJUNG JABUNG TIMUR', 'TJT'),
    ('JAMBI', 'KAB. MUARO JAMBI', 'MJB'),
    ('JAMBI', 'KOTA JAMBI', 'JMB'),
    ('JAMBI', 'KAB. KERINCI', 'KRC'),
    ('JAMBI', 'KAB. MERANGIN', 'MRG'),
    ('LAMPUNG', 'KAB. LAMPUNG SELATAN', 'LGS'),
    ('LAMPUNG', 'KAB. LAMPUNG TENGAH', 'LGT'),
    ('LAMPUNG', 'KAB. LAMPUNG TIMUR', 'LTM'),
    ('BANGKA BELITUNG', 'KAB. BANGKA', 'BKA'),
    ('BANGKA BELITUNG', 'KAB. BANGKA BARAT', 'BKB'),
    ('JAWA TIMUR', 'KAB. JEMBER', 'JBR'),
    ('JAWA TIMUR', 'KAB. LUMAJANG', 'LMJ'),
    ('JAWA TIMUR', 'KAB. PACITAN', 'PCT'),
    ('JAWA TIMUR', 'KAB. PONOROGO', 'PNG'),
    ('JAWA TIMUR', 'KAB. BANGKALAN', 'BKL'),
    ('JAWA TIMUR', 'KAB. NGANJUK', 'NGJ'),
    ('JAWA TIMUR', 'KAB. MOJOKERTO', 'MJK'),
    ('JAWA TIMUR', 'KAB. TUBAN', 'TBN'),
    ('JAWA TIMUR', 'KAB. BOJONEGORO', 'BJN'),
    ('JAWA TIMUR', 'KAB. MALANG', 'MLG')
ON CONFLICT (document_code) DO UPDATE SET province_name = EXCLUDED.province_name, name = EXCLUDED.name;

-- +goose Down
DELETE FROM regencies WHERE document_code IN (
    'LNG', 'BRN', 'SBG', 'DLS', 'TDT', 'SJJ', 'PDP', 'IRH', 'IRL', 'SIK', 'RHL', 'KPR',
    'CLP', 'BMS', 'GRB', 'BLR', 'WNG', 'BBS', 'TGL', 'JPR', 'KBM', 'PBG', 'PML',
    'OKI', 'OKU', 'OKT', 'TJT', 'MJB', 'JMB', 'KRC', 'MRG', 'LGS', 'LGT', 'LTM',
    'BKA', 'BKB', 'JBR', 'LMJ', 'PCT', 'PNG', 'BKL', 'NGJ', 'MJK', 'TBN', 'BJN', 'MLG'
);
-- Note: this cascades to any program_schedules/role_regencies created against
-- these regencies in the meantime (see their ON DELETE CASCADE FKs) — same
-- caveat as this codebase's other seed-data Down migrations.
