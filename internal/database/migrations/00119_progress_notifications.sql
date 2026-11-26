-- +goose Up
-- Kabupaten yang dipantau dari satu HP (Mode Pusat). NULL: belum memilih.
-- Notifikasi yang terikat ke kabupaten hanya dikirim ke HP yang memilihnya,
-- sehingga satu akun bersama di beberapa HP tetap bisa membagi kabupaten.
ALTER TABLE device_tokens ADD COLUMN regency_ids uuid[];
ALTER TABLE notifications ADD COLUMN regency_id uuid REFERENCES regencies(id) ON DELETE SET NULL;

-- Posisi terakhir ringkasan progres distribusi per jadwal: kapan dan pada
-- jumlah selesai berapa ringkasan per jam terakhir dikirim, tonggak 50% dan
-- 100%, dan hari terakhir rekap harian.
CREATE TABLE distribution_progress_notices (
    schedule_id uuid PRIMARY KEY REFERENCES program_schedules(id) ON DELETE CASCADE,
    hourly_at timestamptz NOT NULL DEFAULT now(),
    hourly_completed integer NOT NULL DEFAULT 0,
    half_sent boolean NOT NULL DEFAULT false,
    full_sent boolean NOT NULL DEFAULT false,
    daily_on date
);

-- +goose Down
DROP TABLE distribution_progress_notices;
ALTER TABLE notifications DROP COLUMN regency_id;
ALTER TABLE device_tokens DROP COLUMN regency_ids;
