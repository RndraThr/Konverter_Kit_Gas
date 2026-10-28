-- +goose Up
-- Spatial schema for the distribution map: regency boundaries, a point geometry derived from
-- photo coordinates, and the moment a slot got its recipient (linked_at).
CREATE EXTENSION IF NOT EXISTS postgis;

CREATE TABLE regency_boundaries (
    boundary_name text PRIMARY KEY,
    normalized_key text NOT NULL UNIQUE,
    regency_id uuid REFERENCES regencies(id) ON DELETE SET NULL,
    geom geometry(MultiPolygon, 4326) NOT NULL,
    source text NOT NULL,
    license text NOT NULL,
    asset_checksum text NOT NULL
);
CREATE INDEX regency_boundaries_geom_idx ON regency_boundaries USING gist (geom);
CREATE INDEX regency_boundaries_regency_idx ON regency_boundaries (regency_id);

-- latitude/longitude stay the source of truth; 0,0 means "no GPS fix" and gets no geometry.
ALTER TABLE media_files ADD COLUMN geom geometry(Point, 4326)
    GENERATED ALWAYS AS (
        CASE WHEN latitude IS NOT NULL AND longitude IS NOT NULL
                  AND NOT (latitude = 0 AND longitude = 0)
             THEN ST_SetSRID(ST_MakePoint(longitude::float8, latitude::float8), 4326) END
    ) STORED;
CREATE INDEX media_files_geom_idx ON media_files USING gist (geom) WHERE geom IS NOT NULL;

ALTER TABLE distribution_slots ADD COLUMN linked_at timestamptz;
-- A slot is linked before it is delivered, and updated_at moves on every later edit, so the
-- earliest known time is the best estimate. LEAST ignores NULLs.
UPDATE distribution_slots SET linked_at = LEAST(updated_at, distributed_at, completed_at)
WHERE status IN ('linked', 'completed');

-- +goose StatementBegin
CREATE FUNCTION distribution_slots_set_linked_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    -- Stamp a fresh link (from open) or a missing value; an explicit INSERT value and the original
    -- link time of a reopened slot (completed -> linked) are kept.
    IF NEW.status = 'linked' AND (NEW.linked_at IS NULL OR (TG_OP = 'UPDATE' AND OLD.status = 'open')) THEN
        NEW.linked_at := now();
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd

CREATE TRIGGER distribution_slots_set_linked_at
    BEFORE INSERT OR UPDATE OF status ON distribution_slots
    FOR EACH ROW EXECUTE FUNCTION distribution_slots_set_linked_at();

-- +goose Down
DROP TRIGGER IF EXISTS distribution_slots_set_linked_at ON distribution_slots;
DROP FUNCTION IF EXISTS distribution_slots_set_linked_at();
ALTER TABLE distribution_slots DROP COLUMN IF EXISTS linked_at;
DROP INDEX IF EXISTS media_files_geom_idx;
ALTER TABLE media_files DROP COLUMN IF EXISTS geom;
DROP TABLE IF EXISTS regency_boundaries;
-- The postgis extension is left installed on purpose.
