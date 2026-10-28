-- +goose Up
-- Titik serah dicatat saat distribusi (POS Dokumen), bukan dari cakupan
-- wilayah: setiap nomor bagi menyimpan lokasinya. Daftar lokasi dipakai
-- bersama satu kabupaten (semua jadwal dan petugasnya) dan tumbuh dari
-- lapangan; nama dibandingkan lewat name_key agar varian ketik tidak dobel.
ALTER TABLE bast_handover_locations
    ADD COLUMN regency_id uuid REFERENCES regencies(id) ON DELETE CASCADE,
    ADD COLUMN name_key text,
    ADD COLUMN created_by uuid REFERENCES users(id) ON DELETE SET NULL;
UPDATE bast_handover_locations l SET regency_id = ps.regency_id FROM program_schedules ps WHERE ps.id = l.schedule_id;
UPDATE bast_handover_locations SET name = btrim(regexp_replace(name, '\s+', ' ', 'g')),
    name_key = btrim(regexp_replace(upper(name), '[^A-Z0-9]+', ' ', 'g'));

ALTER TABLE distribution_slots ADD COLUMN handover_location_id uuid REFERENCES bast_handover_locations(id) ON DELETE SET NULL;
CREATE INDEX distribution_slots_handover_location_idx ON distribution_slots (handover_location_id);

-- Pindah manual menjadi lokasi nomor bagi.
UPDATE distribution_slots ds SET handover_location_id = o.location_id
FROM bast_handover_overrides o WHERE o.distribution_slot_id = ds.id;

-- Satu kabupaten satu daftar: lokasi bernama sama digabung ke yang pertama.
CREATE TEMP TABLE handover_keep ON COMMIT DROP AS
SELECT id, first_value(id) OVER (PARTITION BY regency_id, name_key ORDER BY sort_order, created_at, id) AS keep_id
FROM bast_handover_locations;
UPDATE distribution_slots ds SET handover_location_id = k.keep_id
FROM handover_keep k WHERE ds.handover_location_id = k.id AND k.id <> k.keep_id;
UPDATE bast_aggregate_documents d SET status = 'superseded', updated_at = now()
FROM handover_keep k WHERE d.document_type = 'closing_titik_serah:' || k.id::text AND k.id <> k.keep_id AND d.status IN ('active', 'stale');
DELETE FROM bast_handover_locations l USING handover_keep k WHERE l.id = k.id AND k.id <> k.keep_id;

-- Kabupaten dengan satu titik serah: penerima selesai tanpa lokasi masuk ke sana.
UPDATE distribution_slots ds SET handover_location_id = l.id
FROM program_schedules ps, bast_handover_locations l
WHERE ps.id = ds.schedule_id AND l.regency_id = ps.regency_id AND ds.handover_location_id IS NULL
  AND ds.status = 'completed'
  AND (SELECT count(*) FROM bast_handover_locations x WHERE x.regency_id = ps.regency_id) = 1;

DROP TABLE bast_handover_overrides;
DROP INDEX bast_handover_locations_name_uq;
DROP INDEX bast_handover_locations_schedule_idx;
ALTER TABLE bast_handover_locations DROP COLUMN schedule_id, DROP COLUMN areas, DROP COLUMN sort_order;
ALTER TABLE bast_handover_locations ALTER COLUMN regency_id SET NOT NULL, ALTER COLUMN name_key SET NOT NULL;
ALTER TABLE bast_handover_locations ADD CONSTRAINT bast_handover_locations_name_key_check CHECK (name_key <> '');
CREATE UNIQUE INDEX bast_handover_locations_regency_key_uq ON bast_handover_locations (regency_id, name_key);

-- +goose Down
DROP INDEX IF EXISTS bast_handover_locations_regency_key_uq;
ALTER TABLE bast_handover_locations DROP CONSTRAINT IF EXISTS bast_handover_locations_name_key_check;
ALTER TABLE bast_handover_locations
    ADD COLUMN schedule_id uuid REFERENCES program_schedules(id) ON DELETE CASCADE,
    ADD COLUMN areas jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN sort_order integer NOT NULL DEFAULT 0;
-- Lokasi kembali ke satu jadwal: jadwal pertama yang memakainya, atau jadwal
-- pertama kabupatennya.
UPDATE bast_handover_locations l SET schedule_id = COALESCE(
    (SELECT ds.schedule_id FROM distribution_slots ds WHERE ds.handover_location_id = l.id ORDER BY ds.schedule_id LIMIT 1),
    (SELECT ps.id FROM program_schedules ps WHERE ps.regency_id = l.regency_id ORDER BY ps.created_at, ps.id LIMIT 1));
DELETE FROM bast_handover_locations WHERE schedule_id IS NULL;
ALTER TABLE bast_handover_locations ALTER COLUMN schedule_id SET NOT NULL;
CREATE TABLE bast_handover_overrides (
    distribution_slot_id uuid PRIMARY KEY REFERENCES distribution_slots(id) ON DELETE CASCADE,
    location_id uuid NOT NULL REFERENCES bast_handover_locations(id) ON DELETE CASCADE,
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX bast_handover_overrides_location_idx ON bast_handover_overrides (location_id);
INSERT INTO bast_handover_overrides (distribution_slot_id, location_id)
SELECT ds.id, ds.handover_location_id FROM distribution_slots ds
JOIN bast_handover_locations l ON l.id = ds.handover_location_id AND l.schedule_id = ds.schedule_id;
DROP INDEX IF EXISTS distribution_slots_handover_location_idx;
ALTER TABLE distribution_slots DROP COLUMN handover_location_id;
ALTER TABLE bast_handover_locations DROP COLUMN regency_id, DROP COLUMN name_key, DROP COLUMN created_by;
CREATE UNIQUE INDEX bast_handover_locations_name_uq ON bast_handover_locations (schedule_id, lower(btrim(name)));
CREATE INDEX bast_handover_locations_schedule_idx ON bast_handover_locations (schedule_id, sort_order);
