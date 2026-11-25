-- +goose Up
-- A release's APK for other phone types. The release's own APK is the 64-bit
-- one (arm64-v8a); a phone that can only run 32-bit apps (many Android Go
-- phones) gets the armeabi-v7a APK stored here. Variants have no delta
-- patches: those phones download the whole APK.
CREATE TABLE app_release_variants (
    release_id uuid NOT NULL REFERENCES app_releases(id) ON DELETE CASCADE,
    abi text NOT NULL CHECK (abi IN ('armeabi-v7a', 'x86_64')),
    storage_key text NOT NULL,
    size_bytes bigint NOT NULL CHECK (size_bytes > 0),
    checksum text NOT NULL,
    uploaded_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (release_id, abi)
);

-- A new variant is an update for its phones: signal them like a release.
CREATE TRIGGER app_release_variants_notify AFTER INSERT OR UPDATE OR DELETE ON app_release_variants
    FOR EACH ROW EXECUTE FUNCTION konkit_notify_app_release();

-- +goose Down
DROP TRIGGER IF EXISTS app_release_variants_notify ON app_release_variants;
DROP TABLE IF EXISTS app_release_variants;
