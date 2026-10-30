-- +goose Up
-- A new release is offered to phones only once its delta patches are made;
-- recording a patch signals phones (like a new release) so they check again
-- and see the update right away, with the small download.
CREATE TRIGGER app_release_patches_notify AFTER INSERT ON app_release_patches
    FOR EACH ROW EXECUTE FUNCTION konkit_notify_app_release();

-- +goose Down
DROP TRIGGER IF EXISTS app_release_patches_notify ON app_release_patches;
