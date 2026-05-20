DROP INDEX IF EXISTS cost_records_pricing_version_idx;
ALTER TABLE cost_records
  DROP CONSTRAINT IF EXISTS cost_records_pricing_version_fk,
  DROP COLUMN IF EXISTS pricing_version_id;

DROP INDEX IF EXISTS model_pricing_versions_active_idx;
DROP INDEX IF EXISTS model_pricing_versions_model_idx;
DROP TABLE IF EXISTS model_pricing_versions;
