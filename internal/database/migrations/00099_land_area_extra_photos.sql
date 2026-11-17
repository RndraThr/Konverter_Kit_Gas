-- +goose Up
-- A land area certificate often has two or more pages, so its slot takes up
-- to five photos. Open distributions get the same capacity; slots of
-- completed distributions keep their snapshot.
UPDATE documentation_template_slots SET max_files = 5
WHERE slot_code = 'land_area_certificate' AND max_files = 1;

UPDATE documentation_slots d SET max_files = 5, updated_at = now()
FROM distribution_slots s
WHERE s.id = d.distribution_slot_id AND d.slot_code = 'land_area_certificate'
  AND d.max_files = 1 AND s.status IN ('open', 'linked');

-- +goose Down
-- Slots that already hold extra photos keep their capacity, so no photo is
-- left over its slot's limit.
UPDATE documentation_slots d SET max_files = 1, updated_at = now()
WHERE d.slot_code = 'land_area_certificate' AND d.max_files = 5
  AND NOT EXISTS (SELECT 1 FROM media_files m WHERE m.documentation_slot_id = d.id GROUP BY m.documentation_slot_id HAVING count(*) > 1);

UPDATE documentation_template_slots SET max_files = 1
WHERE slot_code = 'land_area_certificate' AND max_files = 5;
