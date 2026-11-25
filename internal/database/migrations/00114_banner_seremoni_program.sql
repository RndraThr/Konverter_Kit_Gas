-- +goose Up
-- The program lines of the seremoni banner ("PROGRAM KEMITRAAN ... & DPR RI"),
-- one or two lines separated by a newline. NULL prints the template's own text.
ALTER TABLE banner_settings ADD COLUMN seremoni_program text;

-- +goose Down
ALTER TABLE banner_settings DROP COLUMN seremoni_program;
