-- +goose Up
CREATE TABLE program_zones (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    program_id uuid NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
    code text NOT NULL CHECK (code ~ '^[A-Z0-9][A-Z0-9_-]{0,49}$'),
    name text NOT NULL CHECK (btrim(name) <> ''),
    sort_order integer NOT NULL DEFAULT 0 CHECK (sort_order >= 0),
    is_placeholder boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, program_id),
    UNIQUE (program_id, code)
);
CREATE UNIQUE INDEX program_zones_program_name_uq ON program_zones (program_id, lower(name));
CREATE INDEX program_zones_program_sort_idx ON program_zones (program_id, sort_order, name);

CREATE TABLE program_regency_assignments (
    program_id uuid NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
    regency_id uuid NOT NULL REFERENCES regencies(id) ON DELETE RESTRICT,
    zone_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (program_id, regency_id),
    FOREIGN KEY (zone_id, program_id) REFERENCES program_zones(id, program_id) ON DELETE RESTRICT
);
CREATE INDEX program_regency_assignments_zone_regency_idx ON program_regency_assignments (zone_id, regency_id);

INSERT INTO program_zones (program_id, code, name, sort_order, is_placeholder)
SELECT id, 'UNASSIGNED', 'ZONA BELUM DIATUR', 0, true
FROM programs
ON CONFLICT (program_id, code) DO NOTHING;

INSERT INTO program_regency_assignments (program_id, regency_id, zone_id)
SELECT DISTINCT schedules.program_id, schedules.regency_id, zones.id
FROM program_schedules schedules
JOIN program_zones zones
  ON zones.program_id = schedules.program_id
 AND zones.is_placeholder = true
ON CONFLICT (program_id, regency_id) DO NOTHING;

CREATE TABLE program_document_profile_versions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    program_id uuid NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
    version integer NOT NULL CHECK (version > 0),
    title text NOT NULL CHECK (btrim(title) <> ''),
    subtitle text NOT NULL DEFAULT '',
    procurement_description text NOT NULL CHECK (btrim(procurement_description) <> ''),
    document_series text NOT NULL CHECK (btrim(document_series) <> ''),
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'retired')),
    published_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (program_id, version)
);
CREATE INDEX program_document_profiles_published_idx
    ON program_document_profile_versions (program_id, version DESC)
    WHERE status = 'published';

CREATE TABLE program_document_logo_assets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_version_id uuid NOT NULL REFERENCES program_document_profile_versions(id) ON DELETE CASCADE,
    slot_code text NOT NULL CHECK (slot_code ~ '^[a-z][a-z0-9_]{0,49}$'),
    storage_key text NOT NULL UNIQUE,
    original_filename text NOT NULL,
    mime_type text NOT NULL CHECK (mime_type IN ('image/png', 'image/jpeg')),
    byte_size bigint NOT NULL CHECK (byte_size > 0 AND byte_size <= 10485760),
    checksum char(64) NOT NULL CHECK (checksum ~ '^[0-9a-f]{64}$'),
    sort_order integer NOT NULL DEFAULT 0 CHECK (sort_order >= 0),
    max_width_mm numeric(6,2) NOT NULL DEFAULT 35 CHECK (max_width_mm > 0),
    max_height_mm numeric(6,2) NOT NULL DEFAULT 18 CHECK (max_height_mm > 0),
    is_visible boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (profile_version_id, slot_code)
);
CREATE INDEX program_document_logo_assets_order_idx
    ON program_document_logo_assets (profile_version_id, sort_order, id);

-- +goose StatementBegin
CREATE FUNCTION reject_published_program_document_profile_change() RETURNS trigger AS $$
BEGIN
    IF OLD.status = 'published' THEN
        RAISE EXCEPTION 'published document profile is immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER program_document_profile_immutable
BEFORE UPDATE ON program_document_profile_versions
FOR EACH ROW EXECUTE FUNCTION reject_published_program_document_profile_change();

ALTER TABLE activity_media ADD COLUMN program_id uuid REFERENCES programs(id) ON DELETE RESTRICT;

WITH unambiguous_programs AS (
    SELECT regency_id, min(program_id::text)::uuid AS program_id
    FROM program_schedules
    GROUP BY regency_id
    HAVING count(DISTINCT program_id) = 1
)
UPDATE activity_media media
SET program_id = context.program_id
FROM unambiguous_programs context
WHERE context.regency_id = media.regency_id;

CREATE INDEX activity_media_program_regency_type_idx
    ON activity_media (program_id, regency_id, activity_type, status, uploaded_at DESC);

-- +goose Down
DROP INDEX activity_media_program_regency_type_idx;
ALTER TABLE activity_media DROP COLUMN program_id;

DROP TRIGGER program_document_profile_immutable ON program_document_profile_versions;
DROP FUNCTION reject_published_program_document_profile_change();

DROP TABLE program_document_logo_assets;
DROP TABLE program_document_profile_versions;
DROP TABLE program_regency_assignments;
DROP TABLE program_zones;
