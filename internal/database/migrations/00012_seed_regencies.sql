-- +goose Up
INSERT INTO regencies (province_name, name, document_code) VALUES
    ('Aceh', 'Kota Langsa', 'LNG'),
    ('Aceh', 'Kab. Bireuen', 'BRN'),
    ('Sumatera Utara', 'Kab. Serdang Bedagai', 'SBG'),
    ('Sumatera Utara', 'Kab. Deli Serdang', 'DLS'),
    ('Sumatera Barat', 'Kab. Tanah Datar', 'TDT'),
    ('Sumatera Barat', 'Kab. Sijunjung', 'SJJ'),
    ('Sumatera Barat', 'Kota Padang Panjang', 'PDP'),
    ('Riau', 'Kab. Indragiri Hulu', 'IRH'),
    ('Riau', 'Kab. Indragiri Hilir', 'IRL'),
    ('Riau', 'Kab. Siak', 'SIK'),
    ('Riau', 'Kab. Rokan Hilir', 'RHL'),
    ('Riau', 'Kab. Kampar', 'KPR'),
    ('Jawa Tengah', 'Kab. Cilacap', 'CLP'),
    ('Jawa Tengah', 'Kab. Banyumas', 'BMS'),
    ('Jawa Tengah', 'Kab. Grobogan', 'GRB'),
    ('Jawa Tengah', 'Kab. Blora', 'BLR'),
    ('Jawa Tengah', 'Kab. Wonogiri', 'WNG'),
    ('Jawa Tengah', 'Kab. Brebes', 'BBS'),
    ('Jawa Tengah', 'Kab. Tegal', 'TGL'),
    ('Jawa Tengah', 'Kab. Jepara', 'JPR'),
    ('Jawa Tengah', 'Kab. Kebumen', 'KBM'),
    ('Jawa Tengah', 'Kab. Purbalingga', 'PBG'),
    ('Jawa Tengah', 'Kab. Pemalang', 'PML'),
    ('Sumatera Selatan', 'Kab. Ogan Komering Ilir', 'OKI'),
    ('Sumatera Selatan', 'Kab. Ogan Komering Ulu', 'OKU'),
    ('Sumatera Selatan', 'Kab. Ogan Komering Ulu Timur', 'OKT'),
    ('Jambi', 'Kab. Tanjung Jabung Timur', 'TJT'),
    ('Jambi', 'Kab. Muaro Jambi', 'MJB'),
    ('Jambi', 'Kota Jambi', 'JMB'),
    ('Jambi', 'Kab. Kerinci', 'KRC'),
    ('Jambi', 'Kab. Merangin', 'MRG'),
    ('Lampung', 'Kab. Lampung Selatan', 'LGS'),
    ('Lampung', 'Kab. Lampung Tengah', 'LGT'),
    ('Lampung', 'Kab. Lampung Timur', 'LTM'),
    ('Bangka Belitung', 'Kab. Bangka', 'BKA'),
    ('Bangka Belitung', 'Kab. Bangka Barat', 'BKB'),
    ('Jawa Timur', 'Kab. Jember', 'JBR'),
    ('Jawa Timur', 'Kab. Lumajang', 'LMJ'),
    ('Jawa Timur', 'Kab. Pacitan', 'PCT'),
    ('Jawa Timur', 'Kab. Ponorogo', 'PNG'),
    ('Jawa Timur', 'Kab. Bangkalan', 'BKL'),
    ('Jawa Timur', 'Kab. Nganjuk', 'NGJ'),
    ('Jawa Timur', 'Kab. Mojokerto', 'MJK'),
    ('Jawa Timur', 'Kab. Tuban', 'TBN'),
    ('Jawa Timur', 'Kab. Bojonegoro', 'BJN'),
    ('Jawa Timur', 'Kab. Malang', 'MLG')
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
