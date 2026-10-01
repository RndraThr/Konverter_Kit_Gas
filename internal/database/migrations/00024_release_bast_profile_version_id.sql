-- +goose Up
ALTER TABLE bast_individual_documents DROP COLUMN profile_version_id;
ALTER TABLE bast_daily_bundles DROP COLUMN profile_version_id;

-- +goose Down
ALTER TABLE bast_individual_documents ADD COLUMN profile_version_id uuid;
ALTER TABLE bast_daily_bundles ADD COLUMN profile_version_id uuid;
