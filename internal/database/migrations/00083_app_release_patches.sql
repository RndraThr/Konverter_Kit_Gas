-- +goose Up
-- Delta updates: a patch from an earlier release's APK to a newer one, so a
-- phone downloads only the difference (a few MB instead of the whole APK).
-- The server makes them in the background for the newest release from the
-- few releases before it. storage_key NULL: no patch (not smaller than the
-- APK, or it could not be made); phones then download the whole APK.
CREATE TABLE app_release_patches (
    release_id uuid NOT NULL REFERENCES app_releases(id) ON DELETE CASCADE,
    from_release_id uuid NOT NULL REFERENCES app_releases(id) ON DELETE CASCADE,
    storage_key text,
    size_bytes bigint NOT NULL DEFAULT 0,
    checksum text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (release_id, from_release_id),
    CHECK (release_id <> from_release_id)
);

CREATE INDEX app_release_patches_from_idx ON app_release_patches (from_release_id);

-- +goose Down
DROP TABLE IF EXISTS app_release_patches;
