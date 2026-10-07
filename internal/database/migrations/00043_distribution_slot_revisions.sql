-- +goose Up
ALTER TABLE distribution_slots
    ADD COLUMN needs_recompletion boolean NOT NULL DEFAULT false,
    ADD COLUMN reopened_at timestamptz,
    ADD COLUMN reopened_by uuid REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN reopened_stage text,
    ADD COLUMN revision_reason text,
    ADD CONSTRAINT distribution_slots_reopen_stage_check
        CHECK (reopened_stage IS NULL OR reopened_stage IN ('mesin', 'dokumen', 'penyerahan')),
    ADD CONSTRAINT distribution_slots_revision_metadata_check CHECK (
        needs_recompletion = false OR (
            reopened_at IS NOT NULL
            AND reopened_stage IS NOT NULL
            AND btrim(revision_reason) <> ''
        )
    );

ALTER TABLE bast_individual_documents
    DROP CONSTRAINT bast_individual_documents_status_check,
    ADD CONSTRAINT bast_individual_documents_status_check
        CHECK (status IN ('final', 'superseded', 'stale'));

ALTER TABLE bast_daily_bundles
    DROP CONSTRAINT bast_daily_bundles_status_check,
    ADD CONSTRAINT bast_daily_bundles_status_check
        CHECK (status IN ('active', 'superseded', 'failed', 'stale'));

ALTER TABLE bast_aggregate_documents
    DROP CONSTRAINT bast_aggregate_documents_status_check,
    ADD CONSTRAINT bast_aggregate_documents_status_check
        CHECK (status IN ('active', 'superseded', 'stale'));

-- +goose Down
UPDATE bast_individual_documents SET status = 'superseded' WHERE status = 'stale';
UPDATE bast_daily_bundles SET status = 'superseded' WHERE status = 'stale';
UPDATE bast_aggregate_documents SET status = 'superseded' WHERE status = 'stale';

ALTER TABLE bast_aggregate_documents
    DROP CONSTRAINT bast_aggregate_documents_status_check,
    ADD CONSTRAINT bast_aggregate_documents_status_check
        CHECK (status IN ('active', 'superseded'));

ALTER TABLE bast_daily_bundles
    DROP CONSTRAINT bast_daily_bundles_status_check,
    ADD CONSTRAINT bast_daily_bundles_status_check
        CHECK (status IN ('active', 'superseded', 'failed'));

ALTER TABLE bast_individual_documents
    DROP CONSTRAINT bast_individual_documents_status_check,
    ADD CONSTRAINT bast_individual_documents_status_check
        CHECK (status IN ('final', 'superseded'));

ALTER TABLE distribution_slots
    DROP CONSTRAINT distribution_slots_revision_metadata_check,
    DROP CONSTRAINT distribution_slots_reopen_stage_check,
    DROP COLUMN revision_reason,
    DROP COLUMN reopened_stage,
    DROP COLUMN reopened_by,
    DROP COLUMN reopened_at,
    DROP COLUMN needs_recompletion;
