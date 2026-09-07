-- +goose Up
ALTER TABLE roles ADD COLUMN all_regencies_access boolean NOT NULL DEFAULT false;

CREATE TABLE role_regencies (
    role_id uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    regency_id uuid NOT NULL REFERENCES regencies(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, regency_id)
);

-- +goose Down
DROP TABLE role_regencies;
ALTER TABLE roles DROP COLUMN all_regencies_access;
