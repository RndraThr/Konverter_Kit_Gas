-- +goose Up
-- Hasil cek foto dokumentasi kegiatan (rakor, sosialisasi, unloading, dst.)
-- oleh tim pusat, per foto seperti cek dokumentasi distribusi. Foto pengganti
-- punya id baru, jadi otomatis "belum dicek".
CREATE TABLE activity_media_reviews (
    media_id uuid PRIMARY KEY REFERENCES activity_media(id) ON DELETE CASCADE,
    status text NOT NULL CHECK (status IN ('approved', 'rejected')),
    reason text NOT NULL DEFAULT '',
    reviewed_by uuid REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at timestamptz NOT NULL DEFAULT now()
);

-- Sinyal realtime: halaman monitoring dan galeri kegiatan ikut diperbarui.
-- +goose StatementBegin
CREATE FUNCTION konkit_notify_activity_media_review() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    rec activity_media;
BEGIN
    SELECT * INTO rec FROM activity_media WHERE id = CASE WHEN TG_OP = 'DELETE' THEN OLD.media_id ELSE NEW.media_id END;
    IF FOUND THEN
        PERFORM pg_notify('konkit_changes', json_build_object('t', 'activities', 'program_id', rec.program_id, 'regency_id', rec.regency_id)::text);
    END IF;
    RETURN NULL;
END $$;
-- +goose StatementEnd

CREATE TRIGGER activity_media_reviews_notify AFTER INSERT OR UPDATE OR DELETE ON activity_media_reviews
    FOR EACH ROW EXECUTE FUNCTION konkit_notify_activity_media_review();

-- +goose Down
DROP TABLE activity_media_reviews;
DROP FUNCTION konkit_notify_activity_media_review();
