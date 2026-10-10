-- +goose Up
-- Realtime change signals for the mobile app (WebSocket /api/v1/realtime).
-- Every change to slots, their documentation and photos, candidate
-- allocations or activity media sends a small NOTIFY on channel
-- konkit_changes: {"t": kind, "schedule_id" | "program_id", "regency_id"}.
-- The app then pulls the delta, so the payload never carries data. Identical
-- payloads in one transaction are delivered once, so bulk imports send one
-- signal per schedule, not one per row.

-- +goose StatementBegin
CREATE FUNCTION konkit_notify_schedule(kind text, schedule uuid) RETURNS void
LANGUAGE plpgsql AS $$
DECLARE
    regency uuid;
BEGIN
    IF schedule IS NULL THEN
        RETURN;
    END IF;
    SELECT regency_id INTO regency FROM program_schedules WHERE id = schedule;
    PERFORM pg_notify('konkit_changes', json_build_object('t', kind, 'schedule_id', schedule, 'regency_id', regency)::text);
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION konkit_notify_distribution_slot() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    row_schedule uuid;
BEGIN
    IF TG_OP = 'DELETE' THEN row_schedule := OLD.schedule_id; ELSE row_schedule := NEW.schedule_id; END IF;
    PERFORM konkit_notify_schedule('slots', row_schedule);
    RETURN NULL;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION konkit_notify_documentation_slot() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    slot uuid;
    row_schedule uuid;
BEGIN
    IF TG_OP = 'DELETE' THEN slot := OLD.distribution_slot_id; ELSE slot := NEW.distribution_slot_id; END IF;
    SELECT schedule_id INTO row_schedule FROM distribution_slots WHERE id = slot;
    PERFORM konkit_notify_schedule('slots', row_schedule);
    RETURN NULL;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION konkit_notify_media_file() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    doc uuid;
    row_schedule uuid;
BEGIN
    IF TG_OP = 'DELETE' THEN doc := OLD.documentation_slot_id; ELSE doc := NEW.documentation_slot_id; END IF;
    SELECT ds.schedule_id INTO row_schedule
    FROM documentation_slots dcs JOIN distribution_slots ds ON ds.id = dcs.distribution_slot_id
    WHERE dcs.id = doc;
    PERFORM konkit_notify_schedule('slots', row_schedule);
    RETURN NULL;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION konkit_notify_package_allocation() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    row_schedule uuid;
BEGIN
    IF TG_OP = 'DELETE' THEN row_schedule := OLD.schedule_id; ELSE row_schedule := NEW.schedule_id; END IF;
    PERFORM konkit_notify_schedule('candidates', row_schedule);
    RETURN NULL;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION konkit_notify_activity_media() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    rec activity_media;
BEGIN
    IF TG_OP = 'DELETE' THEN rec := OLD; ELSE rec := NEW; END IF;
    PERFORM pg_notify('konkit_changes', json_build_object('t', 'activities', 'program_id', rec.program_id, 'regency_id', rec.regency_id)::text);
    RETURN NULL;
END $$;
-- +goose StatementEnd

CREATE TRIGGER distribution_slots_notify AFTER INSERT OR UPDATE OR DELETE ON distribution_slots
    FOR EACH ROW EXECUTE FUNCTION konkit_notify_distribution_slot();
CREATE TRIGGER documentation_slots_notify AFTER INSERT OR UPDATE OR DELETE ON documentation_slots
    FOR EACH ROW EXECUTE FUNCTION konkit_notify_documentation_slot();
CREATE TRIGGER media_files_notify AFTER INSERT OR UPDATE OR DELETE ON media_files
    FOR EACH ROW EXECUTE FUNCTION konkit_notify_media_file();
CREATE TRIGGER package_allocations_notify AFTER INSERT OR UPDATE OR DELETE ON package_allocations
    FOR EACH ROW EXECUTE FUNCTION konkit_notify_package_allocation();
CREATE TRIGGER activity_media_notify AFTER INSERT OR UPDATE OR DELETE ON activity_media
    FOR EACH ROW EXECUTE FUNCTION konkit_notify_activity_media();

-- +goose Down
DROP TRIGGER IF EXISTS activity_media_notify ON activity_media;
DROP TRIGGER IF EXISTS package_allocations_notify ON package_allocations;
DROP TRIGGER IF EXISTS media_files_notify ON media_files;
DROP TRIGGER IF EXISTS documentation_slots_notify ON documentation_slots;
DROP TRIGGER IF EXISTS distribution_slots_notify ON distribution_slots;
DROP FUNCTION IF EXISTS konkit_notify_activity_media();
DROP FUNCTION IF EXISTS konkit_notify_package_allocation();
DROP FUNCTION IF EXISTS konkit_notify_media_file();
DROP FUNCTION IF EXISTS konkit_notify_documentation_slot();
DROP FUNCTION IF EXISTS konkit_notify_distribution_slot();
DROP FUNCTION IF EXISTS konkit_notify_schedule(text, uuid);
