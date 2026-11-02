-- +goose Up
-- Kantor Dinas Pertanian tiap kabupaten, ditampilkan sebagai lapisan di Map
-- Distribusi. Titik awal dari peta Google "Lokasi Kantor Dinas Pertanian"
-- (Oktober 2026), dicocokkan lewat document_code. Kosong = belum diketahui.
ALTER TABLE regencies
    ADD COLUMN agriculture_office_latitude numeric(9,6) CHECK (agriculture_office_latitude BETWEEN -90 AND 90),
    ADD COLUMN agriculture_office_longitude numeric(9,6) CHECK (agriculture_office_longitude BETWEEN -180 AND 180),
    ADD CONSTRAINT regencies_agriculture_office_pair CHECK ((agriculture_office_latitude IS NULL) = (agriculture_office_longitude IS NULL));

UPDATE regencies r SET agriculture_office_latitude = v.lat, agriculture_office_longitude = v.lon
FROM (VALUES
    ('LNG', 4.47297, 97.950986),
    ('SBG', 3.49154, 99.147989),
    ('DLS', 3.545932, 98.871492),
    ('BRN', 5.204991, 96.665993),
    ('PDP', -0.469585, 100.414063),
    ('SJJ', -0.663341, 100.923967),
    ('TDT', -0.473856, 100.624503),
    ('SIK', 0.814358, 102.020955),
    ('RHL', 2.104677, 100.822876),
    ('KPR', 0.317413, 101.017324),
    ('IRH', -0.375775, 102.441517),
    ('IRL', -0.324265, 103.158242),
    ('CLP', -7.711401, 109.013863),
    ('BMS', -7.418749, 109.248382),
    ('PBG', -7.38274, 109.359892),
    ('KBM', -7.674161, 109.647824),
    ('BBS', -6.874711, 109.053079),
    ('TGL', -6.976314, 109.138867),
    ('PML', -6.891312, 109.379815),
    ('WNG', -7.795777, 110.914345),
    ('GRB', -7.10537, 110.912307),
    ('BLR', -6.939306, 111.426565),
    ('JPR', -6.593862, 110.667192),
    ('OKI', -3.316158, 104.83268),
    ('OKU', -4.113975, 104.176775),
    ('OKT', -4.328496, 104.357629),
    ('JMB', -1.621238, 103.554592),
    ('MJB', -1.450653, 103.522738),
    ('TJT', -1.275074, 103.733507),
    ('KRC', -1.947042, 101.336174),
    ('MRG', -2.055337, 102.272268),
    ('LGS', -5.712597, 105.586465),
    ('LTM', -5.046102, 105.523296),
    ('LGT', -4.988315, 105.19599),
    ('BKA', -1.808636, 106.108464),
    ('BKB', -2.019391, 105.160609),
    ('JBR', -8.189865, 113.648994),
    ('LMJ', -8.07401, 113.235053),
    ('BKL', -7.047423, 112.735052),
    ('NGJ', -7.624975, 111.897247),
    ('PNG', -7.86353, 111.494177),
    ('PCT', -8.199708, 111.084788),
    ('TBN', -6.901976, 112.043608),
    ('BJN', -7.178019, 111.907068),
    ('MJK', -7.487434, 112.426903),
    ('MLG', -8.136176, 112.566943)
) AS v(code, lat, lon)
WHERE r.document_code = v.code;

-- +goose Down
ALTER TABLE regencies
    DROP CONSTRAINT IF EXISTS regencies_agriculture_office_pair,
    DROP COLUMN IF EXISTS agriculture_office_longitude,
    DROP COLUMN IF EXISTS agriculture_office_latitude;
