-- +goose Up
-- Realtime: a change to a person's data (name, NIK, address, village,
-- phone) or to a nomination also signals 'candidates' for every schedule the
-- person is allocated to, so phones pull the corrected recipient data right
-- away instead of at the next reconnect. Payloads are deduplicated per
-- transaction, so a bulk edit still sends one signal per schedule.

-- +goose StatementBegin
CREATE FUNCTION konkit_notify_person_candidates(person uuid) RETURNS void
LANGUAGE plpgsql AS $$
DECLARE
    row_schedule uuid;
BEGIN
    FOR row_schedule IN
        SELECT DISTINCT pa.schedule_id
        FROM candidate_nominations cn JOIN package_allocations pa ON pa.nomination_id = cn.id
        WHERE cn.person_id = person
    LOOP
        PERFORM konkit_notify_schedule('candidates', row_schedule);
    END LOOP;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION konkit_notify_person() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    PERFORM konkit_notify_person_candidates(NEW.id);
    RETURN NULL;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION konkit_notify_nomination() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    row_schedule uuid;
BEGIN
    FOR row_schedule IN
        SELECT DISTINCT schedule_id FROM package_allocations
        WHERE nomination_id = CASE WHEN TG_OP = 'DELETE' THEN OLD.id ELSE NEW.id END
    LOOP
        PERFORM konkit_notify_schedule('candidates', row_schedule);
    END LOOP;
    RETURN NULL;
END $$;
-- +goose StatementEnd

CREATE TRIGGER people_notify AFTER UPDATE ON people
    FOR EACH ROW EXECUTE FUNCTION konkit_notify_person();
CREATE TRIGGER candidate_nominations_notify AFTER INSERT OR UPDATE OR DELETE ON candidate_nominations
    FOR EACH ROW EXECUTE FUNCTION konkit_notify_nomination();

-- +goose Down
DROP TRIGGER IF EXISTS candidate_nominations_notify ON candidate_nominations;
DROP TRIGGER IF EXISTS people_notify ON people;
DROP FUNCTION IF EXISTS konkit_notify_nomination();
DROP FUNCTION IF EXISTS konkit_notify_person();
DROP FUNCTION IF EXISTS konkit_notify_person_candidates(uuid);
