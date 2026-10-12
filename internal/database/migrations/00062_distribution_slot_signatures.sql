-- +goose Up
-- Recipient's handwritten signature taken in POS Dokumen. One per slot (one
-- receipt); Berita Acara perorangan and the Training attendance list of that
-- receipt use it. A small PNG (transparent background), so it lives in the
-- database next to the slot instead of in media storage. person_id records
-- whose signature it is: replacing the recipient removes it.
CREATE TABLE distribution_slot_signatures (
    distribution_slot_id uuid PRIMARY KEY REFERENCES distribution_slots(id) ON DELETE CASCADE,
    person_id uuid NOT NULL REFERENCES people(id),
    image_png bytea NOT NULL,
    byte_size integer NOT NULL CHECK (byte_size > 0),
    signed_by uuid REFERENCES users(id),
    signed_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS distribution_slot_signatures;
