-- +goose Up
UPDATE package_allocations pa
SET distribution_number = NULL,
    updated_at = now()
FROM candidate_nominations cn
WHERE cn.id = pa.nomination_id
  AND cn.batch_id IS NULL
  AND cn.import_row_id IS NULL
  AND pa.distribution_number IS NOT NULL
  AND pa.status IN ('candidate', 'ready', 'needs_review')
  AND NOT EXISTS (
      SELECT 1
      FROM distribution_slots ds
      WHERE ds.allocation_id = pa.id
  );

-- +goose Down
-- Nomor bagi lama tidak dapat direkonstruksi dengan aman. Rollback tidak mengubah data.
SELECT 1;
