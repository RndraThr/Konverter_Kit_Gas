-- +goose Up
DROP TRIGGER IF EXISTS program_document_profile_immutable ON program_document_profile_versions;
DROP FUNCTION IF EXISTS reject_published_program_document_profile_change();
DROP TABLE IF EXISTS program_document_logo_assets;
DROP TABLE IF EXISTS program_document_profile_versions;

-- +goose Down
-- Forward-only: restore the legacy document-profile schema from a backup if needed.
SELECT 1;
