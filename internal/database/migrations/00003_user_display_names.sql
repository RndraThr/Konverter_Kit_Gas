-- +goose Up
UPDATE users
SET full_name = username
WHERE btrim(full_name) = '';

-- +goose Down
-- Existing display names are intentionally preserved.
