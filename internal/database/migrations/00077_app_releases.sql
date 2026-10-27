-- +goose Up
-- Mobile app releases outside the Play Store: the admin uploads the signed
-- APK with its version; phones ask for the newest one, download it and
-- offer to install it (mandatory releases must be installed). Adding a
-- release signals every connected phone through realtime ('app_release').
CREATE TABLE app_releases (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    platform text NOT NULL DEFAULT 'android' CHECK (platform IN ('android')),
    version_name text NOT NULL CHECK (length(version_name) BETWEEN 1 AND 40),
    version_code integer NOT NULL CHECK (version_code > 0),
    notes text NOT NULL DEFAULT '',
    mandatory boolean NOT NULL DEFAULT false,
    storage_key text NOT NULL,
    size_bytes bigint NOT NULL CHECK (size_bytes > 0),
    checksum text NOT NULL,
    uploaded_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (platform, version_code)
);

-- +goose StatementBegin
CREATE FUNCTION konkit_notify_app_release() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_notify('konkit_changes', json_build_object('t', 'app_release')::text);
    RETURN NULL;
END $$;
-- +goose StatementEnd

CREATE TRIGGER app_releases_notify AFTER INSERT OR UPDATE OR DELETE ON app_releases
    FOR EACH ROW EXECUTE FUNCTION konkit_notify_app_release();

-- +goose Down
DROP TRIGGER IF EXISTS app_releases_notify ON app_releases;
DROP FUNCTION IF EXISTS konkit_notify_app_release();
DROP TABLE IF EXISTS app_releases;
