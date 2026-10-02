-- +goose Up
-- Konfigurasi Berita Acara per jadwal (bukan profil dokumen global). Field ini dipakai
-- header & halaman tanda tangan DP3 dan Rekap Harian. Nilai awal `supervisor_name` diambil
-- dari `program_schedules.supervisor_name` agar tidak terjadi input ulang.
CREATE TABLE bast_schedule_settings (
    schedule_id uuid PRIMARY KEY REFERENCES program_schedules(id) ON DELETE CASCADE,
    handover_location text,
    consultant_company_name text,
    agriculture_office_name text,
    agriculture_office_nip text,
    installer_name text,
    supervisor_name text,
    pertamina_rep_name text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO bast_schedule_settings (schedule_id, supervisor_name)
SELECT id, supervisor_name
FROM program_schedules
WHERE supervisor_name IS NOT NULL AND btrim(supervisor_name) <> ''
ON CONFLICT (schedule_id) DO NOTHING;

-- +goose Down
DROP TABLE bast_schedule_settings;
