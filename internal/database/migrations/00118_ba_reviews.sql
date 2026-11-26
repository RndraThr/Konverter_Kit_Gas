-- +goose Up
-- Hasil cek BA oleh tim pusat: satu baris per dokumen yang diunggah lapangan
-- (pindaian basah, halaman tanda tangan, arsip daftar hadir) atau per BA
-- perorangan. version = sidik dokumen saat dicek; dokumen diganti = belum dicek.
CREATE TABLE ba_reviews (
    schedule_id uuid NOT NULL REFERENCES program_schedules(id) ON DELETE CASCADE,
    item_key text NOT NULL,
    version text NOT NULL,
    status text NOT NULL CHECK (status IN ('approved', 'rejected')),
    reason text NOT NULL DEFAULT '',
    reviewed_by uuid REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (schedule_id, item_key)
);

-- +goose StatementBegin
CREATE FUNCTION konkit_notify_ba_review() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    PERFORM konkit_notify_schedule('reviews', CASE WHEN TG_OP = 'DELETE' THEN OLD.schedule_id ELSE NEW.schedule_id END);
    RETURN NULL;
END $$;
-- +goose StatementEnd

CREATE TRIGGER ba_reviews_notify AFTER INSERT OR UPDATE OR DELETE ON ba_reviews
    FOR EACH ROW EXECUTE FUNCTION konkit_notify_ba_review();

-- +goose Down
DROP TABLE ba_reviews;
DROP FUNCTION konkit_notify_ba_review();
