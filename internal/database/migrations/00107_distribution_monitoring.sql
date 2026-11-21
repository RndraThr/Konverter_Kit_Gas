-- +goose Up
-- Monitoring distribusi oleh tim pusat.
--
-- schedule_phase_days: jadwal tahapan per kabupaten per tanggal, seperti
-- lembar "Jadwal Pendistribusian dan Pemasangan". Satu tanggal satu tahap.
CREATE TABLE schedule_phase_days (
    schedule_id uuid NOT NULL REFERENCES program_schedules(id) ON DELETE CASCADE,
    day date NOT NULL,
    phase text NOT NULL CHECK (phase IN ('material', 'rakor', 'sosialisasi', 'distribusi', 'penutupan')),
    PRIMARY KEY (schedule_id, day)
);
CREATE INDEX schedule_phase_days_day_idx ON schedule_phase_days (day, phase);

-- Acara serentak satu program (Rakornas), tampil di semua baris jadwal.
CREATE TABLE program_calendar_days (
    program_id uuid NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
    day date NOT NULL,
    kind text NOT NULL CHECK (kind IN ('rakornas')),
    PRIMARY KEY (program_id, day)
);

-- Penugasan per kabupaten (jadwal): reviewer = tim pusat yang memeriksa
-- dokumentasi, leader = leader lapangan yang ikut menerima notifikasi revisi.
CREATE TABLE monitoring_assignments (
    schedule_id uuid NOT NULL REFERENCES program_schedules(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('reviewer', 'leader')),
    PRIMARY KEY (schedule_id, user_id, kind)
);
CREATE INDEX monitoring_assignments_user_idx ON monitoring_assignments (user_id, kind);

-- Hasil cek per foto oleh tim pusat. Foto baru atau pengganti punya id baru,
-- jadi otomatis "belum dicek"; foto yang sudah disetujui tidak perlu dicek
-- lagi walau pelanggan belum selesai (mis. POS Mesin diisi H-1).
CREATE TABLE media_reviews (
    media_id uuid PRIMARY KEY REFERENCES media_files(id) ON DELETE CASCADE,
    status text NOT NULL CHECK (status IN ('approved', 'rejected')),
    reason text NOT NULL DEFAULT '',
    reviewed_by uuid REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at timestamptz NOT NULL DEFAULT now()
);

-- Sinyal realtime: tahapan dan hasil cek ikut diperbarui di semua layar.
-- +goose StatementBegin
CREATE FUNCTION konkit_notify_schedule_phase() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM konkit_notify_schedule('phases', OLD.schedule_id);
    ELSE
        PERFORM konkit_notify_schedule('phases', NEW.schedule_id);
    END IF;
    RETURN NULL;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION konkit_notify_media_review() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    media uuid;
    row_schedule uuid;
BEGIN
    IF TG_OP = 'DELETE' THEN media := OLD.media_id; ELSE media := NEW.media_id; END IF;
    SELECT ds.schedule_id INTO row_schedule
    FROM media_files mf
    JOIN documentation_slots dcs ON dcs.id = mf.documentation_slot_id
    JOIN distribution_slots ds ON ds.id = dcs.distribution_slot_id
    WHERE mf.id = media;
    PERFORM konkit_notify_schedule('reviews', row_schedule);
    RETURN NULL;
END $$;
-- +goose StatementEnd

CREATE TRIGGER schedule_phase_days_notify AFTER INSERT OR UPDATE OR DELETE ON schedule_phase_days
    FOR EACH ROW EXECUTE FUNCTION konkit_notify_schedule_phase();
CREATE TRIGGER monitoring_assignments_notify AFTER INSERT OR UPDATE OR DELETE ON monitoring_assignments
    FOR EACH ROW EXECUTE FUNCTION konkit_notify_schedule_phase();
CREATE TRIGGER media_reviews_notify AFTER INSERT OR UPDATE OR DELETE ON media_reviews
    FOR EACH ROW EXECUTE FUNCTION konkit_notify_media_review();

INSERT INTO permissions (code, name, description) VALUES
    ('monitoring.view', 'Lihat monitoring distribusi', 'Melihat monitoring distribusi live dan jadwal tahapan'),
    ('monitoring.review', 'Cek dokumentasi distribusi', 'Menyetujui dokumentasi pelanggan atau meminta revisi'),
    ('monitoring.manage', 'Kelola monitoring distribusi', 'Mengatur jadwal tahapan dan pembagian tugas monitoring')
ON CONFLICT (code) DO NOTHING;

-- Role admin (tim pusat) mendapat seluruh akses monitoring.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r
JOIN permissions p ON p.code IN ('monitoring.view', 'monitoring.review', 'monitoring.manage')
WHERE r.code = 'admin'
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM permissions WHERE code IN ('monitoring.view', 'monitoring.review', 'monitoring.manage');
DROP TABLE media_reviews;
DROP TABLE monitoring_assignments;
DROP TABLE program_calendar_days;
DROP TABLE schedule_phase_days;
DROP FUNCTION konkit_notify_media_review();
DROP FUNCTION konkit_notify_schedule_phase();
