-- +goose Up
-- Realtime: a schedule added or changed (for example moved to another
-- package template version for its regency) or a package template version
-- edited signals 'schedules', so phones reload their schedules and the
-- equipment options at once instead of keeping the old version until the app
-- restarts.

-- +goose StatementBegin
CREATE FUNCTION konkit_notify_program_schedule() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    PERFORM konkit_notify_schedule('schedules', NEW.id);
    RETURN NULL;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION konkit_notify_package_template_version() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    row_schedule uuid;
BEGIN
    FOR row_schedule IN SELECT id FROM program_schedules WHERE package_template_version_id = NEW.id LOOP
        PERFORM konkit_notify_schedule('schedules', row_schedule);
    END LOOP;
    RETURN NULL;
END $$;
-- +goose StatementEnd

CREATE TRIGGER program_schedules_notify AFTER INSERT OR UPDATE ON program_schedules
    FOR EACH ROW EXECUTE FUNCTION konkit_notify_program_schedule();
CREATE TRIGGER package_template_versions_notify AFTER UPDATE ON package_template_versions
    FOR EACH ROW EXECUTE FUNCTION konkit_notify_package_template_version();

-- +goose Down
DROP TRIGGER IF EXISTS package_template_versions_notify ON package_template_versions;
DROP TRIGGER IF EXISTS program_schedules_notify ON program_schedules;
DROP FUNCTION IF EXISTS konkit_notify_package_template_version();
DROP FUNCTION IF EXISTS konkit_notify_program_schedule();
