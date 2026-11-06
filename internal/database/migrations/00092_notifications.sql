-- +goose Up
-- Kotak notifikasi per pengguna. Setiap baris memberi sinyal realtime ke
-- pemiliknya; push ke HP dikirim worker lewat push_state supaya tidak hilang
-- bila FCM sedang gagal.
CREATE TABLE notifications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category text NOT NULL,
    title text NOT NULL,
    body text NOT NULL DEFAULT '',
    link text NOT NULL DEFAULT '',
    read_at timestamptz,
    push_state text NOT NULL DEFAULT 'pending' CHECK (push_state IN ('pending', 'sent', 'skipped', 'failed')),
    push_attempts integer NOT NULL DEFAULT 0,
    next_push_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX notifications_user_idx ON notifications (user_id, created_at DESC);
CREATE INDEX notifications_unread_idx ON notifications (user_id) WHERE read_at IS NULL;
CREATE INDEX notifications_push_idx ON notifications (next_push_at) WHERE push_state = 'pending';

CREATE TABLE device_tokens (
    token text PRIMARY KEY CHECK (length(token) BETWEEN 1 AND 4096),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    platform text NOT NULL DEFAULT 'android' CHECK (platform IN ('android', 'ios')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX device_tokens_user_idx ON device_tokens (user_id);

-- Baris hanya ada untuk kategori yang diubah pengguna; bawaannya aktif.
CREATE TABLE notification_preferences (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category text NOT NULL,
    push_enabled boolean NOT NULL,
    PRIMARY KEY (user_id, category)
);

-- +goose StatementBegin
CREATE FUNCTION konkit_notify_notification() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_notify('konkit_changes', json_build_object('t', 'notification', 'user_id', NEW.user_id)::text);
    RETURN NULL;
END $$;
-- +goose StatementEnd

CREATE TRIGGER notifications_notify AFTER INSERT ON notifications
    FOR EACH ROW EXECUTE FUNCTION konkit_notify_notification();

-- +goose Down
DROP TRIGGER notifications_notify ON notifications;
DROP FUNCTION konkit_notify_notification();
DROP TABLE notification_preferences;
DROP TABLE device_tokens;
DROP TABLE notifications;
