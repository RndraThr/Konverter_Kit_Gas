-- +goose Up
CREATE TABLE program_ba_logo_assets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    program_id uuid NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
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
    UNIQUE (program_id, slot_code)
);

CREATE INDEX program_ba_logo_assets_order_idx
    ON program_ba_logo_assets (program_id, sort_order, id);

-- Salin logo dari profil dokumen terbaru per program (published diutamakan, lalu versi tertinggi).
-- Pada database fresh tanpa profil lama, tidak ada baris yang tersalin.
INSERT INTO program_ba_logo_assets
    (program_id, slot_code, storage_key, original_filename, mime_type, byte_size,
     checksum, sort_order, max_width_mm, max_height_mm, is_visible, created_at, updated_at)
SELECT v.program_id, l.slot_code, l.storage_key, l.original_filename, l.mime_type,
       l.byte_size, l.checksum, l.sort_order, l.max_width_mm, l.max_height_mm,
       l.is_visible, l.created_at, l.updated_at
FROM program_document_logo_assets l
JOIN program_document_profile_versions v ON v.id = l.profile_version_id
JOIN LATERAL (
    SELECT pv.id
    FROM program_document_profile_versions pv
    WHERE pv.program_id = v.program_id
    ORDER BY (pv.status = 'published') DESC, pv.version DESC
    LIMIT 1
) latest ON latest.id = v.id;

-- +goose Down
DROP TABLE program_ba_logo_assets;
