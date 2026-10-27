-- +goose Up
-- BA Serah Terima (Closing Titik Serah) per lokasi titik serah. Satu jadwal
-- dapat punya beberapa titik serah; penerima masuk ke lokasi sesuai
-- kecamatan/desanya (areas) atau dipindah manual (overrides). Satu dokumen
-- Closing Titik Serah per lokasi: document_type "closing_titik_serah:<id lokasi>"
-- dan kunci tanda tangan basah "closing-titik-serah@<id lokasi>".
CREATE TABLE bast_handover_locations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    schedule_id uuid NOT NULL REFERENCES program_schedules(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (btrim(name) <> ''),
    -- [{"district": "...", "village": "..."}]; village kosong = seluruh kecamatan.
    areas jsonb NOT NULL DEFAULT '[]'::jsonb,
    sort_order integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX bast_handover_locations_name_uq ON bast_handover_locations (schedule_id, lower(btrim(name)));
CREATE INDEX bast_handover_locations_schedule_idx ON bast_handover_locations (schedule_id, sort_order);

CREATE TABLE bast_handover_overrides (
    distribution_slot_id uuid PRIMARY KEY REFERENCES distribution_slots(id) ON DELETE CASCADE,
    location_id uuid NOT NULL REFERENCES bast_handover_locations(id) ON DELETE CASCADE,
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX bast_handover_overrides_location_idx ON bast_handover_overrides (location_id);

-- Titik serah yang sudah ada menjadi lokasi pertama jadwalnya.
INSERT INTO bast_handover_locations (schedule_id, name)
SELECT schedule_id, btrim(handover_location) FROM bast_schedule_settings
WHERE btrim(COALESCE(handover_location, '')) <> '';

ALTER TABLE bast_aggregate_documents DROP CONSTRAINT bast_aggregate_documents_document_type_check;
ALTER TABLE bast_aggregate_documents ADD CONSTRAINT bast_aggregate_documents_document_type_check
    CHECK (document_type IN ('dp3', 'daily_recap', 'closing_titik_serah', 'closing_kabupaten', 'servis_berkala', 'tkdn', 'gas_elpiji', 'gas_elpiji_gabungan')
        OR document_type ~ '^pemeriksaan:[a-z0-9_]{1,40}$'
        OR document_type ~ '^closing_titik_serah:[0-9a-f-]{36}$');

UPDATE bast_aggregate_documents d SET document_type = 'closing_titik_serah:' || l.id::text
FROM bast_handover_locations l
WHERE d.document_type = 'closing_titik_serah' AND l.schedule_id = d.schedule_id;

UPDATE bast_wet_signature_scans s SET document_key = 'closing-titik-serah@' || l.id::text
FROM bast_handover_locations l
WHERE s.document_key = 'closing-titik-serah' AND l.schedule_id = s.schedule_id;

-- +goose Down
-- Hanya lokasi pertama tiap jadwal yang kembali menjadi titik serah tunggal.
CREATE TEMP TABLE first_handover_locations ON COMMIT DROP AS
SELECT DISTINCT ON (schedule_id) id, schedule_id FROM bast_handover_locations ORDER BY schedule_id, sort_order, created_at, id;
DELETE FROM bast_wet_signature_scans WHERE document_key LIKE 'closing-titik-serah@%'
    AND substring(document_key FROM 21) NOT IN (SELECT id::text FROM first_handover_locations);
UPDATE bast_wet_signature_scans SET document_key = 'closing-titik-serah' WHERE document_key LIKE 'closing-titik-serah@%';
DELETE FROM bast_aggregate_documents WHERE document_type LIKE 'closing_titik_serah:%'
    AND substring(document_type FROM 21) NOT IN (SELECT id::text FROM first_handover_locations);
UPDATE bast_aggregate_documents SET document_type = 'closing_titik_serah' WHERE document_type LIKE 'closing_titik_serah:%';
ALTER TABLE bast_aggregate_documents DROP CONSTRAINT bast_aggregate_documents_document_type_check;
ALTER TABLE bast_aggregate_documents ADD CONSTRAINT bast_aggregate_documents_document_type_check
    CHECK (document_type IN ('dp3', 'daily_recap', 'closing_titik_serah', 'closing_kabupaten', 'servis_berkala', 'tkdn', 'gas_elpiji', 'gas_elpiji_gabungan')
        OR document_type ~ '^pemeriksaan:[a-z0-9_]{1,40}$');
DROP TABLE IF EXISTS bast_handover_overrides;
DROP TABLE IF EXISTS bast_handover_locations;
