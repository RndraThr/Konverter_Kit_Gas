-- +goose Up
CREATE TABLE drive_folder_cache (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    path_key text NOT NULL UNIQUE,
    drive_folder_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE drive_folder_cache;
