-- +goose Up
-- Digital signatures and stamps of the Berita Acara signatories (Dinas
-- Pertanian, Pelaksana, Konsultan Pengawas, PT Pertamina Patra Niaga), per
-- schedule (one regency), next to the signatory names already kept in
-- bast_schedule_settings. The recipient's own signature stays in
-- distribution_slot_signatures. Images are small transparent PNGs kept in the
-- database, like recipient signatures; the stamp is printed at a fixed spot
-- over the left part of the signature.
CREATE TABLE bast_signatory_assets (
    schedule_id uuid NOT NULL REFERENCES program_schedules(id) ON DELETE CASCADE,
    role text NOT NULL CHECK (role IN ('agriculture_office', 'installer', 'supervisor', 'pertamina')),
    kind text NOT NULL CHECK (kind IN ('signature', 'stamp')),
    image_png bytea NOT NULL,
    byte_size integer NOT NULL CHECK (byte_size > 0),
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (schedule_id, role, kind)
);

-- Which signatures and stamps are printed. document_key 'global' holds the
-- schedule default; a row per Berita Acara (e.g. 'dp3') holds that document's
-- own rules, used only while override is true.
CREATE TABLE bast_signature_rules (
    schedule_id uuid NOT NULL REFERENCES program_schedules(id) ON DELETE CASCADE,
    document_key text NOT NULL,
    override boolean NOT NULL DEFAULT false,
    rules jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(rules) = 'object'),
    -- wet_signature: the last (signature) page is printed, signed by hand,
    -- scanned, and uploaded; previews and the final PDF use the scan.
    wet_signature boolean NOT NULL DEFAULT false,
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (schedule_id, document_key)
);

-- Scanned, hand-signed signature page of one Berita Acara (one document date).
-- content_code identifies the document content the page was printed from; a
-- scan whose code no longer matches (the content changed) is kept as history
-- but not used until the page is signed again.
CREATE TABLE bast_wet_signature_scans (
    schedule_id uuid NOT NULL REFERENCES program_schedules(id) ON DELETE CASCADE,
    document_key text NOT NULL,
    document_date date NOT NULL,
    content_code text NOT NULL,
    image_png bytea NOT NULL,
    byte_size integer NOT NULL CHECK (byte_size > 0),
    uploaded_by uuid REFERENCES users(id) ON DELETE SET NULL,
    uploaded_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (schedule_id, document_key, document_date)
);

-- +goose Down
DROP TABLE IF EXISTS bast_wet_signature_scans;
DROP TABLE IF EXISTS bast_signature_rules;
DROP TABLE IF EXISTS bast_signatory_assets;
