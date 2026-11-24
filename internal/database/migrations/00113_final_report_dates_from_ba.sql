-- +goose Up
-- Tanggal pengiriman Laporan Akhir diambil dari BA Pemeriksaan Barang
-- (form Selang Hisap & Buang, Mesin Pompa, Tabung Gas), bukan diisi manual.
ALTER TABLE final_report_settings
    DROP COLUMN delivery_hose_date,
    DROP COLUMN delivery_machine_group_date,
    DROP COLUMN delivery_gas_cylinder_date;

-- +goose Down
ALTER TABLE final_report_settings
    ADD COLUMN delivery_hose_date date,
    ADD COLUMN delivery_machine_group_date date,
    ADD COLUMN delivery_gas_cylinder_date date;
